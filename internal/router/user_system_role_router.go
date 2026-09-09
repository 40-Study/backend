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
	permChecker *middleware.PermissionChecker,
) {
	//User Routes (Authenticated)
	// GET /me/system-roles - Lấy system roles của chính mình
	me := api.Group("/me")
	me.Use(middleware.AuthMiddleware(cfg, redis))
	me.Get("/system-roles", userSystemRoleHandler.GetMySystemRoles)

	// ============ Admin Routes (Authenticated + ROLES_MANAGE_SYSTEM) ============
	// User system role management
	// H-01 (review vòng 1): TRƯỚC ĐÂY dùng users.Use(...) ở mức group cho prefix "/users" —
	// trong Fiber v2, group.Use(h) đăng ký một route "use" khớp PREFIX, áp dụng cho MỌI route
	// cùng prefix đăng ký SAU nó trong stack, không giới hạn trong biến group này. Vì
	// SetupUserSystemRoleRoutes chạy trước SetupUserOrganizationRoleRoutes và
	// SetupUserStatsRoutes (route.go), permission ROLES_MANAGE_SYSTEM vô tình khóa luôn
	// GET /users/:id/public-profile (vốn public) và toàn bộ /users/:user_id/org-roles
	// (đáng lẽ ORG_OWNER quản lý được, không cần SYSTEM_ADMIN). Sửa: bỏ Use() ở mức group,
	// gắn AuthMiddleware + permission trực tiếp lên TỪNG route admin thật sự cần.
	users := api.Group("/users")
	usersAuth := middleware.AuthMiddleware(cfg, redis)
	usersAdminPerm := permChecker.RequirePermissions("ROLES_MANAGE_SYSTEM")

	// GET /users/:user_id/system-roles - Lấy system roles của user
	users.Get("/:user_id/system-roles", usersAuth, usersAdminPerm, userSystemRoleHandler.GetUserSystemRoles)

	// POST /users/:user_id/system-roles - Gán system roles cho user
	users.Post("/:user_id/system-roles", usersAuth, usersAdminPerm, userSystemRoleHandler.AssignSystemRolesToUser)

	// DELETE /users/:user_id/system-roles/:system_role_id - Gỡ system role khỏi user
	users.Delete("/:user_id/system-roles/:system_role_id", usersAuth, usersAdminPerm, userSystemRoleHandler.RevokeSystemRoleFromUser)

	// ============ System Role User Management ============
	systemRoles := api.Group("/system-roles")
	systemRoles.Use(middleware.AuthMiddleware(cfg, redis))
	systemRoles.Use(permChecker.RequirePermissions("ROLES_MANAGE_SYSTEM"))

	// GET /system-roles/:system_role_id/users - Lấy users theo system role
	systemRoles.Get("/:system_role_id/users", userSystemRoleHandler.GetUsersBySystemRole)
}
