package router

import (
	"github.com/gofiber/fiber/v2"
	"study.com/v1/internal/handler"
)

func SetupSystemRoleRoutes(
	api fiber.Router,
	systemRoleHandler *handler.SystemRoleHandler,
) {
	systemRoles := api.Group("/system-roles")
	{
		systemRoles.Post("/", systemRoleHandler.CreateSystemRole)
		systemRoles.Get("/", systemRoleHandler.GetAllSystemRoles)
		systemRoles.Get("/:id", systemRoleHandler.GetSystemRole)
		systemRoles.Put("/:id", systemRoleHandler.UpdateSystemRole)
		systemRoles.Delete("/:id", systemRoleHandler.DeleteSystemRole)

		// SystemRole-Permission management
		systemRoles.Get("/:id/permissions", systemRoleHandler.GetSystemRolePermissions)
		systemRoles.Post("/:id/permissions", systemRoleHandler.AddPermissionsToSystemRole)
		systemRoles.Put("/:id/permissions", systemRoleHandler.SetSystemRolePermissions)
		systemRoles.Delete("/:id/permissions", systemRoleHandler.RemovePermissionsFromSystemRole)
	}
}
