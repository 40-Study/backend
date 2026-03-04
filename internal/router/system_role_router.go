package router

import (
	"github.com/gofiber/fiber/v2"
	"github.com/redis/go-redis/v9"
	"study.com/v1/internal/config"
	"study.com/v1/internal/handler"
	"study.com/v1/internal/middleware"
)

func SetupSystemRoleRoutes(
	api fiber.Router,
	cfg *config.Config,
	systemRoleHandler *handler.SystemRoleHandler,
	redis *redis.Client,
) {
	systemRoles := api.Group("/system-roles", middleware.AuthMiddleware(cfg, redis))
	{
		systemRoles.Post("/", systemRoleHandler.CreateSystemRole)
		systemRoles.Get("/", systemRoleHandler.GetAllSystemRoles)
		systemRoles.Get("/:id", systemRoleHandler.GetSystemRole)
		systemRoles.Put("/:id", systemRoleHandler.UpdateSystemRole)
		systemRoles.Delete("/:id", systemRoleHandler.DeleteSystemRole)
		systemRoles.Patch("/:id/restore", systemRoleHandler.RestoreSystemRole)

		systemRoles.Get("/:id/permissions", systemRoleHandler.GetSystemRolePermissions)
		systemRoles.Post("/:id/permissions", systemRoleHandler.AddPermissionsToSystemRole)
		systemRoles.Put("/:id/permissions", systemRoleHandler.SetSystemRolePermissions)
		systemRoles.Delete("/:id/permissions", systemRoleHandler.RemovePermissionsFromSystemRole)
	}
}
