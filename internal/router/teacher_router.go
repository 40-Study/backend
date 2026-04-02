package router

import (
	"github.com/gofiber/fiber/v2"
	"github.com/redis/go-redis/v9"
	"study.com/v1/internal/config"
	"study.com/v1/internal/handler"
	"study.com/v1/internal/middleware"
)

func SetupTeacherRoutes(
	api fiber.Router,
	cfg *config.Config,
	teacherHandler *handler.TeacherHandler,
	redis *redis.Client,
) {
	teachers := api.Group("/teachers")
	{
		teachers.Get("/", teacherHandler.GetAllTeachers)
		teachers.Get("/me/students", middleware.AuthMiddleware(cfg, redis), teacherHandler.GetMyStudents)
		teachers.Get("/:id", teacherHandler.GetTeacher)
		teachers.Delete("/:id", teacherHandler.DeleteTeacher)
	}
}
