package service

// Test cho S1 (QA 260929): /api/hls/* đòi URL ký. Các test ở đây khoá phía CẤP URL: chỉ người đã
// qua kiểm quyền mới nhận URL HLS ký, học viên/khách không bao giờ nhận URL file gốc, và URL cấp
// ra thật sự verify được bằng hlsauth. Bỏ applyVideoAccess (hoặc để mapper trả URL đã lưu) làm các
// test này ĐỎ.

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"study.com/v1/internal/dto"
	"study.com/v1/internal/hlsauth"
	"study.com/v1/internal/model"
	"study.com/v1/internal/repository"
)

// verifyURL kiểm một URL tương đối "/api/hls/{id}/...?exp=&uid=&sig=" theo scope.
func verifyURL(t *testing.T, raw string, scope string, uploadID uuid.UUID) error {
	t.Helper()
	q := hlsauth.QueryOf(raw)
	_, err := hlsauth.Verify(scope, uploadID, q.Get("exp"), q.Get("uid"), q.Get("sig"), time.Now())
	return err
}

func hlsContent(uploadID uuid.UUID) model.LessonContent {
	stored := "/api/hls/" + uploadID.String() + "/master.m3u8"
	return model.LessonContent{Type: "video", VideoURL: &stored}
}

func ptrStr(p *string) string {
	if p == nil {
		return "<nil>"
	}
	return *p
}

// Học viên đã ghi danh: nhận video_hls_url ký (scope hls, gắn user_id), KHÔNG nhận video_url gốc.
func TestGetContentsByLessonID_HocVienGhiDanh_NhanURLHLSKy_KhongCoURLGoc(t *testing.T) {
	lessonID, courseID, student := uuid.New(), uuid.New(), uuid.New()
	uploadID := uuid.New()
	lesson := &model.Lesson{IsPreview: false}
	lesson.ID = lessonID

	lessonRepo := &fakeLessonRepoForLock{lesson: lesson, contents: []model.LessonContent{hlsContent(uploadID)}}
	course := &model.Course{Sequential: false, InstructorID: uuid.New(), Status: model.CourseStatusPublished}
	enrollmentRepo := &fakeEnrollmentRepoForLock{
		courseID:   courseID,
		enrollment: &model.Enrollment{},
		order:      []repository.LessonOrderInfo{{ID: lessonID}},
	}
	s := NewLessonContentService(lessonRepo, nil, &fakeCourseRepoForLock{course: course}, enrollmentRepo, nil)

	got, err := s.GetContentsByLessonID(context.Background(), lessonID, student, false)
	if err != nil || len(got) != 1 {
		t.Fatalf("hoc vien da ghi danh phai xem duoc: len=%d err=%v", len(got), err)
	}
	c := got[0]
	if c.VideoHLSURL == nil {
		t.Fatal("thieu video_hls_url")
	}
	if err := verifyURL(t, *c.VideoHLSURL, hlsauth.ScopeStream, uploadID); err != nil {
		t.Fatalf("video_hls_url khong verify duoc: %v (url=%s)", err, *c.VideoHLSURL)
	}
	if !strings.Contains(*c.VideoHLSURL, "uid="+student.String()) {
		t.Errorf("chu ky phai gan user_id cua hoc vien: %s", *c.VideoHLSURL)
	}
	if c.VideoURL != nil {
		t.Errorf("hoc vien KHONG duoc nhan video_url (file goc): %s", *c.VideoURL)
	}
	if c.VideoUploadID == nil || *c.VideoUploadID != uploadID.String() {
		t.Errorf("video_upload_id = %v", ptrStr(c.VideoUploadID))
	}
	// URL HLS của học viên không được mở file gốc.
	if err := verifyURL(t, *c.VideoHLSURL, hlsauth.ScopeOriginal, uploadID); err == nil {
		t.Error("URL HLS cua hoc vien verify duoc o scope file goc")
	}
}

// Chủ khoá / admin: còn video_url trỏ file gốc (scope src) để trang quản lý xem lại file vừa upload.
func TestGetContentsByLessonID_ChuKhoaVaAdmin_NhanURLGocKy(t *testing.T) {
	for _, tc := range []struct {
		name    string
		isAdmin bool
		owner   bool
	}{{"chu khoa", false, true}, {"admin", true, false}} {
		t.Run(tc.name, func(t *testing.T) {
			lessonID, courseID, actor := uuid.New(), uuid.New(), uuid.New()
			uploadID := uuid.New()
			lesson := &model.Lesson{}
			lesson.ID = lessonID
			course := &model.Course{InstructorID: uuid.New()}
			if tc.owner {
				course.InstructorID = actor
			}
			s := NewLessonContentService(
				&fakeLessonRepoForLock{lesson: lesson, contents: []model.LessonContent{hlsContent(uploadID)}},
				nil, &fakeCourseRepoForLock{course: course},
				&fakeEnrollmentRepoForLock{courseID: courseID, order: []repository.LessonOrderInfo{{ID: lessonID}}}, nil)

			got, err := s.GetContentsByLessonID(context.Background(), lessonID, actor, tc.isAdmin)
			if err != nil || len(got) != 1 {
				t.Fatalf("len=%d err=%v", len(got), err)
			}
			if got[0].VideoURL == nil || !strings.Contains(*got[0].VideoURL, "/video.mp4?") {
				t.Fatalf("chu khoa/admin phai nhan video_url goc ky, nhan: %s", ptrStr(got[0].VideoURL))
			}
			if err := verifyURL(t, *got[0].VideoURL, hlsauth.ScopeOriginal, uploadID); err != nil {
				t.Errorf("video_url goc khong verify duoc o scope src: %v", err)
			}
			if got[0].VideoHLSURL == nil || verifyURL(t, *got[0].VideoHLSURL, hlsauth.ScopeStream, uploadID) != nil {
				t.Errorf("van phai co video_hls_url ky hop le")
			}
		})
	}
}

// checkStudentVsOwner khẳng định bất biến chung cho MỌI đường trả contents: học viên nhận URL HLS
// ký và không có URL gốc; chủ khoá nhận thêm URL gốc ký.
func checkStudentVsOwner(t *testing.T, where string, student, owner dto.LessonContentResponseDTO) {
	t.Helper()
	if student.VideoHLSURL == nil || !strings.Contains(*student.VideoHLSURL, "sig=") {
		t.Errorf("%s: hoc vien phai nhan video_hls_url ky, nhan: %s", where, ptrStr(student.VideoHLSURL))
	}
	if student.VideoURL != nil {
		t.Errorf("%s: hoc vien KHONG duoc nhan video_url goc: %s", where, *student.VideoURL)
	}
	if owner.VideoURL == nil || !strings.Contains(*owner.VideoURL, "/video.mp4?") || !strings.Contains(*owner.VideoURL, "sig=") {
		t.Errorf("%s: chu khoa phai nhan video_url goc ky, nhan: %s", where, ptrStr(owner.VideoURL))
	}
}

// Ba đường còn lại trả contents (GET /courses/:id, GET /sections/:id/lessons, GET /lessons/:id)
// cũng phải ký URL và tách quyền file gốc — trước đây course_service trả thẳng video_url đã lưu.
func TestCacDuongTraContents_KyURLVaTachQuyenFileGoc(t *testing.T) {
	ctx := context.Background()

	t.Run("GetCourseByID", func(t *testing.T) {
		run := func(viewer uuid.UUID, ownerID uuid.UUID, enrolled bool) dto.LessonContentResponseDTO {
			bai1, bai2, course, _ := buildCourseWithTwoLessons(false, ownerID)
			var enr *model.Enrollment
			if enrolled {
				enr = &model.Enrollment{UserID: viewer, CourseID: course.ID}
			}
			svc := NewCourseService(&fakeCourseDetailRepoForC1{course: course}, nil, nil,
				&fakeEnrollmentRepoForLock{courseID: course.ID, enrollment: enr, order: []repository.LessonOrderInfo{{ID: bai1}, {ID: bai2}}})
			got, err := svc.GetCourseByID(ctx, course.ID, viewer, false)
			if err != nil {
				t.Fatal(err)
			}
			return got.Sections[0].Lessons[1].Contents[0]
		}
		owner := uuid.New()
		checkStudentVsOwner(t, "GetCourseByID", run(uuid.New(), uuid.New(), true), run(owner, owner, false))
	})

	t.Run("GetAllLessons", func(t *testing.T) {
		run := func(viewer, ownerID uuid.UUID, enrolled bool) dto.LessonContentResponseDTO {
			bai1, bai2, course, section := buildCourseWithTwoLessons(false, ownerID)
			var enr *model.Enrollment
			if enrolled {
				enr = &model.Enrollment{UserID: viewer, CourseID: course.ID}
			}
			svc := NewLessonService(&fakeLessonRepoForC1{lessons: section.Lessons}, &fakeSectionRepoForC1{section: section},
				&fakeCourseRepoForC1{course: course},
				&fakeEnrollmentRepoForLock{courseID: course.ID, enrollment: enr, order: []repository.LessonOrderInfo{{ID: bai1}, {ID: bai2}}})
			got, err := svc.GetAllLessons(ctx, section.ID, viewer, false)
			if err != nil {
				t.Fatal(err)
			}
			return got[1].Contents[0]
		}
		owner := uuid.New()
		checkStudentVsOwner(t, "GetAllLessons", run(uuid.New(), uuid.New(), true), run(owner, owner, false))
	})

	t.Run("GetLessonByID", func(t *testing.T) {
		run := func(viewer, ownerID uuid.UUID, enrolled bool) dto.LessonContentResponseDTO {
			lessonID, courseID := uuid.New(), uuid.New()
			lesson := &model.Lesson{}
			lesson.ID = lessonID
			var enr *model.Enrollment
			if enrolled {
				enr = &model.Enrollment{UserID: viewer, CourseID: courseID}
			}
			svc := NewLessonService(
				&fakeLessonRepoForLock{lesson: lesson, contents: []model.LessonContent{hlsContent(uuid.New())}},
				nil,
				&fakeCourseRepoForLock{course: &model.Course{InstructorID: ownerID, Status: model.CourseStatusPublished}},
				&fakeEnrollmentRepoForLock{courseID: courseID, enrollment: enr, order: []repository.LessonOrderInfo{{ID: lessonID}}})
			got, err := svc.GetLessonByID(ctx, lessonID, viewer, false)
			if err != nil {
				t.Fatal(err)
			}
			return got.Contents[0]
		}
		owner := uuid.New()
		checkStudentVsOwner(t, "GetLessonByID", run(uuid.New(), uuid.New(), true), run(owner, owner, false))
	})
}

// Học viên CHƯA ghi danh (bài không preview) không nhận được URL ký nào — chỉ ErrLessonLocked.
func TestGetContentsByLessonID_HocVienChuaGhiDanh_KhongNhanURLKy(t *testing.T) {
	lessonID, courseID := uuid.New(), uuid.New()
	lesson := &model.Lesson{IsPreview: false}
	lesson.ID = lessonID
	s := NewLessonContentService(
		&fakeLessonRepoForLock{lesson: lesson, contents: []model.LessonContent{hlsContent(uuid.New())}},
		nil, &fakeCourseRepoForLock{course: &model.Course{InstructorID: uuid.New(), Status: model.CourseStatusPublished}},
		&fakeEnrollmentRepoForLock{courseID: courseID, enrollment: nil, order: []repository.LessonOrderInfo{{ID: lessonID}}}, nil)

	got, err := s.GetContentsByLessonID(context.Background(), lessonID, uuid.New(), false)
	if err != ErrLessonLocked || len(got) != 0 {
		t.Fatalf("chua ghi danh phai ErrLessonLocked khong co noi dung, nhan len=%d err=%v", len(got), err)
	}
}

// Khách xem bài preview của khoá đã xuất bản: nhận URL HLS ký (không user_id), không có URL gốc.
func TestGetPreviewContentsByLessonID_Khach_NhanURLHLSKy(t *testing.T) {
	svc, lessonID := buildPreviewFixture(model.CourseStatusPublished, true)
	uploadID := uuid.New()
	svc.lessonRepo = &previewLessonRepoStub{
		lesson:   svc.lessonRepo.(*previewLessonRepoStub).lesson,
		contents: []model.LessonContent{hlsContent(uploadID)},
	}

	got, err := svc.GetPreviewContentsByLessonID(context.Background(), "khoa-da-xuat-ban", lessonID)
	if err != nil || len(got) != 1 {
		t.Fatalf("len=%d err=%v", len(got), err)
	}
	c := got[0]
	if c.VideoHLSURL == nil || verifyURL(t, *c.VideoHLSURL, hlsauth.ScopeStream, uploadID) != nil {
		t.Fatalf("khach xem thu phai nhan video_hls_url ky hop le, nhan: %s", ptrStr(c.VideoHLSURL))
	}
	if strings.Contains(*c.VideoHLSURL, "uid=") {
		t.Errorf("khach khong co user_id, URL khong duoc gan uid: %s", *c.VideoHLSURL)
	}
	if c.VideoURL != nil {
		t.Errorf("khach KHONG duoc nhan URL video goc: %s", *c.VideoURL)
	}
}

// Khoá nháp: route xem thử từ chối (handler ánh xạ ErrCourseHidden -> 404), không cấp URL nào.
func TestGetPreviewContentsByLessonID_KhoaNhap_KhongCapURL(t *testing.T) {
	svc, lessonID := buildPreviewFixture(model.CourseStatusDraft, true)
	svc.lessonRepo = &previewLessonRepoStub{
		lesson:   svc.lessonRepo.(*previewLessonRepoStub).lesson,
		contents: []model.LessonContent{hlsContent(uuid.New())},
	}
	got, err := svc.GetPreviewContentsByLessonID(context.Background(), "khoa-nhap", lessonID)
	if err != ErrCourseHidden || len(got) != 0 {
		t.Fatalf("khoa nhap phai ErrCourseHidden khong co noi dung, nhan len=%d err=%v", len(got), err)
	}
}

// Video ngoài hệ thống (video mẫu của seed) giữ nguyên URL; đường chưa kiểm quyền không cấp URL nội bộ.
func TestApplyVideoAccess_NgoaiHeThongVaWithheld(t *testing.T) {
	external := "https://interactive-examples.mdn.mozilla.net/media/cc0-videos/flower.mp4"
	var r dto.LessonContentResponseDTO
	applyVideoAccess(&r, &external, guestVideoViewer)
	if r.VideoURL == nil || *r.VideoURL != external || r.VideoHLSURL != nil {
		t.Errorf("video ngoai he thong phai giu nguyen: %+v", r)
	}

	internal := "/api/hls/" + uuid.New().String() + "/master.m3u8"
	var w dto.LessonContentResponseDTO
	applyVideoAccess(&w, &internal, withheldVideoViewer)
	if w.VideoURL != nil || w.VideoHLSURL != nil || w.VideoUploadID != nil {
		t.Errorf("duong chua kiem quyen khong duoc cap/lo URL noi bo: %+v", w)
	}
}

// GetContentByID không nhận người gọi: không có căn cứ cấp URL, và cũng không được lộ URL đã lưu.
func TestGetContentByID_KhongCapURLVideoNoiBo(t *testing.T) {
	uploadID := uuid.New()
	c := hlsContent(uploadID)
	c.ID = uuid.New()
	s := NewLessonContentService(&fakeLessonRepoOneContent{content: &c}, nil, nil, nil, nil)
	got, err := s.GetContentByID(context.Background(), c.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.VideoURL != nil || got.VideoHLSURL != nil {
		t.Errorf("GetContentByID lo URL video noi bo: %s / %s", ptrStr(got.VideoURL), ptrStr(got.VideoHLSURL))
	}
}

type fakeLessonRepoOneContent struct {
	repository.LessonRepositoryInterface
	content *model.LessonContent
}

func (f *fakeLessonRepoOneContent) GetContentByID(context.Context, uuid.UUID) (*model.LessonContent, error) {
	return f.content, nil
}
