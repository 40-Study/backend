package router

import (
	"github.com/gofiber/fiber/v2"
	"github.com/redis/go-redis/v9"
	"study.com/v1/internal/config"
	"study.com/v1/internal/handler"
	"study.com/v1/internal/middleware"
)

func SetupClassRoutes(
	api fiber.Router,
	cfg *config.Config,
	classHandler *handler.ClassHandler,
	attendanceHandler *handler.AttendanceHandler,
	redis *redis.Client,
) {
	classes := api.Group("/classes", middleware.AuthMiddleware(cfg, redis))
	{
		classes.Post("/", classHandler.CreateClass)
		classes.Get("/", classHandler.GetAllClasses)
		classes.Get("/me", classHandler.GetMyClasses)
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
