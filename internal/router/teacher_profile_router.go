package router

import (
	"github.com/gofiber/fiber/v2"
	"study.com/v1/internal/handler"
)

func SetupTeacherProfileRoutes(
	api fiber.Router,
	teacherProfileHandler *handler.TeacherProfileHandler,
) {
	profiles := api.Group("/teacher-profiles")
	{
		profiles.Post("/", teacherProfileHandler.Create)
		profiles.Get("/", teacherProfileHandler.GetAll)
		profiles.Get("/:id", teacherProfileHandler.GetByID)
		profiles.Put("/:id", teacherProfileHandler.Update)
		profiles.Delete("/:id", teacherProfileHandler.Delete)
	}
}
