package handler

// Test cho SEC-1 (F4 review 260917), tang handler: service.ErrLessonLocked tu cac endpoint quiz phai
// thanh 403 {message:"LESSON_LOCKED"} — cung shape voi LessonContentHandler.GetContent — chu khong
// roi vao nhanh 404/400/500 chung. Mot 404 "quiz not found" se de web hieu nham la quiz bien mat.
//
// mountWithCaller/doJSON dung lai tu livestream_authz_handler_test.go (cung package).

import (
	"context"
	"testing"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"study.com/v1/internal/dto"
	"study.com/v1/internal/service"
)

// stubLockedQuizService: method khong cai dat thuoc interface nhung (nil) se panic neu handler goi nham.
type stubLockedQuizService struct {
	service.QuizServiceInterface
	err error
}

func (s *stubLockedQuizService) GetQuizByID(ctx context.Context, id, userID uuid.UUID, isAdmin bool) (*dto.QuizDetailDTO, error) {
	return nil, s.err
}

func (s *stubLockedQuizService) GetQuestionsByQuiz(ctx context.Context, quizID, userID uuid.UUID, isAdmin bool) ([]dto.QuestionResponseDTO, error) {
	return nil, s.err
}

func (s *stubLockedQuizService) StartQuiz(ctx context.Context, quizID, userID uuid.UUID, isAdmin bool, req dto.StartQuizDTO) (*dto.StartQuizResponseDTO, error) {
	return nil, s.err
}

func TestQuizEndpoints_BaiKhoa_Tra403LessonLocked(t *testing.T) {
	h := NewQuizHandler(&stubLockedQuizService{err: service.ErrLessonLocked}, nil)
	quizID := uuid.NewString()
	cases := []struct {
		method, route, path string
		handler             fiber.Handler
	}{
		{"GET", "/quizzes/:id", "/quizzes/" + quizID, h.GetQuizByID},
		{"GET", "/quizzes/:quizId/questions", "/quizzes/" + quizID + "/questions", h.GetQuestionsByQuiz},
		{"POST", "/quizzes/:id/start", "/quizzes/" + quizID + "/start", h.StartQuiz},
	}
	for _, tc := range cases {
		app := mountWithCaller(tc.method, tc.route, uuid.New(), tc.handler)
		if code := doJSON(t, app, tc.method, tc.path, ""); code != 403 {
			t.Errorf("%s %s: bai khoa phai tra 403, nhan %d", tc.method, tc.path, code)
		}
	}
}
