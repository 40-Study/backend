package service

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"study.com/v1/internal/model"
	"study.com/v1/internal/repository"
)

// fakeScheduleRepoForSubmission là fake tối thiểu cho ScheduleRepositoryInterface, chỉ
// override TeacherCanManageSession (duy nhất method canManageAssignment cần).
type fakeScheduleRepoForSubmission struct {
	repository.ScheduleRepositoryInterface
	allowedTeacherID uuid.UUID
	sessionID        uuid.UUID
}

func (f *fakeScheduleRepoForSubmission) TeacherCanManageSession(ctx context.Context, sessionID, teacherID uuid.UUID) (bool, error) {
	return sessionID == f.sessionID && teacherID == f.allowedTeacherID, nil
}

// fakeClassRepoForSubmission là fake tối thiểu cho ClassRepositoryInterface, chỉ override
// TeacherClassExists.
type fakeClassRepoForSubmission struct {
	repository.ClassRepositoryInterface
	allowedTeacherID uuid.UUID
	classID          uuid.UUID
}

func (f *fakeClassRepoForSubmission) TeacherClassExists(ctx context.Context, classID, teacherID uuid.UUID) (bool, error) {
	return classID == f.classID && teacherID == f.allowedTeacherID, nil
}

// TestCanManageAssignment (H-03, review vòng 1) — pin lại đúng hợp đồng ownership dùng chung
// cho GetByAssignment VÀ canAccessSubmission: giáo viên "sở hữu" assignment qua SessionID
// (live coding) HOẶC ClassID (bài tập về nhà) hoặc admin mới được xem, người khác thì không.
func TestCanManageAssignment(t *testing.T) {
	teacherID := uuid.New()
	otherTeacherID := uuid.New()
	sessionID := uuid.New()
	classID := uuid.New()

	svc := &SubmissionService{
		scheduleRepo: &fakeScheduleRepoForSubmission{allowedTeacherID: teacherID, sessionID: sessionID},
		classRepo:    &fakeClassRepoForSubmission{allowedTeacherID: teacherID, classID: classID},
	}

	tests := []struct {
		name       string
		assignment *model.Assignment
		requester  uuid.UUID
		isAdmin    bool
		want       bool
	}{
		{
			name:       "giao vien dung session -> cho phep",
			assignment: &model.Assignment{SessionID: &sessionID},
			requester:  teacherID,
			want:       true,
		},
		{
			name:       "giao vien KHONG day session do -> chan",
			assignment: &model.Assignment{SessionID: &sessionID},
			requester:  otherTeacherID,
			want:       false,
		},
		{
			name:       "giao vien dung lop (homework, SessionID nil) -> cho phep",
			assignment: &model.Assignment{ClassID: &classID},
			requester:  teacherID,
			want:       true,
		},
		{
			name:       "giao vien KHONG day lop do -> chan",
			assignment: &model.Assignment{ClassID: &classID},
			requester:  otherTeacherID,
			want:       false,
		},
		{
			name:       "khong co session lan class -> chan",
			assignment: &model.Assignment{},
			requester:  teacherID,
			want:       false,
		},
		{
			name:       "isAdmin bo qua moi kiem tra",
			assignment: &model.Assignment{SessionID: &sessionID},
			requester:  otherTeacherID,
			isAdmin:    true,
			want:       true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := svc.canManageAssignment(context.Background(), tt.assignment, tt.requester, tt.isAdmin)
			if err != nil {
				t.Fatalf("canManageAssignment() unexpected error: %v", err)
			}
			if got != tt.want {
				t.Errorf("canManageAssignment() = %v, want %v", got, tt.want)
			}
		})
	}
}
