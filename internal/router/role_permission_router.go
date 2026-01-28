package router

import (
	"github.com/gofiber/fiber/v2"
	"study.com/v1/internal/handler"
)

func SetupRolePermissionRoutes(
	api fiber.Router,
	rolePermissionHandler *handler.RolePermissionHandler,
) {
	// Role routes
	roles := api.Group("/roles")
	{
		roles.Post("/", rolePermissionHandler.CreateRole)
		roles.Get("/", rolePermissionHandler.GetAllRoles)
		roles.Get("/:id", rolePermissionHandler.GetRole)
		roles.Put("/:id", rolePermissionHandler.UpdateRole)
		roles.Delete("/:id", rolePermissionHandler.DeleteRole)

		// Role-Permission management
		roles.Get("/:id/permissions", rolePermissionHandler.GetRolePermissions)
		roles.Post("/:id/permissions", rolePermissionHandler.AddPermissionsToRole)
		roles.Put("/:id/permissions", rolePermissionHandler.SetRolePermissions)
		roles.Delete("/:id/permissions", rolePermissionHandler.RemovePermissionsFromRole)
	}

	// Permission routes
	permissions := api.Group("/permissions")
	{
		permissions.Get("/", rolePermissionHandler.GetAllPermissions)
		permissions.Get("/:id", rolePermissionHandler.GetPermission)
		permissions.Put("/:id", rolePermissionHandler.UpdatePermission)
	}
}
