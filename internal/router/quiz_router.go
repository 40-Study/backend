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
		questions.Put("/:id", quizHandler.UpdateQuestion)
		questions.Delete("/:id", quizHandler.DeleteQuestion)
		questions.Put("/reorder", quizHandler.ReorderQuestions)
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
	// MY QUIZZES
	// ============================================================================
	api.Get("/me/quizzes", auth, quizHandler.GetMyCreatedQuizzes)
	api.Get("/me/quiz-history", auth, quizHandler.GetMyQuizHistory)
}
