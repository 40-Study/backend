package router

import (
	"github.com/gofiber/fiber/v2"
	"study.com/v1/internal/handler"
)

func SetupTeacherRoutes(
	api fiber.Router,
	teacherHandler *handler.TeacherHandler,
) {
	teachers := api.Group("/teachers")
	{
		teachers.Get("/", teacherHandler.GetAllTeachers)
		teachers.Get("/:id", teacherHandler.GetTeacher)
		teachers.Delete("/:id", teacherHandler.DeleteTeacher)
	}
}
