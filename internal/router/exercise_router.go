package router

import (
	"github.com/gofiber/fiber/v2"
	"github.com/redis/go-redis/v9"
	"study.com/v1/internal/config"
	"study.com/v1/internal/handler"
	"study.com/v1/internal/middleware"
)

func SetupExerciseRoutes(
	api fiber.Router,
	cfg *config.Config,
	exerciseHandler *handler.ExerciseHandler,
	redis *redis.Client,
) {
	auth := middleware.AuthMiddleware(cfg, redis)

	// ============================================================================
	// EXERCISE CRUD
	// ============================================================================
	exercises := api.Group("/exercises", auth)
	{
		exercises.Post("/", exerciseHandler.CreateExercise)
		exercises.Get("/", exerciseHandler.ListExercises)
		exercises.Get("/:id", exerciseHandler.GetExerciseByID)
		exercises.Put("/:id", exerciseHandler.UpdateExercise)
		exercises.Delete("/:id", exerciseHandler.DeleteExercise)
	}

	// ============================================================================
	// TEST CASES
	// ============================================================================
	exercises.Get("/:id/testcases", exerciseHandler.GetTestCases)
	exercises.Post("/:id/testcases", exerciseHandler.CreateTestCase)
	exercises.Post("/:id/testcases/import", exerciseHandler.ImportTestCases)
	exercises.Delete("/:id/testcases/:testCaseId", exerciseHandler.DeleteTestCase)

	// ============================================================================
	// SUBMISSIONS
	// ============================================================================
	exercises.Post("/:id/submit", exerciseHandler.SubmitExercise)
	exercises.Get("/:id/submissions", exerciseHandler.GetSubmissions)
	exercises.Get("/:id/my-submissions", exerciseHandler.GetMySubmissions)

	api.Get("/submissions/:submissionId", auth, exerciseHandler.GetSubmissionByID)

	// ============================================================================
	// CONTENT PROGRESS
	// ============================================================================
	api.Post("/content-progress", auth, exerciseHandler.UpdateContentProgress)
	api.Get("/content-progress/:lessonContentId", auth, exerciseHandler.GetContentProgress)
}
