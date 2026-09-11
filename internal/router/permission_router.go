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
	permChecker *middleware.PermissionChecker,
) {
	permissions := api.Group("/permissions", middleware.AuthMiddleware(cfg, redis))
	{
		// Đọc danh sách permission: chỉ cần đăng nhập (dùng để hiển thị UI phân quyền).
		permissions.Get("/", permissionHandler.GetAllPermissions)
		permissions.Get("/:id", permissionHandler.GetPermissionByID)
		// Sửa permission là thao tác quản trị RBAC hệ thống.
		permissions.Put("/:id", permChecker.RequirePermissions("ROLES_MANAGE_SYSTEM"), permissionHandler.UpdatePermission)
	}
}
