package router

import (
	"github.com/gofiber/fiber/v2"
	"github.com/redis/go-redis/v9"
	"study.com/v1/internal/config"
	"study.com/v1/internal/handler"
	"study.com/v1/internal/middleware"
)

func SetupQuizRoutes(
	api fiber.Router,
	cfg *config.Config,
	quizHandler *handler.QuizHandler,
	redis *redis.Client,
) {
	auth := middleware.AuthMiddleware(cfg, redis)

	// ============================================================================
	// QUIZ CRUD
	// ============================================================================
	quizzes := api.Group("/quizzes", auth)
	{
		quizzes.Post("/", quizHandler.CreateQuiz)
		quizzes.Get("/", quizHandler.GetAllQuizzes)
		quizzes.Get("/:id", quizHandler.GetQuizByID)
		quizzes.Put("/:id", quizHandler.UpdateQuiz)
		quizzes.Delete("/:id", quizHandler.DeleteQuiz)
		quizzes.Post("/:id/duplicate", quizHandler.DuplicateQuiz)
	}

	// ============================================================================
	// QUESTIONS
	// ============================================================================
	questions := api.Group("/quizzes/:quizId/questions", auth)
	{
		questions.Post("/", quizHandler.CreateQuestion)
		questions.Get("/", quizHandler.GetQuestionsByQuiz)
		// M3-01 (review vòng 4): Fiber khớp route theo THỨ TỰ ĐĂNG KÝ — PUT /reorder đăng ký
		// TRƯỚC PUT /:id (cùng lỗi MEDIUM-11 đã sửa ở grade_router.go, quét ra còn sót ở đây).
		questions.Put("/reorder", quizHandler.ReorderQuestions)
		questions.Put("/:id", quizHandler.UpdateQuestion)
		questions.Delete("/:id", quizHandler.DeleteQuestion)
		questions.Post("/bulk", quizHandler.BulkCreateQuestions)
	}

	// ============================================================================
	// QUIZ ATTEMPTS
	// ============================================================================
	api.Post("/quizzes/:id/start", auth, quizHandler.StartQuiz)
	api.Post("/quizzes/:id/submit", auth, quizHandler.SubmitQuiz)
	api.Get("/quizzes/:id/attempts", auth, quizHandler.GetMyAttempts)
	api.Get("/quizzes/:id/attempts/:attemptId", auth, quizHandler.GetAttemptByID)
	api.Get("/quizzes/:id/results", auth, quizHandler.GetQuizResults)
	api.Get("/quizzes/:id/statistics", auth, quizHandler.GetQuizStatistics)

	// ============================================================================
	// QUIZ IN PROGRESS
	// ============================================================================
	api.Post("/attempts/:attemptId/save-answer", auth, quizHandler.SaveAnswer)
	api.Get("/attempts/:attemptId/progress", auth, quizHandler.GetAttemptProgress)

	// ============================================================================
	// QUIZ THEO BAI HOC
	// ============================================================================
	// Web goi dung duong dan nay (services/quiz.service.ts getByLesson va
	// lib/server-fetchers/curriculum.ts). Thieu no thi moi bai hoc nhan 404 va
	// quiz khong bao gio tai duoc trong trinh phat. Tra 200 [] khi bai khong co quiz.
	api.Get("/lessons/:lessonId/quizzes", auth, quizHandler.GetQuizzesByLesson)

	// ============================================================================
	// MY QUIZZES
	// ============================================================================
	api.Get("/me/quizzes", auth, quizHandler.GetMyCreatedQuizzes)
	api.Get("/me/quiz-history", auth, quizHandler.GetMyQuizHistory)
}
