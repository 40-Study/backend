package service

// Lane S2, m4: quiz cuộc thi không được lộ "attempt này có tồn tại" qua khác biệt 403/404.
// Người không có quyền xem attempt của người khác nhận 404 CẢ khi attempt có thật lẫn khi id bịa;
// 403 QUIZ_LOCKED_BY_CONTEST chỉ còn cho CHÍNH CHỦ attempt khi quiz còn khoá. Postgres thật.

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"study.com/v1/internal/dto"
)

func TestContestQuizAttempt_NguoiKhacKhongDoDuocAttemptCoThat(t *testing.T) {
	f := newContestFixture(t)
	ctx := context.Background()
	creator, student, stranger := f.user("creator"), f.user("student"), f.user("stranger")
	cq := f.standaloneQuiz(creator, false)
	f.quiz.SetContestGate(&fakeContestGate{locked: map[uuid.UUID]fakeContestLock{cq.ID: {creator: creator}}})
	attemptID := f.startContestAttempt(cq.ID, student)

	// Nhánh 1: người khác, attempt thật và id bịa PHẢI cùng lỗi 404.
	_, errReal := f.quiz.GetAttemptByID(ctx, attemptID, stranger, false)
	_, errFake := f.quiz.GetAttemptByID(ctx, uuid.New(), stranger, false)
	if !errors.Is(errReal, ErrQuizAttemptNotFound) || !errors.Is(errFake, ErrQuizAttemptNotFound) {
		t.Fatalf("người khác: attempt thật %v / id bịa %v, cả hai phải là ErrQuizAttemptNotFound", errReal, errFake)
	}
	if _, err := f.quiz.GetAttemptProgress(ctx, attemptID, stranger, false); !errors.Is(err, ErrQuizAttemptNotFound) {
		t.Errorf("GetAttemptProgress của người khác: muốn ErrQuizAttemptNotFound, nhận %v", err)
	}

	// Lưu đáp án vào attempt người khác cũng không phân biệt được thật/bịa.
	save := dto.SaveAnswerDTO{QuestionID: cq.SingleQ.String()}
	errSaveReal := f.quiz.SaveAnswer(ctx, attemptID, stranger, false, save)
	errSaveFake := f.quiz.SaveAnswer(ctx, uuid.New(), stranger, false, save)
	if !errors.Is(errSaveReal, ErrQuizAttemptNotFound) || !errors.Is(errSaveFake, ErrQuizAttemptNotFound) {
		t.Errorf("SaveAnswer: attempt thật %v / id bịa %v, cả hai phải là ErrQuizAttemptNotFound", errSaveReal, errSaveFake)
	}

	// Nhánh 2: CHÍNH CHỦ attempt khi quiz còn khoá cuộc thi vẫn nhận 403 QUIZ_LOCKED_BY_CONTEST.
	if _, err := f.quiz.GetAttemptByID(ctx, attemptID, student, false); !errors.Is(err, ErrQuizLockedByContest) {
		t.Errorf("chủ attempt khi quiz còn khoá: muốn ErrQuizLockedByContest, nhận %v", err)
	}
}
