package handler

// Test cho QA S7 (261008), tầng handler: service.ErrQuizNotFound từ mọi route quiz đi qua
// respondQuizGateError phải thành 404, không rơi vào nhánh 500/400 chung.

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"study.com/v1/internal/dto"
	"study.com/v1/internal/service"
)

type stubQuizNotFoundService struct {
	service.QuizServiceInterface
	err error
}

func (s *stubQuizNotFoundService) GetMyAttempts(ctx context.Context, quizID, userID uuid.UUID, isAdmin bool) ([]dto.QuizAttemptResponseDTO, error) {
	return nil, s.err
}

func TestGetMyAttempts_QuizKhongTonTai_Tra404(t *testing.T) {
	h := NewQuizHandler(&stubQuizNotFoundService{err: service.ErrQuizNotFound}, nil)
	app := mountWithCaller("GET", "/quizzes/:id/attempts", uuid.New(), h.GetMyAttempts)
	path := "/quizzes/" + uuid.NewString() + "/attempts"
	if code := doJSON(t, app, "GET", path, ""); code != 404 {
		t.Errorf("GET %s với quiz không tồn tại: muốn 404, nhận %d", path, code)
	}
}
