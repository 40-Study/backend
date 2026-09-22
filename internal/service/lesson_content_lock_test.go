package service

// Test cho B-2 + CAO-4 (review vòng 2, PR #60):
//   - B-2 / C-4 (reviewer): LessonContentService.GetContentsByLessonID phải trả về đúng
//     ErrLessonLocked khi bài bị khoá đối với người gọi — mutation M3 của reviewer (xoá dòng
//     `return nil, ErrLessonLocked`) phải làm test này ĐỎ.
//   - CAO-4 / C-1 (reviewer): giảng viên sở hữu khoá học chứa bài này, hoặc admin hệ thống,
//     KHÔNG BAO GIỜ bị khoá — kể cả khi chưa enroll (not_enrolled đáng lẽ khoá mọi bài không
//     phải preview).

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"study.com/v1/internal/model"
	"study.com/v1/internal/repository"
)

// fakeLessonRepoForLock: lesson KHÔNG phải preview, có sẵn một content để khẳng định "khi khoá
// thì KHÔNG được nạp/tra contents" (đường lộ dữ liệu mà B-2/B-3 đóng lại).
type fakeLessonRepoForLock struct {
	repository.LessonRepositoryInterface
	lesson          *model.Lesson
	contents        []model.LessonContent
	gotContentsCall bool
}

func (f *fakeLessonRepoForLock) GetByID(ctx context.Context, id uuid.UUID) (*model.Lesson, error) {
	return f.lesson, nil
}

func (f *fakeLessonRepoForLock) GetContentsByLessonID(ctx context.Context, lessonID uuid.UUID) ([]model.LessonContent, error) {
	f.gotContentsCall = true
	return f.contents, nil
}

// fakeCourseRepoForLock trả về course cố định (Sequential + InstructorID cấu hình được).
type fakeCourseRepoForLock struct {
	repository.CourseRepositoryInterface
	course *model.Course
}

func (f *fakeCourseRepoForLock) GetByID(ctx context.Context, id uuid.UUID) (*model.Course, error) {
	return f.course, nil
}

// fakeEnrollmentRepoForLock: enrolled=nil nghĩa là CHƯA enroll (not_enrolled).
type fakeEnrollmentRepoForLock struct {
	repository.EnrollmentRepositoryInterface
	courseID   uuid.UUID
	enrollment *model.Enrollment
	order      []repository.LessonOrderInfo
	progress   map[uuid.UUID]*model.LessonProgress
}

func (f *fakeEnrollmentRepoForLock) GetCourseIDByLessonID(ctx context.Context, lessonID uuid.UUID) (uuid.UUID, error) {
	return f.courseID, nil
}

func (f *fakeEnrollmentRepoForLock) GetByUserAndCourse(ctx context.Context, userID, courseID uuid.UUID) (*model.Enrollment, error) {
	return f.enrollment, nil
}

func (f *fakeEnrollmentRepoForLock) GetLessonOrderInfoByCourseID(ctx context.Context, courseID uuid.UUID) ([]repository.LessonOrderInfo, error) {
	return f.order, nil
}

func (f *fakeEnrollmentRepoForLock) GetLessonProgressMapByUserAndCourse(ctx context.Context, userID, courseID uuid.UUID) (map[uuid.UUID]*model.LessonProgress, error) {
	return f.progress, nil
}

// TestGetContentsByLessonID_BaiKhoa_TraErrLessonLocked (B-2, mutation M3 của reviewer): người
// dùng CHƯA enroll, bài không phải preview => phải khoá và trả ErrLessonLocked, KHÔNG được nạp
// contents thật (gotContentsCall phải vẫn false).
func TestGetContentsByLessonID_BaiKhoa_TraErrLessonLocked(t *testing.T) {
	lessonID := uuid.New()
	courseID := uuid.New()
	lesson := &model.Lesson{IsPreview: false}
	lesson.ID = lessonID

	lessonRepo := &fakeLessonRepoForLock{
		lesson:   lesson,
		contents: []model.LessonContent{{Type: "video", VideoURL: strPtr("secret.mp4")}},
	}
	courseRepo := &fakeCourseRepoForLock{course: &model.Course{Sequential: true}}
	enrollmentRepo := &fakeEnrollmentRepoForLock{
		courseID:   courseID,
		enrollment: nil, // chua enroll
		order:      []repository.LessonOrderInfo{{ID: lessonID, IsPreview: false}},
	}

	s := NewLessonContentService(lessonRepo, nil, courseRepo, enrollmentRepo, nil)

	_, err := s.GetContentsByLessonID(context.Background(), lessonID, uuid.New(), false)

	if err != ErrLessonLocked {
		t.Fatalf("err = %v, muon ErrLessonLocked", err)
	}
	if lessonRepo.gotContentsCall {
		t.Fatal("contents THAT da bi nap du bai dang khoa — day chinh la duong lo video_url B-2 dong lai")
	}
}

// TestGetContentsByLessonID_LessonKhongThuocKhoa_TraErrLessonNotInCourse (quyet dinh team lead,
// review vong 2): lessonID khong nam trong LessonOrder ma GetLessonOrderInfoByCourseID tra ve
// (du lieu khong nhat quan — courseID van duoc suy dung tu lessonID, nhung LessonOrder cua chinh
// khoa do lai KHONG chua lessonID nay) phai la ErrLessonNotInCourse, KHONG duoc ResolveLessonLock
// am tham mo (locked=false vi idx==-1) va cung KHONG duoc nap contents that.
func TestGetContentsByLessonID_LessonKhongThuocKhoa_TraErrLessonNotInCourse(t *testing.T) {
	lessonID := uuid.New()
	courseID := uuid.New()
	lesson := &model.Lesson{IsPreview: false}
	lesson.ID = lessonID

	lessonRepo := &fakeLessonRepoForLock{
		lesson:   lesson,
		contents: []model.LessonContent{{Type: "video", VideoURL: strPtr("secret.mp4")}},
	}
	courseRepo := &fakeCourseRepoForLock{course: &model.Course{Sequential: true}}
	enrollmentRepo := &fakeEnrollmentRepoForLock{
		courseID:   courseID,
		enrollment: &model.Enrollment{},
		// order KHONG chua lessonID — mo phong du lieu khong nhat quan.
		order: []repository.LessonOrderInfo{{ID: uuid.New(), IsPreview: false}},
	}

	s := NewLessonContentService(lessonRepo, nil, courseRepo, enrollmentRepo, nil)

	_, err := s.GetContentsByLessonID(context.Background(), lessonID, uuid.New(), false)

	if err != ErrLessonNotInCourse {
		t.Fatalf("err = %v, muon ErrLessonNotInCourse", err)
	}
	if lessonRepo.gotContentsCall {
		t.Fatal("contents THAT da bi nap du lessonID khong thuoc khoa dang xet")
	}
}

// TestGetContentsByLessonID_GiangVienSoHuu_KhongBiKhoa (CAO-4): giang vien la InstructorID cua
// course chua bai nay, CHUA enroll (dieu ma luat not_enrolled dang le khoa MOI bai) — van phai
// xem duoc contents vi ho SO HUU khoa hoc.
func TestGetContentsByLessonID_GiangVienSoHuu_KhongBiKhoa(t *testing.T) {
	lessonID := uuid.New()
	courseID := uuid.New()
	instructorID := uuid.New()
	lesson := &model.Lesson{IsPreview: false}
	lesson.ID = lessonID

	lessonRepo := &fakeLessonRepoForLock{
		lesson:   lesson,
		contents: []model.LessonContent{{Type: "video"}},
	}
	course := &model.Course{Sequential: true, InstructorID: instructorID}
	courseRepo := &fakeCourseRepoForLock{course: course}
	enrollmentRepo := &fakeEnrollmentRepoForLock{
		courseID:   courseID,
		enrollment: nil, // giang vien khong can "enroll" khoa cua chinh minh
		order:      []repository.LessonOrderInfo{{ID: lessonID, IsPreview: false}},
	}

	s := NewLessonContentService(lessonRepo, nil, courseRepo, enrollmentRepo, nil)

	result, err := s.GetContentsByLessonID(context.Background(), lessonID, instructorID, false)

	if err != nil {
		t.Fatalf("err = %v, muon nil (giang vien so huu khong bi khoa)", err)
	}
	if len(result) != 1 {
		t.Fatalf("len(result) = %d, muon 1 (giang vien phai xem duoc content that)", len(result))
	}
}

// TestGetContentsByLessonID_Admin_KhongBiKhoa (CAO-4): admin he thong (isAdmin=true) khong bi
// khoa du khong phai chu so huu khoa hoc va chua enroll.
func TestGetContentsByLessonID_Admin_KhongBiKhoa(t *testing.T) {
	lessonID := uuid.New()
	courseID := uuid.New()
	lesson := &model.Lesson{IsPreview: false}
	lesson.ID = lessonID

	lessonRepo := &fakeLessonRepoForLock{
		lesson:   lesson,
		contents: []model.LessonContent{{Type: "video"}},
	}
	courseRepo := &fakeCourseRepoForLock{course: &model.Course{Sequential: true, InstructorID: uuid.New()}}
	enrollmentRepo := &fakeEnrollmentRepoForLock{
		courseID:   courseID,
		enrollment: nil,
		order:      []repository.LessonOrderInfo{{ID: lessonID, IsPreview: false}},
	}

	s := NewLessonContentService(lessonRepo, nil, courseRepo, enrollmentRepo, nil)

	result, err := s.GetContentsByLessonID(context.Background(), lessonID, uuid.New(), true)

	if err != nil {
		t.Fatalf("err = %v, muon nil (admin khong bi khoa)", err)
	}
	if len(result) != 1 {
		t.Fatalf("len(result) = %d, muon 1", len(result))
	}
}
