package router

import (
	"github.com/gofiber/fiber/v2"
	"study.com/v1/internal/handler"
)

func SetupCourseRoutes(
	api fiber.Router,
	courseHandler *handler.CourseHandler,
	sectionHandler *handler.SectionHandler,
	lessonHandler *handler.LessonHandler,
) {
	// Courses
	courses := api.Group("/courses")
	{
		courses.Post("/", courseHandler.CreateCourse)
		courses.Get("/", courseHandler.GetAllCourses)
		courses.Get("/:id", courseHandler.GetCourseByID)
		courses.Put("/:id", courseHandler.UpdateCourse)
		courses.Delete("/:id", courseHandler.DeleteCourse)

		// Sections (nested under course)
		sections := courses.Group("/:courseId/sections")
		{
			sections.Post("/", sectionHandler.CreateSection)
			sections.Get("/", sectionHandler.GetAllSections)
			sections.Put("/reorder", sectionHandler.ReorderSections)
			sections.Put("/:id", sectionHandler.UpdateSection)
			sections.Delete("/:id", sectionHandler.DeleteSection)

			// Lessons (nested under section)
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
