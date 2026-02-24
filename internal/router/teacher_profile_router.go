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
		profiles.Post("/", teacherProfileHandler.CreateTeacherProfile)
		profiles.Get("/", teacherProfileHandler.GetAllTeacherProfiles)
		profiles.Get("/:id", teacherProfileHandler.GetTeacherProfileByID)
		profiles.Put("/:id", teacherProfileHandler.UpdateTeacherProfile)
		profiles.Delete("/:id", teacherProfileHandler.DeleteTeacherProfile)
	}
}
