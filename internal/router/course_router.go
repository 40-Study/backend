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
	lessonContentHandler *handler.LessonContentHandler,
	classHandler *handler.ClassHandler,
	classLessonContentHandler *handler.ClassLessonContentHandler,
	attendanceHandler *handler.AttendanceHandler,
	redis *redis.Client,
) {
	auth := middleware.AuthMiddleware(cfg, redis)
	optionalAuth := middleware.OptionalAuthMiddleware(cfg, redis)

	// Courses - public read, auth required for write
	courses := api.Group("/courses")
	{
		// Public routes (with optional auth for filtering by instructor)
		courses.Get("/", optionalAuth, courseHandler.GetAllCourses)
		courses.Get("/slug/:slug", optionalAuth, courseHandler.GetCourseBySlug)
		courses.Get("/:id", optionalAuth, courseHandler.GetCourseByID)

		// Protected routes - require authentication
		courses.Post("/", auth, courseHandler.CreateCourse)
		courses.Put("/:id", auth, courseHandler.UpdateCourse)
		courses.Delete("/:id", auth, courseHandler.DeleteCourse)

		// Classes under courses
		classes := courses.Group("/:course_id/classes", auth)
		{
			classes.Get("/", classHandler.GetClassesByCourseID)
			classes.Post("/", classHandler.CreateClassForCourse)
			classes.Get("/:id", classHandler.GetClassByID)
			classes.Put("/:id", classHandler.UpdateClass)
			classes.Delete("/:id", classHandler.DeleteClass)

			// Teacher-Class
			classes.Post("/:id/teachers", classHandler.AssignTeacherToClass)
			classes.Delete("/:id/teachers/:teacherId", classHandler.RemoveTeacherFromClass)
			classes.Get("/:id/teachers", classHandler.GetTeachersByClass)

			// Student-Class
			classes.Post("/:id/students", classHandler.EnrollStudentToClass)
			classes.Delete("/:id/students/:studentId", classHandler.RemoveStudentFromClass)
			classes.Get("/:id/students", classHandler.GetStudentsByClass)

			// Content schedule for class
			classes.Get("/:id/contents", classLessonContentHandler.GetContentScheduleForClass)

			// Attendances
			attendances := classes.Group("/:classId/attendances")
			{
				attendances.Post("/", attendanceHandler.MarkAttendance)
				attendances.Get("/", attendanceHandler.GetAllAttendances)
				attendances.Get("/:id", attendanceHandler.GetAttendanceByID)
				attendances.Put("/:id", attendanceHandler.UpdateAttendance)
				attendances.Delete("/:id", attendanceHandler.DeleteAttendance)
			}
		}

		// Sections under courses
		sections := courses.Group("/:course_id/sections", auth)
		{
			sections.Post("/", sectionHandler.CreateSection)
			sections.Get("/", sectionHandler.GetAllSections)
			sections.Get("/:id", sectionHandler.GetSectionByID)
			sections.Put("/reorder", sectionHandler.ReorderSections)
			sections.Put("/:id", sectionHandler.UpdateSection)
			sections.Delete("/:id", sectionHandler.DeleteSection)
		}
	}

	// Sections - flattened routes for lessons
	sectionLessons := api.Group("/sections/:section_id/lessons", auth)
	{
		sectionLessons.Post("/", lessonHandler.CreateLesson)
		sectionLessons.Get("/", lessonHandler.GetAllLessons)
		sectionLessons.Put("/reorder", lessonHandler.ReorderLessons)
	}

	// Lessons - flattened routes
	lessons := api.Group("/lessons", auth)
	{
		lessons.Get("/:id", lessonHandler.GetLessonByID)
		lessons.Put("/:id", lessonHandler.UpdateLesson)
		lessons.Delete("/:id", lessonHandler.DeleteLesson)

		// Contents under lessons
		contents := lessons.Group("/:lesson_id/contents")
		{
			contents.Post("/", lessonContentHandler.CreateContent)
			contents.Get("/", lessonContentHandler.GetContent)
			contents.Put("/reorder", lessonContentHandler.ReorderContents)
			contents.Put("/:id", lessonContentHandler.UpdateContent)
			contents.Delete("/:id", lessonContentHandler.DeleteContent)
		}
	}

	// ClassLessonContent routes - assign classes to lesson contents with scheduling
	lessonContents := api.Group("/lesson-contents", auth)
	{
		lessonContents.Post("/:id/classes/bulk", classLessonContentHandler.BulkAssignClassesToContent)
		lessonContents.Post("/:id/classes", classLessonContentHandler.AssignClassToContent)
		lessonContents.Get("/:id/classes", classLessonContentHandler.GetClassesForContent)
		lessonContents.Put("/:id/classes/:class_id", classLessonContentHandler.UpdateClassContentSchedule)
		lessonContents.Delete("/:id/classes/:class_id", classLessonContentHandler.RemoveClassFromContent)
	}
}
