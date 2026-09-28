package service

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"study.com/v1/internal/dto"
	"study.com/v1/internal/model"
	"study.com/v1/internal/repository"
)

type guardCourseRepoStub struct {
	repository.CourseRepositoryInterface
	course  *model.Course
	updated bool
}

func (s *guardCourseRepoStub) GetByID(context.Context, uuid.UUID) (*model.Course, error) {
	return s.course, nil
}

func (s *guardCourseRepoStub) Update(context.Context, *model.Course) error {
	s.updated = true
	return nil
}

// Phase 3: giáo viên KHÔNG tự xuất bản được qua PUT /courses/:id nữa — trước phase 3 gửi
// {"status":"published"} là khoá lên trang công khai, không ai duyệt.
func TestUpdateCourse_TeacherCannotSetPublishedDirectly(t *testing.T) {
	owner := uuid.New()
	for _, target := range []string{model.CourseStatusPublished, model.CourseStatusPendingReview, model.CourseStatusRejected} {
		repo := &guardCourseRepoStub{course: &model.Course{InstructorID: owner, Status: model.CourseStatusDraft}}
		svc := NewCourseService(repo, nil, nil, nil)
		status := target
		_, err := svc.UpdateCourse(context.Background(), uuid.New(), owner, false, dto.UpdateCourseDTO{Status: &status})
		if !errors.Is(err, ErrCourseStatusChangeNotAllowed) {
			t.Errorf("draft -> %s qua PUT phai bi chan, err=%v", target, err)
		}
		if repo.updated || repo.course.Status != model.CourseStatusDraft {
			t.Errorf("draft -> %s: khoa hoc van bi ghi", target)
		}
	}
}

// Admin cũng không xuất bản qua PUT (phải đi approve để có reviewed_by/reviewed_at). Dựng khoá
// draft: khoá pending_review giờ bị chặn sớm hơn bởi ErrCourseLockedForReview (Q5, xem
// course_edit_lock_test.go).
func TestUpdateCourse_AdminCannotPublishViaPut(t *testing.T) {
	repo := &guardCourseRepoStub{course: &model.Course{InstructorID: uuid.New(), Status: model.CourseStatusDraft}}
	status := model.CourseStatusPublished
	_, err := NewCourseService(repo, nil, nil, nil).UpdateCourse(context.Background(), uuid.New(), uuid.New(), true, dto.UpdateCourseDTO{Status: &status})
	if !errors.Is(err, ErrCourseStatusChangeNotAllowed) {
		t.Fatalf("admin PUT published phai bi chan, err=%v", err)
	}
}

func TestValidateManualCourseStatusChange_AllowedPairs(t *testing.T) {
	allowed := map[[2]string]bool{
		{model.CourseStatusPublished, model.CourseStatusArchived}: true,
		{model.CourseStatusArchived, model.CourseStatusDraft}:     true,
	}
	for _, from := range model.CourseStatuses {
		for _, to := range model.CourseStatuses {
			err := ValidateManualCourseStatusChange(from, to)
			ok := from == to || allowed[[2]string{from, to}]
			if ok != (err == nil) {
				t.Errorf("%s -> %s: err=%v, muon hop le=%v", from, to, err, ok)
			}
		}
	}
}
