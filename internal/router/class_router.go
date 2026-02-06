package router

import (
	"github.com/gofiber/fiber/v2"
	"study.com/v1/internal/handler"
)

func SetupClassRoutes(
	api fiber.Router,
	classHandler *handler.ClassHandler,
	classScheduleHandler *handler.ClassScheduleHandler,
	attendanceHandler *handler.AttendanceHandler,
) {
	classes := api.Group("/classes")
	{
		classes.Post("/", classHandler.Create)
		classes.Get("/", classHandler.GetAll)
		classes.Get("/:id", classHandler.GetByID)
		classes.Put("/:id", classHandler.Update)
		classes.Delete("/:id", classHandler.Delete)

		// Teacher-Class
		classes.Post("/:id/teachers", classHandler.AssignTeacher)
		classes.Delete("/:id/teachers/:teacherId", classHandler.RemoveTeacher)
		classes.Get("/:id/teachers", classHandler.GetTeachers)

		// Student-Class
		classes.Post("/:id/students", classHandler.EnrollStudent)
		classes.Delete("/:id/students/:studentId", classHandler.RemoveStudent)
		classes.Get("/:id/students", classHandler.GetStudents)

		// Schedules
		schedules := classes.Group("/:classId/schedules")
		{
			schedules.Post("/", classScheduleHandler.Create)
			schedules.Get("/", classScheduleHandler.GetAll)
			schedules.Put("/:id", classScheduleHandler.Update)
			schedules.Delete("/:id", classScheduleHandler.Delete)
		}

		// Attendances
		attendances := classes.Group("/:classId/attendances")
		{
			attendances.Post("/", attendanceHandler.MarkAttendance)
			attendances.Get("/", attendanceHandler.GetAll)
			attendances.Get("/:id", attendanceHandler.GetByID)
			attendances.Put("/:id", attendanceHandler.Update)
			attendances.Delete("/:id", attendanceHandler.Delete)
		}
	}
}
