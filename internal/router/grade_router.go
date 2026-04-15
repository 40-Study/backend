package router

import (
	"github.com/gofiber/fiber/v2"
	"github.com/redis/go-redis/v9"
	"study.com/v1/internal/config"
	"study.com/v1/internal/handler"
	"study.com/v1/internal/middleware"
)

func SetupGradeRoutes(
	api fiber.Router,
	cfg *config.Config,
	gradeHandler *handler.GradeHandler,
	redis *redis.Client,
) {
	auth := middleware.AuthMiddleware(cfg, redis)

	// ============================================================================
	// GRADE COLUMNS
	// ============================================================================
	gradeColumns := api.Group("/classes/:classId/grade-columns", auth)
	{
		gradeColumns.Post("/", gradeHandler.CreateGradeColumn)
		gradeColumns.Get("/", gradeHandler.GetGradeColumns)
		gradeColumns.Put("/:id", gradeHandler.UpdateGradeColumn)
		gradeColumns.Delete("/:id", gradeHandler.DeleteGradeColumn)
		gradeColumns.Put("/reorder", gradeHandler.ReorderGradeColumns)
	}

	// ============================================================================
	// GRADES
	// ============================================================================
	classGrades := api.Group("/classes/:classId/grades", auth)
	{
		classGrades.Post("/", gradeHandler.CreateGrade)
		classGrades.Get("/", gradeHandler.GetGradesByClass)
		classGrades.Get("/student/:studentId", gradeHandler.GetStudentGrades)
		classGrades.Post("/bulk", gradeHandler.BulkCreateGrades)
	}

	grades := api.Group("/grades", auth)
	{
		grades.Put("/:id", gradeHandler.UpdateGrade)
		grades.Delete("/:id", gradeHandler.DeleteGrade)
	}

	// ============================================================================
	// FINAL GRADES
	// ============================================================================
	finalGrades := api.Group("/classes/:classId/final-grades", auth)
	{
		finalGrades.Post("/calculate", gradeHandler.CalculateFinalGrades)
		finalGrades.Get("/", gradeHandler.GetFinalGrades)
		finalGrades.Put("/:id", gradeHandler.UpdateFinalGrade)
		finalGrades.Post("/finalize", gradeHandler.FinalizeFinalGrades)
	}

	// ============================================================================
	// MY GRADES
	// ============================================================================
	api.Get("/me/grades", auth, gradeHandler.GetMyGrades)
	api.Get("/me/grades/class/:classId", auth, gradeHandler.GetMyGradesByClass)
}
