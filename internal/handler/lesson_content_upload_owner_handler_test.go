package handler

// Test HTTP cho S1, câu hỏi #2: gắn video_url trỏ upload của NGƯỜI KHÁC vào nội dung bài học phải
// trả 403 {code:"UPLOAD_NOT_OWNED"}; upload không tồn tại 404 {code:"UPLOAD_NOT_FOUND"}; upload của
// chính mình / admin thì 201. Chạy service THẬT (chỉ thay repo) qua handler thật.

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"study.com/v1/internal/hlsauth"
	"study.com/v1/internal/model"
	"study.com/v1/internal/repository"
	"study.com/v1/internal/service"
)

type ownUploadRepo struct {
	repository.VideoUploadRepositoryInterface
	uploads map[uuid.UUID]*model.VideoUpload
}

func (r *ownUploadRepo) GetUploadByID(_ context.Context, id uuid.UUID) (*model.VideoUpload, error) {
	if u, ok := r.uploads[id]; ok {
		return u, nil
	}
	return nil, fmt.Errorf("%w: %s", repository.ErrVideoUploadNotFound, id)
}

func (r *ownUploadRepo) GetUploadByObjectKey(_ context.Context, key string) (*model.VideoUpload, error) {
	for _, u := range r.uploads {
		if u.ObjectKey == key {
			return u, nil
		}
	}
	return nil, fmt.Errorf("%w: object_key=%s", repository.ErrVideoUploadNotFound, key)
}

type ownLessonRepo struct {
	repository.LessonRepositoryInterface
	lesson        *model.Lesson
	created       int
	contents      []model.LessonContent
	lessonUpdated bool
}

func (r *ownLessonRepo) GetContentsByLessonID(context.Context, uuid.UUID) ([]model.LessonContent, error) {
	return r.contents, nil
}
func (r *ownLessonRepo) Update(context.Context, *model.Lesson) error {
	r.lessonUpdated = true
	return nil
}
func (r *ownLessonRepo) UpdateContent(context.Context, *model.LessonContent) error { return nil }

func (r *ownLessonRepo) GetByID(context.Context, uuid.UUID) (*model.Lesson, error) {
	return r.lesson, nil
}
func (r *ownLessonRepo) CreateContent(context.Context, *model.LessonContent) error {
	r.created++
	return nil
}

type ownCourseRepo struct {
	repository.CourseRepositoryInterface
	course *model.Course
}

func (r *ownCourseRepo) GetByID(context.Context, uuid.UUID) (*model.Course, error) {
	return r.course, nil
}

type ownSectionRepo struct {
	repository.SectionRepositoryInterface
	section *model.Section
}

func (r *ownSectionRepo) GetByID(context.Context, uuid.UUID) (*model.Section, error) {
	return r.section, nil
}

func createContentAs(t *testing.T, actor uuid.UUID, courseOwner uuid.UUID, uploads map[uuid.UUID]*model.VideoUpload, videoURL string) (int, map[string]any, *ownLessonRepo) {
	t.Helper()
	courseID, sectionID, lessonID := uuid.New(), uuid.New(), uuid.New()
	course := &model.Course{InstructorID: courseOwner, Status: model.CourseStatusDraft}
	course.ID = courseID
	section := &model.Section{CourseID: courseID}
	section.ID = sectionID
	lessons := &ownLessonRepo{lesson: &model.Lesson{ID: lessonID, SectionID: sectionID}}

	svc := service.NewLessonContentService(lessons, &ownSectionRepo{section: section}, &ownCourseRepo{course: course}, nil,
		service.NewVideoUploadService(&ownUploadRepo{uploads: uploads}, nil, nil, nil, nil))
	h := NewLessonContentHandler(svc, nil)

	app := fiber.New()
	app.Post("/lessons/:lesson_id/contents", func(c *fiber.Ctx) error {
		c.Locals("user_id", actor)
		return h.CreateContent(c)
	})

	body := fmt.Sprintf(`{"type":"video","video_url":%q}`, videoURL)
	req := httptest.NewRequest(http.MethodPost, "/lessons/"+lessonID.String()+"/contents", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp, err := app.Test(req, -1)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	var out map[string]any
	_ = json.Unmarshal(raw, &out)
	return resp.StatusCode, out, lessons
}

func TestCreateContent_UploadCuaNguoiKhac_403UploadNotOwned(t *testing.T) {
	a, b := uuid.New(), uuid.New()
	uploadA := uuid.New()
	uploads := map[uuid.UUID]*model.VideoUpload{uploadA: {ID: uploadA, UserID: a}}

	code, out, lessons := createContentAs(t, b, b, uploads, "/api/hls/"+uploadA.String()+"/master.m3u8")
	if code != http.StatusForbidden || out["code"] != "UPLOAD_NOT_OWNED" {
		t.Fatalf("status=%d body=%v, muon 403 UPLOAD_NOT_OWNED", code, out)
	}
	if lessons.created != 0 {
		t.Error("content van duoc tao")
	}
}

func TestCreateContent_UploadKhongTonTai_404UploadNotFound(t *testing.T) {
	a := uuid.New()
	code, out, lessons := createContentAs(t, a, a, map[uuid.UUID]*model.VideoUpload{}, "/api/hls/"+uuid.New().String()+"/master.m3u8")
	if code != http.StatusNotFound || out["code"] != "UPLOAD_NOT_FOUND" {
		t.Fatalf("status=%d body=%v, muon 404 UPLOAD_NOT_FOUND", code, out)
	}
	if lessons.created != 0 {
		t.Error("content van duoc tao")
	}
}

func TestCreateContent_UploadCuaChinhMinh_201(t *testing.T) {
	a := uuid.New()
	uploadA := uuid.New()
	uploads := map[uuid.UUID]*model.VideoUpload{uploadA: {ID: uploadA, UserID: a}}
	code, out, lessons := createContentAs(t, a, a, uploads, "/api/hls/"+uploadA.String()+"/master.m3u8")
	if code != http.StatusCreated || lessons.created != 1 {
		t.Fatalf("status=%d body=%v created=%d, muon 201", code, out, lessons.created)
	}
}

// Review S1 M1, đường thực tế của web: PUT /lessons/:id {subtitle_url} trỏ .vtt của người khác -> 403.
func TestUpdateLesson_PhuDeCuaNguoiKhac_403UploadNotOwned(t *testing.T) {
	hlsauth.ConfigureObjectBucket("videos")
	defer hlsauth.ConfigureObjectBucket("")

	a, b := uuid.New(), uuid.New()
	keyA := "videos/lesson_content/" + uuid.NewString() + "/1_a.vtt"
	uploads := map[uuid.UUID]*model.VideoUpload{uuid.New(): {UserID: a, ObjectKey: keyA}}

	courseID, sectionID, lessonID := uuid.New(), uuid.New(), uuid.New()
	course := &model.Course{InstructorID: b, Status: model.CourseStatusDraft}
	course.ID = courseID
	section := &model.Section{CourseID: courseID}
	section.ID = sectionID
	lessons := &ownLessonRepo{
		lesson:   &model.Lesson{ID: lessonID, SectionID: sectionID},
		contents: []model.LessonContent{{Type: "video", LessonID: lessonID}},
	}
	lessonSvc := service.NewLessonService(lessons, &ownSectionRepo{section: section}, &ownCourseRepo{course: course}, nil).
		WithUploadOwnership(service.NewVideoUploadService(&ownUploadRepo{uploads: uploads}, nil, nil, nil, nil))
	h := NewLessonHandler(lessonSvc, nil)

	app := fiber.New()
	app.Put("/lessons/:id", func(c *fiber.Ctx) error {
		c.Locals("user_id", b)
		return h.UpdateLesson(c)
	})
	body := fmt.Sprintf(`{"subtitle_url":%q}`, "http://localhost:9000/videos/"+keyA)
	req := httptest.NewRequest(http.MethodPut, "/lessons/"+lessonID.String(), strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp, err := app.Test(req, -1)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	var out map[string]any
	_ = json.Unmarshal(raw, &out)
	if resp.StatusCode != http.StatusForbidden || out["code"] != "UPLOAD_NOT_OWNED" {
		t.Fatalf("status=%d body=%s, muon 403 UPLOAD_NOT_OWNED", resp.StatusCode, raw)
	}
	if lessons.lessonUpdated {
		t.Error("bai hoc van bi ghi du bi chan")
	}
}
