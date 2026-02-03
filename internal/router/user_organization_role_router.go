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

	// User organization roles management (under /users/:user_id)
	users := api.Group("/users")
	{
		// Organization roles of a user
		userOrgRoles := users.Group("/:user_id/organization-roles", authMiddleware)
		{
			// GET /users/:user_id/organization-roles - Get all org roles of a user
			userOrgRoles.Get("/", userOrgRoleHandler.GetUserOrgRoles)

			// POST /users/:user_id/organization-roles - Assign org roles to user (single or multiple)
			userOrgRoles.Post("/", userOrgRoleHandler.AssignOrgRolesToUser)

			// DELETE /users/:user_id/organization-roles - Remove all org roles from user
			userOrgRoles.Delete("/", userOrgRoleHandler.RemoveAllOrgRolesFromUser)
		}

		// User roles in a specific organization
		userOrgs := users.Group("/:user_id/organizations", authMiddleware)
		{
			// GET /users/:user_id/organizations/:organization_id/roles - Get user roles in organization
			userOrgs.Get("/:organization_id/roles", userOrgRoleHandler.GetUserOrgRolesInOrganization)

			// GET /users/:user_id/organizations/:organization_id/roles/:role_id/check - Check if user has role in org
			userOrgs.Get("/:organization_id/roles/:role_id/check", userOrgRoleHandler.CheckUserHasOrgRole)

			// DELETE /users/:user_id/organizations/:organization_id - Remove user from organization
			userOrgs.Delete("/:organization_id", userOrgRoleHandler.RemoveUserFromOrganization)
		}
	}

	// Organization members management (under /organizations/:organization_id)
	organizations := api.Group("/organizations")
	{
		orgMembers := organizations.Group("/:organization_id", authMiddleware)
		{
			// GET /organizations/:organization_id/members - Get all members of organization
			orgMembers.Get("/members", userOrgRoleHandler.GetOrganizationMembers)

			// GET /organizations/:organization_id/roles/:role_id/users - Get users with role in org
			orgMembers.Get("/roles/:role_id/users", userOrgRoleHandler.GetUsersWithOrgRole)
		}
	}

	// Direct user organization role operations (under /user-organization-roles)
	userOrgRolesGroup := api.Group("/user-organization-roles", authMiddleware)
	{
		// GET /user-organization-roles/:id - Get user org role by ID
		userOrgRolesGroup.Get("/:id", userOrgRoleHandler.GetUserOrgRoleByID)

		// PATCH /user-organization-roles/:id/status - Update status
		userOrgRolesGroup.Patch("/:id/status", userOrgRoleHandler.UpdateUserOrgRoleStatus)

		// DELETE /user-organization-roles/:id - Remove a specific org role from user
		userOrgRolesGroup.Delete("/:id", userOrgRoleHandler.RemoveOrgRoleFromUser)
	}
}
