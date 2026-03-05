package router

import (
	"github.com/gofiber/fiber/v2"
	"github.com/redis/go-redis/v9"
	"study.com/v1/internal/config"
	"study.com/v1/internal/handler"
	"study.com/v1/internal/middleware"
)

func SetupPermissionRoutes(
	api fiber.Router,
	cfg *config.Config,
	permissionHandler *handler.PermissionHandler,
	redis *redis.Client,
) {
	permissions := api.Group("/permissions", middleware.AuthMiddleware(cfg, redis))
	{
		permissions.Get("/", permissionHandler.GetAllPermissions)
		permissions.Get("/:id", permissionHandler.GetPermissionByID)
		permissions.Put("/:id", permissionHandler.UpdatePermission)
	}
}
