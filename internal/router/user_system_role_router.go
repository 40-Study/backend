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
	// Các Routes yêu cầu xác thực
	authMiddleware := middleware.AuthMiddleware(cfg, redis)

	// Quản lý vai trò hệ thống người dùng (dưới /users/:user_id/system-roles)
	users := api.Group("/users")
	{
		userSystemRoles := users.Group("/:user_id/system-roles", authMiddleware)
		{
			// GET /users/:user_id/system-roles - Lấy tất cả vai trò hệ thống của người dùng
			userSystemRoles.Get("/", userSystemRoleHandler.GetUserSystemRoles)

			// POST /users/:user_id/system-roles - Gán vai trò hệ thống cho người dùng (đơn hoặc đa)
			userSystemRoles.Post("/", userSystemRoleHandler.AssignSystemRolesToUser)

			// DELETE /users/:user_id/system-roles - Xóa tất cả vai trò hệ thống khỏi người dùng
			userSystemRoles.Delete("/", userSystemRoleHandler.RemoveAllSystemRolesFromUser)

			// GET /users/:user_id/system-roles/:system_role_id/check - Kiểm tra xem người dùng có vai trò hệ thống không
			userSystemRoles.Get("/:system_role_id/check", userSystemRoleHandler.CheckUserHasSystemRole)
		}
	}

	// Người dùng có vai trò hệ thống (dưới /system-roles/:system_role_id/users)
	systemRoles := api.Group("/system-roles")
	{
		// GET /system-roles/:system_role_id/users - Lấy tất cả người dùng có một vai trò hệ thống cụ thể
		systemRoles.Get("/:system_role_id/users", authMiddleware, userSystemRoleHandler.GetUsersWithSystemRole)
	}

	// Các thao tác trực tiếp với vai trò hệ thống người dùng (dưới /user-system-roles)
	userSystemRolesGroup := api.Group("/user-system-roles", authMiddleware)
	{
		// GET /user-system-roles/:id - Lấy vai trò hệ thống người dùng theo ID
		userSystemRolesGroup.Get("/:id", userSystemRoleHandler.GetUserSystemRoleByID)

		// PATCH /user-system-roles/:id/status - Cập nhật trạng thái
		userSystemRolesGroup.Patch("/:id/status", userSystemRoleHandler.UpdateUserSystemRoleStatus)

		// DELETE /user-system-roles/:id - Xóa một vai trò hệ thống cụ thể khỏi người dùng
		userSystemRolesGroup.Delete("/:id", userSystemRoleHandler.RemoveSystemRoleFromUser)
	}
}
