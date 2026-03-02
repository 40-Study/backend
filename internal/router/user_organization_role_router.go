package router

import (
	"github.com/gofiber/fiber/v2"
	"github.com/redis/go-redis/v9"
	"study.com/v1/internal/config"
	"study.com/v1/internal/handler"
	"study.com/v1/internal/middleware"
)

func SetupUserOrganizationRoleRoutes(
	api fiber.Router,
	cfg *config.Config,
	userOrgRoleHandler *handler.UserOrganizationRoleHandler,
	redis *redis.Client,
) {
	authMiddleware := middleware.AuthMiddleware(cfg, redis)

	users := api.Group("/users", authMiddleware)
	{
		userOrgRoles := users.Group("/:user_id/org-roles")
		userOrgRoles.Get("/", userOrgRoleHandler.GetUserOrgRoles)
		userOrgRoles.Post("/", userOrgRoleHandler.AssignOrgRolesToUser)
		userOrgRoles.Delete("/:org_role_id", userOrgRoleHandler.RevokeOrgRoleFromUser)
	}

	orgRoles := api.Group("/org-roles", authMiddleware)
	orgRoles.Get("/:role_id/users", userOrgRoleHandler.GetUsersWithOrgRoleSimple)

	organizations := api.Group("/organizations", authMiddleware)
	{
		orgGroup := organizations.Group("/:organization_id")
		orgGroup.Get("/members", userOrgRoleHandler.GetOrganizationMembers)
		orgGroup.Get("/roles/:role_id/users", userOrgRoleHandler.GetUsersWithOrgRole)
	}
}
