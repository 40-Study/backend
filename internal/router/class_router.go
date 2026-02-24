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
		classes.Post("/", classHandler.CreateClass)
		classes.Get("/", classHandler.GetAllClasses)
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

		// Schedules
		schedules := classes.Group("/:classId/schedules")
		{
			schedules.Post("/", classScheduleHandler.CreateClassSchedule)
			schedules.Get("/", classScheduleHandler.GetAllClassSchedules)
			schedules.Put("/:id", classScheduleHandler.UpdateClassSchedule)
			schedules.Delete("/:id", classScheduleHandler.DeleteClassSchedule)
		}

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
}
