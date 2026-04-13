package router

import (
	"github.com/gofiber/fiber/v2"
	"github.com/redis/go-redis/v9"
	"study.com/v1/internal/config"
	"study.com/v1/internal/handler"
	"study.com/v1/internal/middleware"
)

func SetupEnrollmentRoutes(
	api fiber.Router,
	cfg *config.Config,
	enrollmentHandler *handler.EnrollmentHandler,
	redis *redis.Client,
) {
	auth := middleware.AuthMiddleware(cfg, redis)

	// Enroll/Unenroll under courses
	courses := api.Group("/courses")
	{
		courses.Post("/:courseId/enroll", auth, enrollmentHandler.Enroll)
		courses.Delete("/:courseId/enroll", auth, enrollmentHandler.Unenroll)
		courses.Get("/:courseId/enrollments", auth, enrollmentHandler.GetCourseEnrollments)
		courses.Get("/:courseId/enrollments/debug", auth, enrollmentHandler.DebugCourseEnrollments)
	}

	// My enrollments
	enrollments := api.Group("/enrollments", auth)
	{
		enrollments.Get("/", enrollmentHandler.GetMyEnrollments)
		enrollments.Get("/:id", enrollmentHandler.GetEnrollmentDetail)
	}

	// Lesson progress
	lessons := api.Group("/lessons", auth)
	{
		lessons.Put("/:lessonId/progress", enrollmentHandler.UpdateLessonProgress)
	}
}
