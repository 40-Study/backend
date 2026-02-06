package router

import (
	"github.com/gofiber/fiber/v2"
	"github.com/minio/minio-go/v7"
	"github.com/redis/go-redis/v9"
	"study.com/v1/internal/config"
	"study.com/v1/internal/handler"
)

func SetupAllRoutes(
	app *fiber.App,
	cfg *config.Config,
	authHandler *handler.AuthHandler,
	roleHandler *handler.RoleHandler,
	systemRoleHandler *handler.SystemRoleHandler,
	userSystemRoleHandler *handler.UserSystemRoleHandler,
	userOrgRoleHandler *handler.UserOrganizationRoleHandler,
	permissionHandler *handler.PermissionHandler,
	organizationHandler *handler.OrganizationHandler,
	profileHandler *handler.ProfileHandler,
	redis *redis.Client,
	minio *minio.Client,
) {
	api := app.Group("/api")

	// Health check
	api.Get("/health", func(c *fiber.Ctx) error {
		return c.JSON(fiber.Map{
			"status":  "ok",
			"message": "Server is running",
		})
	})

	SetupAuthRoutes(api, cfg, authHandler, redis)
	SetupRoleRoutes(api, roleHandler)
	SetupSystemRoleRoutes(api, systemRoleHandler)
	SetupUserSystemRoleRoutes(api, cfg, userSystemRoleHandler, redis)
	SetupUserOrganizationRoleRoutes(api, cfg, userOrgRoleHandler, redis)
	SetupPermissionRoutes(api, permissionHandler)
	SetupOrganizationRoutes(api, organizationHandler)
	SetupProfileRoutes(api, cfg, profileHandler, redis)
}
