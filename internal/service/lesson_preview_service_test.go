package service

// Test cho F1 (QA vòng 2, 260929): "khách chưa đăng nhập xem bài preview" — trước bản vá
// `GET /lessons/:lesson_id/contents` luôn nằm sau `auth`, khách nhận 401 dù bài đã đánh dấu
// is_preview trên khoá published. GetPreviewContentsByLessonID là route công khai thay thế,
// PHẢI từ chối (ErrCourseHidden/ErrLessonNotPreview/ErrLessonNotInCourse) trong mọi trường hợp
// không phải "khoá published + bài is_preview thuộc đúng khoá đó", để không lộ khoá nháp.

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"study.com/v1/internal/model"
	"study.com/v1/internal/repository"
)

type previewCourseRepoStub struct {
	repository.CourseRepositoryInterface
	course *model.Course
}

func (s *previewCourseRepoStub) GetDetailBySlug(context.Context, string) (*model.Course, error) {
	return s.course, nil
}

type previewLessonRepoStub struct {
	repository.LessonRepositoryInterface
	lesson   *model.Lesson
	contents []model.LessonContent
}

func (s *previewLessonRepoStub) GetByID(context.Context, uuid.UUID) (*model.Lesson, error) {
	return s.lesson, nil
}

func (s *previewLessonRepoStub) GetContentsByLessonID(context.Context, uuid.UUID) ([]model.LessonContent, error) {
	return s.contents, nil
}

type previewSectionRepoStub struct {
	repository.SectionRepositoryInterface
	section *model.Section
}

func (s *previewSectionRepoStub) GetByID(context.Context, uuid.UUID) (*model.Section, error) {
	return s.section, nil
}

func buildPreviewFixture(courseStatus string, lessonIsPreview bool) (*LessonContentService, uuid.UUID) {
	courseID := uuid.New()
	sectionID := uuid.New()
	lessonID := uuid.New()
	videoURL := "https://cdn.example/preview.mp4"

	course := &model.Course{Status: courseStatus}
	course.ID = courseID
	section := &model.Section{CourseID: courseID}
	section.ID = sectionID
	lesson := &model.Lesson{ID: lessonID, SectionID: sectionID, IsPreview: lessonIsPreview}

	svc := NewLessonContentService(
		&previewLessonRepoStub{lesson: lesson, contents: []model.LessonContent{{Type: "video", VideoURL: &videoURL}}},
		&previewSectionRepoStub{section: section},
		&previewCourseRepoStub{course: course},
		nil, // enrollmentRepo: không dùng ở đường public này.
		nil, // videoUploadService: không dùng.
	)
	return svc, lessonID
}

func TestGetPreviewContentsByLessonID_PublishedVaPreview_TraContents(t *testing.T) {
	svc, lessonID := buildPreviewFixture(model.CourseStatusPublished, true)

	got, err := svc.GetPreviewContentsByLessonID(context.Background(), "khoa-da-xuat-ban", lessonID)
	if err != nil {
		t.Fatalf("khoa published + bai preview phai duoc xem, loi: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("len(contents) = %d, muon 1", len(got))
	}
}

// Revert điều kiện `lesson.IsPreview` (coi mọi bài là preview) sẽ làm test này ĐỎ.
func TestGetPreviewContentsByLessonID_BaiKhongPhaiPreview_TuChoi(t *testing.T) {
	svc, lessonID := buildPreviewFixture(model.CourseStatusPublished, false)

	_, err := svc.GetPreviewContentsByLessonID(context.Background(), "khoa-da-xuat-ban", lessonID)
	if err != ErrLessonNotPreview {
		t.Fatalf("bai khong phai preview phai bi tu choi ErrLessonNotPreview, nhan duoc: %v", err)
	}
}

// Revert điều kiện `course.Status == published` (bỏ qua trạng thái khoá) sẽ làm test này ĐỎ —
// đây chính là ràng buộc "không để lộ khoá nháp" của F1.
func TestGetPreviewContentsByLessonID_KhoaChuaPublished_TuChoi(t *testing.T) {
	svc, lessonID := buildPreviewFixture(model.CourseStatusDraft, true)

	_, err := svc.GetPreviewContentsByLessonID(context.Background(), "khoa-nhap", lessonID)
	if err != ErrCourseHidden {
		t.Fatalf("khoa draft phai bi tu choi ErrCourseHidden, nhan duoc: %v", err)
	}
}

// M3 (review): bài preview thuộc KHOÁ KHÁC (section.CourseID != course.ID) phải bị từ chối dù cả hai
// điều kiện published/is_preview đều đúng. Bỏ điều kiện đối chiếu CourseID làm test này ĐỎ.
func TestGetPreviewContentsByLessonID_BaiThuocKhoaKhac_TuChoi(t *testing.T) {
	svc, lessonID := buildPreviewFixture(model.CourseStatusPublished, true)
	// Đổi section trả về sang một khoá khác với khoá của slug trên URL.
	otherSection := &model.Section{CourseID: uuid.New()}
	svc.sectionRepo = &previewSectionRepoStub{section: otherSection}

	_, err := svc.GetPreviewContentsByLessonID(context.Background(), "khoa-da-xuat-ban", lessonID)
	if err != ErrLessonNotInCourse {
		t.Fatalf("bai thuoc khoa khac phai bi tu choi ErrLessonNotInCourse, nhan duoc: %v", err)
	}
}

func TestGetPreviewContentsByLessonID_KhoaKhongTonTai_TuChoi(t *testing.T) {
	svc := NewLessonContentService(
		&previewLessonRepoStub{},
		&previewSectionRepoStub{},
		&previewCourseRepoStub{course: nil},
		nil,
		nil,
	)

	_, err := svc.GetPreviewContentsByLessonID(context.Background(), "khong-ton-tai", uuid.New())
	if err != ErrCourseHidden {
		t.Fatalf("slug khong ton tai phai tra ErrCourseHidden, nhan duoc: %v", err)
	}
}
