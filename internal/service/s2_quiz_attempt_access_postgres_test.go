package service

// Lane S2, lỗi 3: bài làm/kết quả quiz chỉ dành cho chính chủ, giảng viên chủ khoá của quiz,
// phụ huynh liên kết ACTIVE và admin; người khác nhận ErrQuizAttemptNotFound / ErrQuizResultsNotFound
// (handler dịch thành 404 để không dò được sự tồn tại). Bỏ kiểm tra trong GetAttemptByID /
// GetQuizResults / GetQuizStatistics thì các test này ĐỎ.

import (
	"context"
	"errors"
	"testing"

	"study.com/v1/internal/model"
	"study.com/v1/internal/repository"
)

type s2QuizWorld struct {
	svc                                        *QuizService
	quiz                                       model.Quiz
	attempt                                    model.QuizAttempt
	student, otherStudent                      model.User
	teacher, otherTeacher                      model.User
	activeParent, pendingParent, revokedParent model.User
	admin                                      model.User
}

func newS2QuizWorld(t *testing.T) *s2QuizWorld {
	t.Helper()
	f := newS2Fixture(t)
	w := &s2QuizWorld{
		student: f.user("student"), otherStudent: f.user("student2"),
		teacher: f.user("teacher"), otherTeacher: f.user("teacher2"),
		activeParent: f.user("parent-active"), pendingParent: f.user("parent-pending"),
		revokedParent: f.user("parent-revoked"), admin: f.user("admin"),
	}
	course := f.course(w.teacher)
	// Quiz do CHÍNH ADMIN tạo (không phải giảng viên): giảng viên chủ khoá vẫn phải xem được — đúng
	// yêu cầu "giảng viên chủ khoá", không phụ thuộc created_by.
	w.quiz = model.Quiz{CourseID: &course.ID, CreatedBy: &w.admin.ID, Title: "QA-s2 quiz"}
	if err := f.db.Create(&w.quiz).Error; err != nil {
		t.Fatalf("tạo quiz: %v", err)
	}
	w.attempt = model.QuizAttempt{UserID: w.student.ID, QuizID: w.quiz.ID}
	if err := f.db.Create(&w.attempt).Error; err != nil {
		t.Fatalf("tạo attempt: %v", err)
	}
	for parent, status := range map[*model.User]string{
		&w.activeParent: model.ParentStudentRelationStatusActive, &w.pendingParent: "pending", &w.revokedParent: "revoked",
	} {
		rel := model.ParentStudentRelation{ParentUserID: parent.ID, StudentUserID: w.student.ID, Relationship: "parent", Status: status}
		if err := f.db.Create(&rel).Error; err != nil {
			t.Fatalf("tạo quan hệ phụ huynh %s: %v", status, err)
		}
	}
	w.svc = NewQuizService(repository.NewQuizRepository(f.db), nil, repository.NewCourseRepository(f.db),
		repository.NewSectionRepository(f.db), repository.NewLessonRepository(f.db), nil, repository.NewEnrollmentRepository(f.db))
	w.svc.SetParentLinkChecker(repository.NewParentStudentRepository(f.db))
	return w
}

func TestS2_QuizAttempt_ChiNguoiCoQuyenXemDuoc(t *testing.T) {
	w := newS2QuizWorld(t)
	ctx := context.Background()

	allowed := map[string]struct {
		id      model.User
		isAdmin bool
	}{
		"chính chủ":           {w.student, false},
		"giảng viên chủ khoá": {w.teacher, false},
		"phụ huynh active":    {w.activeParent, false},
		"admin":               {w.admin, true},
	}
	for name, who := range allowed {
		if _, err := w.svc.GetAttemptByID(ctx, w.attempt.ID, who.id.ID, who.isAdmin); err != nil {
			t.Errorf("%s phải xem được bài làm, nhận lỗi %v", name, err)
		}
	}

	denied := map[string]model.User{
		"học viên khác":               w.otherStudent,
		"giảng viên khác":             w.otherTeacher,
		"phụ huynh chưa xác nhận":     w.pendingParent,
		"phụ huynh đã bị gỡ liên kết": w.revokedParent,
	}
	for name, u := range denied {
		_, err := w.svc.GetAttemptByID(ctx, w.attempt.ID, u.ID, false)
		if !errors.Is(err, ErrQuizAttemptNotFound) {
			t.Errorf("%s KHÔNG được xem bài làm; muốn ErrQuizAttemptNotFound (=> 404), nhận %v", name, err)
		}
	}
}

// Bài làm không tồn tại và bài làm của người khác phải cho CÙNG một lỗi — nếu khác nhau thì kẻ
// dò ID phân biệt được "có bài làm này" với "không có".
func TestS2_QuizAttempt_KhongPhanBietKhongTonTaiVaKhongCoQuyen(t *testing.T) {
	w := newS2QuizWorld(t)
	ctx := context.Background()
	missing := model.QuizAttempt{}
	missing.ID = w.attempt.ID
	missing.ID[0] ^= 0xff

	_, errMissing := w.svc.GetAttemptByID(ctx, missing.ID, w.otherStudent.ID, false)
	_, errForeign := w.svc.GetAttemptByID(ctx, w.attempt.ID, w.otherStudent.ID, false)
	if !errors.Is(errMissing, ErrQuizAttemptNotFound) || !errors.Is(errForeign, ErrQuizAttemptNotFound) {
		t.Fatalf("hai trường hợp phải cùng ErrQuizAttemptNotFound, nhận %v / %v", errMissing, errForeign)
	}
}

func TestS2_QuizResults_ChiNguoiQuanLyQuizVaAdmin(t *testing.T) {
	w := newS2QuizWorld(t)
	ctx := context.Background()

	if _, err := w.svc.GetQuizResults(ctx, w.quiz.ID, w.teacher.ID, false); err != nil {
		t.Errorf("giảng viên chủ khoá phải xem được kết quả cả quiz: %v", err)
	}
	if _, err := w.svc.GetQuizResults(ctx, w.quiz.ID, w.admin.ID, true); err != nil {
		t.Errorf("admin phải xem được kết quả cả quiz: %v", err)
	}
	if _, err := w.svc.GetQuizStatistics(ctx, w.quiz.ID, w.teacher.ID, false); err != nil {
		t.Errorf("giảng viên chủ khoá phải xem được thống kê: %v", err)
	}
	for name, u := range map[string]model.User{"học viên (kể cả người đã làm bài)": w.student, "học viên khác": w.otherStudent, "giảng viên khác": w.otherTeacher, "phụ huynh": w.activeParent} {
		if _, err := w.svc.GetQuizResults(ctx, w.quiz.ID, u.ID, false); !errors.Is(err, ErrQuizResultsNotFound) {
			t.Errorf("%s KHÔNG được xem kết quả cả quiz; muốn ErrQuizResultsNotFound, nhận %v", name, err)
		}
		if _, err := w.svc.GetQuizStatistics(ctx, w.quiz.ID, u.ID, false); !errors.Is(err, ErrQuizResultsNotFound) {
			t.Errorf("%s KHÔNG được xem thống kê cả quiz; muốn ErrQuizResultsNotFound, nhận %v", name, err)
		}
	}
}
