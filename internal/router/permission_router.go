package router

import (
	"github.com/gofiber/fiber/v2"
	"study.com/v1/internal/handler"
)

func SetupPermissionRoutes(
	api fiber.Router,
	permissionHandler *handler.PermissionHandler,
) {
	permissions := api.Group("/permissions")
	{
		permissions.Get("/", permissionHandler.GetAllPermissions)
		permissions.Get("/:id", permissionHandler.GetPermission)
		permissions.Put("/:id", permissionHandler.UpdatePermission)
	}
}
