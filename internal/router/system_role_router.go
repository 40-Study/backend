package router

import (
	"github.com/gofiber/fiber/v2"
	"github.com/redis/go-redis/v9"
	"study.com/v1/internal/config"
	"study.com/v1/internal/handler"
	"study.com/v1/internal/middleware"
)

func SetupSystemRoleRoutes(
	api fiber.Router,
	cfg *config.Config,
	systemRoleHandler *handler.SystemRoleHandler,
	redis *redis.Client,
	permChecker *middleware.PermissionChecker,
) {
	// A-P2-1 (QA 260927): trước đây route này public hoàn toàn (comment cũ "allow clients to list
	// system roles without auth"). Đã grep web/src: màn hình cần danh sách vai trò TRƯỚC khi đăng
	// nhập (/login/role) gọi GET /auth/system-roles (auth_router.go, vẫn public — route KHÁC),
	// còn GET /system-roles (route này) chỉ được /admin/page.tsx và /admin/roles/page.tsx gọi —
	// hai trang admin luôn có token sẵn — nên gate `auth` không phá luồng nào.
	systemRolesPublic := api.Group("/system-roles", middleware.AuthMiddleware(cfg, redis))
	systemRolesPublic.Get("/", systemRoleHandler.GetAllSystemRoles)

	// Protected routes: require auth + quyền quản trị RBAC hệ thống cho mọi thao tác ghi.
	systemRoles := api.Group("/system-roles", middleware.AuthMiddleware(cfg, redis), permChecker.RequirePermissions("ROLES_MANAGE_SYSTEM"))
	{
		systemRoles.Post("/", systemRoleHandler.CreateSystemRole)
		systemRoles.Get("/:id", systemRoleHandler.GetSystemRole)
		systemRoles.Put("/:id", systemRoleHandler.UpdateSystemRole)
		systemRoles.Delete("/:id", systemRoleHandler.DeleteSystemRole)
		systemRoles.Patch("/:id/restore", systemRoleHandler.RestoreSystemRole)

		systemRoles.Get("/:id/permissions", systemRoleHandler.GetSystemRolePermissions)
		systemRoles.Post("/:id/permissions", systemRoleHandler.AddPermissionsToSystemRole)
		systemRoles.Put("/:id/permissions", systemRoleHandler.SetSystemRolePermissions)
		systemRoles.Delete("/:id/permissions", systemRoleHandler.RemovePermissionsFromSystemRole)
	}
}
