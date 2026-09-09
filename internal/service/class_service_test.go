package service

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"study.com/v1/internal/repository"
)

// fakeClassRepoForTeacherCheck (H-11, audit 260909 vòng 2): fake tối giản chỉ implement
// TeacherClassExists — embed interface nil để MỌI method khác panic nếu lỡ bị gọi (an toàn:
// test fail rõ ràng thay vì âm thầm trả zero-value sai). Codebase này không có thư viện mock
// (không có sqlmock trong go.sum) nên đây là cách chuẩn để test logic phụ thuộc repository mà
// không cần DB thật — cùng tinh thần với gợi ý "sqlmock nếu có, không thì test logic thuần"
// mà lead đưa ra cho voucher race.
type fakeClassRepoForTeacherCheck struct {
	repository.ClassRepositoryInterface
	teacherClassExists bool
	err                error
}

func (f *fakeClassRepoForTeacherCheck) TeacherClassExists(ctx context.Context, classID, teacherID uuid.UUID) (bool, error) {
	return f.teacherClassExists, f.err
}

// TestRequireClassTeacherOrAdmin (H-11) kiểm tra 3 nhánh của requireClassTeacherOrAdmin:
//  1. isAdmin=true -> luôn qua, KHÔNG cần gọi repo (an toàn cả khi repo nil).
//  2. isAdmin=false, actor là giáo viên lớp (TeacherClassExists=true) -> qua.
//  3. isAdmin=false, actor KHÔNG phải giáo viên lớp -> ErrNotClassTeacher (403 ở handler).
func TestRequireClassTeacherOrAdmin(t *testing.T) {
	classID := uuid.New()
	actorID := uuid.New()

	t.Run("admin bypass không cần gọi repo", func(t *testing.T) {
		s := &ClassService{classRepo: nil} // nil repo -> nếu code lỡ gọi repo sẽ panic, test tự fail
		if err := s.requireClassTeacherOrAdmin(context.Background(), classID, actorID, true); err != nil {
			t.Errorf("expected nil error for admin, got %v", err)
		}
	})

	t.Run("giáo viên của lớp được phép", func(t *testing.T) {
		s := &ClassService{classRepo: &fakeClassRepoForTeacherCheck{teacherClassExists: true}}
		if err := s.requireClassTeacherOrAdmin(context.Background(), classID, actorID, false); err != nil {
			t.Errorf("expected nil error for class teacher, got %v", err)
		}
	})

	t.Run("không phải giáo viên lớp bị từ chối", func(t *testing.T) {
		s := &ClassService{classRepo: &fakeClassRepoForTeacherCheck{teacherClassExists: false}}
		err := s.requireClassTeacherOrAdmin(context.Background(), classID, actorID, false)
		if !errors.Is(err, ErrNotClassTeacher) {
			t.Errorf("expected ErrNotClassTeacher, got %v", err)
		}
	})

	t.Run("lỗi repo được propagate, không nuốt", func(t *testing.T) {
		wantErr := errors.New("db down")
		s := &ClassService{classRepo: &fakeClassRepoForTeacherCheck{err: wantErr}}
		err := s.requireClassTeacherOrAdmin(context.Background(), classID, actorID, false)
		if !errors.Is(err, wantErr) {
			t.Errorf("expected repo error to propagate, got %v", err)
		}
	})
}
