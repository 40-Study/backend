package service

// Test cho QA S7 (261008): GET /quizzes/:id/attempts với quiz không tồn tại trước đây trả 500.
// Nguyên nhân: checkStandaloneQuizReader / checkQuizOwner trả errors.New("quiz not found") (lỗi
// trần, handler không nhận ra) nên rơi vào nhánh 500. Giờ phải là sentinel ErrQuizNotFound để
// handler ánh xạ 404.
//
// Mutation muốn bắt: đổi errors.New("quiz not found") trở lại ở hai hàm check trên phải làm các
// test này ĐỎ.

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"study.com/v1/internal/model"
	"study.com/v1/internal/repository"
)

// repoQuizAbsent: GetQuizByID giả lập hợp đồng repo thật — không có dòng thì trả (nil, nil).
type repoQuizAbsent struct {
	repository.QuizRepositoryInterface
}

func (repoQuizAbsent) GetQuizByID(ctx context.Context, id uuid.UUID) (*model.Quiz, error) {
	return nil, nil
}

func TestGetMyAttempts_QuizKhongTonTai_TraErrQuizNotFound(t *testing.T) {
	s := NewQuizService(repoQuizAbsent{}, nil, nil, nil, nil, nil, nil)
	_, err := s.GetMyAttempts(context.Background(), uuid.New(), uuid.New(), false)
	if !errors.Is(err, ErrQuizNotFound) {
		t.Fatalf("GetMyAttempts(quiz không tồn tại): muốn ErrQuizNotFound, nhận %v", err)
	}
}

func TestCheckQuizOwner_QuizKhongTonTai_TraErrQuizNotFound(t *testing.T) {
	s := NewQuizService(repoQuizAbsent{}, nil, nil, nil, nil, nil, nil)
	err := s.checkQuizOwner(context.Background(), uuid.New(), uuid.New(), false)
	if !errors.Is(err, ErrQuizNotFound) {
		t.Fatalf("checkQuizOwner(quiz không tồn tại): muốn ErrQuizNotFound, nhận %v", err)
	}
}
