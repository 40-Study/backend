package handler

// Test HTTP cho route xem thử công khai GET /courses/:slug/preview-lessons/:lesson_id/contents (S1):
// khách nhận video_hls_url KÝ cho bài preview của khoá đã xuất bản; khoá nháp -> 404 (không lộ
// khoá nháp tồn tại); bài không phải preview -> 404. Bỏ điều kiện published/preview ở service hoặc
// bỏ ký URL làm các test này ĐỎ.

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"study.com/v1/internal/hlsauth"
	"study.com/v1/internal/model"
	"study.com/v1/internal/repository"
	"study.com/v1/internal/service"
)

type previewCourseRepo struct {
	repository.CourseRepositoryInterface
	course *model.Course
}

func (r *previewCourseRepo) GetDetailBySlug(context.Context, string) (*model.Course, error) {
	return r.course, nil
}

type previewLessonRepo struct {
	repository.LessonRepositoryInterface
	lesson   *model.Lesson
	contents []model.LessonContent
}

func (r *previewLessonRepo) GetByID(context.Context, uuid.UUID) (*model.Lesson, error) {
	return r.lesson, nil
}
func (r *previewLessonRepo) GetContentsByLessonID(context.Context, uuid.UUID) ([]model.LessonContent, error) {
	return r.contents, nil
}

type previewSectionRepo struct {
	repository.SectionRepositoryInterface
	section *model.Section
}

func (r *previewSectionRepo) GetByID(context.Context, uuid.UUID) (*model.Section, error) {
	return r.section, nil
}

func previewApp(status string, isPreview bool, uploadID uuid.UUID) (*fiber.App, uuid.UUID) {
	courseID, sectionID, lessonID := uuid.New(), uuid.New(), uuid.New()
	course := &model.Course{Status: status}
	course.ID = courseID
	section := &model.Section{CourseID: courseID}
	section.ID = sectionID
	lesson := &model.Lesson{ID: lessonID, SectionID: sectionID, IsPreview: isPreview}
	stored := "/api/hls/" + uploadID.String() + "/master.m3u8"

	svc := service.NewLessonContentService(
		&previewLessonRepo{lesson: lesson, contents: []model.LessonContent{{Type: "video", VideoURL: &stored}}},
		&previewSectionRepo{section: section},
		&previewCourseRepo{course: course},
		nil, nil,
	)
	app := fiber.New()
	app.Get("/courses/:slug/preview-lessons/:lesson_id/contents", NewLessonPreviewHandler(svc).GetPreviewContents)
	return app, lessonID
}

func TestPreviewContents_Khach_NhanURLHLSKy(t *testing.T) {
	uploadID := uuid.New()
	app, lessonID := previewApp(model.CourseStatusPublished, true, uploadID)

	resp, err := app.Test(httptest.NewRequest(http.MethodGet, "/courses/khoa-x/preview-lessons/"+lessonID.String()+"/contents", nil), -1)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, muon 200. body: %s", resp.StatusCode, raw)
	}
	var out struct {
		Data []struct {
			VideoURL    *string `json:"video_url"`
			VideoHLSURL *string `json:"video_hls_url"`
		} `json:"data"`
	}
	if err := json.Unmarshal(raw, &out); err != nil || len(out.Data) != 1 {
		t.Fatalf("body khong hop le: %v %s", err, raw)
	}
	d := out.Data[0]
	if d.VideoHLSURL == nil || !strings.Contains(*d.VideoHLSURL, "sig=") {
		t.Fatalf("khach phai nhan video_hls_url co chu ky, nhan: %s", raw)
	}
	q := hlsauth.QueryOf(*d.VideoHLSURL)
	if _, err := hlsauth.Verify(hlsauth.ScopeStream, uploadID, q.Get("exp"), q.Get("uid"), q.Get("sig"), time.Now()); err != nil {
		t.Errorf("chu ky trong video_hls_url khong hop le: %v", err)
	}
	if d.VideoURL != nil {
		t.Errorf("khach khong duoc nhan video_url (file goc): %s", *d.VideoURL)
	}
}

func TestPreviewContents_KhoaNhapHoacBaiKhongPreview_404(t *testing.T) {
	for name, tc := range map[string]struct {
		status  string
		preview bool
	}{
		"khoa nhap":         {model.CourseStatusDraft, true},
		"bai khong preview": {model.CourseStatusPublished, false},
	} {
		t.Run(name, func(t *testing.T) {
			app, lessonID := previewApp(tc.status, tc.preview, uuid.New())
			resp, err := app.Test(httptest.NewRequest(http.MethodGet, "/courses/khoa-x/preview-lessons/"+lessonID.String()+"/contents", nil), -1)
			if err != nil {
				t.Fatal(err)
			}
			defer resp.Body.Close()
			raw, _ := io.ReadAll(resp.Body)
			if resp.StatusCode != http.StatusNotFound {
				t.Fatalf("status = %d, muon 404. body: %s", resp.StatusCode, raw)
			}
			if strings.Contains(string(raw), "sig=") || strings.Contains(string(raw), "/hls/") {
				t.Errorf("404 van lo URL video: %s", raw)
			}
		})
	}
}
