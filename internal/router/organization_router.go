package router

import (
	"github.com/gofiber/fiber/v2"
	"study.com/v1/internal/handler"
)

func SetupOrganizationRoutes(
	api fiber.Router,
	organizationHandler *handler.OrganizationHandler,
) {
	organizations := api.Group("/organizations")
	{
		organizations.Post("/", organizationHandler.CreateOrganization)
		organizations.Get("/", organizationHandler.GetAllOrganizations)
		organizations.Get("/:id", organizationHandler.GetOrganization)
		organizations.Put("/:id", organizationHandler.UpdateOrganization)
		organizations.Delete("/:id", organizationHandler.DeleteOrganization)
	}
}
