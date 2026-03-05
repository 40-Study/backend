package router

import (
	"github.com/gofiber/fiber/v2"
	"github.com/redis/go-redis/v9"
	"study.com/v1/internal/config"
	"study.com/v1/internal/handler"
	"study.com/v1/internal/middleware"
)

func SetupOrgRoleRoutes(
	api fiber.Router,
	cfg *config.Config,
	roleHandler *handler.RoleHandler,
	redis *redis.Client,
) {
	orgRoles := api.Group("/org-roles", middleware.AuthMiddleware(cfg, redis))
	{
		orgRoles.Post("/", roleHandler.CreateRole)
		orgRoles.Get("/", roleHandler.GetAllRoles)
		orgRoles.Get("/:id", roleHandler.GetRole)
		orgRoles.Put("/:id", roleHandler.UpdateRole)
		orgRoles.Delete("/:id", roleHandler.DeleteRole)
		orgRoles.Patch("/:id/restore", roleHandler.RestoreRole)

		orgRoles.Get("/:id/permissions", roleHandler.GetRolePermissions)
		orgRoles.Post("/:id/permissions", roleHandler.AddPermissionsToRole)
		orgRoles.Put("/:id/permissions", roleHandler.SetRolePermissions)
		orgRoles.Delete("/:id/permissions", roleHandler.RemovePermissionsFromRole)
	}
}
