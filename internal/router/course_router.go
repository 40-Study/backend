package router

import (
	"github.com/gofiber/fiber/v2"
	"github.com/redis/go-redis/v9"
	"study.com/v1/internal/config"
	"study.com/v1/internal/handler"
	"study.com/v1/internal/middleware"
)

func SetupCourseRoutes(
	api fiber.Router,
	cfg *config.Config,
	courseHandler *handler.CourseHandler,
	sectionHandler *handler.SectionHandler,
	lessonHandler *handler.LessonHandler,
	redis *redis.Client,
) {
	auth := middleware.AuthMiddleware(cfg, redis)

	// Courses - public read, auth required for write
	courses := api.Group("/courses")
	{
		// Public routes
		courses.Get("/", courseHandler.GetAllCourses)
		courses.Get("/:id", courseHandler.GetCourseByID)

		// Protected routes - require authentication
		courses.Post("/", auth, courseHandler.CreateCourse)
		courses.Put("/:id", auth, courseHandler.UpdateCourse)
		courses.Delete("/:id", auth, courseHandler.DeleteCourse)

		// Sections (nested under course) - all protected
		sections := courses.Group("/:courseId/sections", auth)
		{
			sections.Post("/", sectionHandler.CreateSection)
			sections.Get("/", sectionHandler.GetAllSections)
			sections.Put("/reorder", sectionHandler.ReorderSections)
			sections.Put("/:id", sectionHandler.UpdateSection)
			sections.Delete("/:id", sectionHandler.DeleteSection)

			// Lessons (nested under section) - all protected
			lessons := sections.Group("/:sectionId/lessons")
			{
				lessons.Post("/", lessonHandler.CreateLesson)
				lessons.Get("/", lessonHandler.GetAllLessons)
				lessons.Put("/reorder", lessonHandler.ReorderLessons)
				lessons.Put("/:id", lessonHandler.UpdateLesson)
				lessons.Delete("/:id", lessonHandler.DeleteLesson)
			}
		}
	}
}
