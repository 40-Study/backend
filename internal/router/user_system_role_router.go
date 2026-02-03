package router

import (
	"github.com/gofiber/fiber/v2"
	"github.com/redis/go-redis/v9"
	"study.com/v1/internal/config"
	"study.com/v1/internal/handler"
	"study.com/v1/internal/middleware"
)

func SetupUserSystemRoleRoutes(
	api fiber.Router,
	cfg *config.Config,
	userSystemRoleHandler *handler.UserSystemRoleHandler,
	redis *redis.Client,
) {
	// Routes that require authentication
	authMiddleware := middleware.AuthMiddleware(cfg, redis)

	// User system roles management (under /users/:user_id/system-roles)
	users := api.Group("/users")
	{
		userSystemRoles := users.Group("/:user_id/system-roles", authMiddleware)
		{
			// GET /users/:user_id/system-roles - Get all system roles of a user
			userSystemRoles.Get("/", userSystemRoleHandler.GetUserSystemRoles)

			// POST /users/:user_id/system-roles - Assign system roles to user (single or multiple)
			userSystemRoles.Post("/", userSystemRoleHandler.AssignSystemRolesToUser)

			// DELETE /users/:user_id/system-roles - Remove all system roles from user
			userSystemRoles.Delete("/", userSystemRoleHandler.RemoveAllSystemRolesFromUser)

			// GET /users/:user_id/system-roles/:system_role_id/check - Check if user has system role
			userSystemRoles.Get("/:system_role_id/check", userSystemRoleHandler.CheckUserHasSystemRole)
		}
	}

	// System roles users (under /system-roles/:system_role_id/users)
	systemRoles := api.Group("/system-roles")
	{
		// GET /system-roles/:system_role_id/users - Get all users with a specific system role
		systemRoles.Get("/:system_role_id/users", authMiddleware, userSystemRoleHandler.GetUsersWithSystemRole)
	}

	// Direct user system role operations (under /user-system-roles)
	userSystemRolesGroup := api.Group("/user-system-roles", authMiddleware)
	{
		// GET /user-system-roles/:id - Get user system role by ID
		userSystemRolesGroup.Get("/:id", userSystemRoleHandler.GetUserSystemRoleByID)

		// PATCH /user-system-roles/:id/status - Update status
		userSystemRolesGroup.Patch("/:id/status", userSystemRoleHandler.UpdateUserSystemRoleStatus)

		// DELETE /user-system-roles/:id - Remove a specific system role from user
		userSystemRolesGroup.Delete("/:id", userSystemRoleHandler.RemoveSystemRoleFromUser)
	}
}
