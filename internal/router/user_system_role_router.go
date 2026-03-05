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
	//User Routes (Authenticated) 
	// GET /me/system-roles - Lấy system roles của chính mình
	me := api.Group("/me")
	me.Use(middleware.AuthMiddleware(cfg, redis))
	me.Get("/system-roles", userSystemRoleHandler.GetMySystemRoles)

	// ============ Admin Routes (Authenticated + TODO: Permission check) ============
	// User system role management
	users := api.Group("/users")
	users.Use(middleware.AuthMiddleware(cfg, redis))
	// TODO: Add permission middleware for admin-only access
	// users.Use(middleware.RequirePermission("manage_user_roles"))

	// GET /users/:user_id/system-roles - Lấy system roles của user
	users.Get("/:user_id/system-roles", userSystemRoleHandler.GetUserSystemRoles)

	// POST /users/:user_id/system-roles - Gán system roles cho user
	users.Post("/:user_id/system-roles", userSystemRoleHandler.AssignSystemRolesToUser)

	// DELETE /users/:user_id/system-roles/:system_role_id - Gỡ system role khỏi user
	users.Delete("/:user_id/system-roles/:system_role_id", userSystemRoleHandler.RevokeSystemRoleFromUser)

	// ============ System Role User Management ============
	systemRoles := api.Group("/system-roles")
	systemRoles.Use(middleware.AuthMiddleware(cfg, redis))
	// TODO: Add permission middleware for admin-only access

	// GET /system-roles/:system_role_id/users - Lấy users theo system role
	systemRoles.Get("/:system_role_id/users", userSystemRoleHandler.GetUsersBySystemRole)
}
