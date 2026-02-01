package router

import (
	"github.com/gofiber/fiber/v2"
	"study.com/v1/internal/handler"
)

func SetupRoleRoutes(
	api fiber.Router,
	roleHandler *handler.RoleHandler,
) {
	roles := api.Group("/roles")
	{
		roles.Post("/", roleHandler.CreateRole)
		roles.Get("/", roleHandler.GetAllRoles)
		roles.Get("/:id", roleHandler.GetRole)
		roles.Put("/:id", roleHandler.UpdateRole)
		roles.Delete("/:id", roleHandler.DeleteRole)

		// Role-Permission management
		roles.Get("/:id/permissions", roleHandler.GetRolePermissions)
		roles.Post("/:id/permissions", roleHandler.AddPermissionsToRole)
		roles.Put("/:id/permissions", roleHandler.SetRolePermissions)
		roles.Delete("/:id/permissions", roleHandler.RemovePermissionsFromRole)
	}
}
