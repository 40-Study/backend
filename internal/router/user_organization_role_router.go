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

	// ============ User Routes ============
	// GET /me/org-roles - Get my organization roles
	me := api.Group("/me", authMiddleware)
	me.Get("/org-roles", userOrgRoleHandler.GetMyOrgRoles)

	// ============ Admin Routes ============
	// User organization roles management
	users := api.Group("/users", authMiddleware)
	{
		userOrgRoles := users.Group("/:user_id/org-roles")
		{
			// GET /users/:user_id/org-roles - Get all organization roles of a user
			userOrgRoles.Get("/", userOrgRoleHandler.GetUserOrgRoles)

			// POST /users/:user_id/org-roles - Assign organization roles to user
			userOrgRoles.Post("/", userOrgRoleHandler.AssignOrgRolesToUser)

			// DELETE /users/:user_id/org-roles/:org_role_id - Revoke role (soft delete)
			userOrgRoles.Delete("/:org_role_id", userOrgRoleHandler.RevokeOrgRoleFromUser)
		}
	}

	// ============ Organization Role Management ============
	// GET /org-roles/:org_role_id/users - Get users by organization role
	orgRoles := api.Group("/org-roles", authMiddleware)
	orgRoles.Get("/:role_id/users", userOrgRoleHandler.GetUsersWithOrgRoleSimple)

	// ============ Organization Members ============
	organizations := api.Group("/organizations", authMiddleware)
	{
		orgGroup := organizations.Group("/:organization_id")
		{
			// GET /organizations/:organization_id/members - Get all members of organization
			orgGroup.Get("/members", userOrgRoleHandler.GetOrganizationMembers)

			// GET /organizations/:organization_id/roles/:role_id/users - Get users with role in organization
			orgGroup.Get("/roles/:role_id/users", userOrgRoleHandler.GetUsersWithOrgRole)
		}
	}
}
