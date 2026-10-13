package router

import (
	"github.com/gofiber/fiber/v2"
	"github.com/redis/go-redis/v9"
	"study.com/v1/internal/config"
	"study.com/v1/internal/handler"
	"study.com/v1/internal/middleware"
	"study.com/v1/internal/model"
)

// CatalogueReadPermissions: ai được ĐỌC danh mục vai trò hệ thống (GET /system-roles) và permission (GET /permissions).
// Mỗi quyền gác một nhóm trang quản trị trong web gọi các endpoint này: ROLES_MANAGE_SYSTEM (trang vai trò + phân
// quyền), USERS_VIEW_ALL (ô lọc/gán vai trò ở trang người dùng), SYSTEM_SETTINGS_MANAGE (chọn đối tượng thông báo,
// dashboard). Thêm màn quản trị mới cần danh mục thì thêm quyền gác màn đó vào đây, không mở lại cho mọi user.
var CatalogueReadPermissions = []string{"ROLES_MANAGE_SYSTEM", "USERS_VIEW_ALL", "SYSTEM_SETTINGS_MANAGE"}

func SetupPermissionRoutes(
	api fiber.Router,
	cfg *config.Config,
	permissionHandler *handler.PermissionHandler,
	redis *redis.Client,
	permChecker *middleware.PermissionChecker,
	auditRec middleware.AuditRecorder,
) {
	permissions := api.Group("/permissions", middleware.AuthMiddleware(cfg, redis))
	{
		// Đọc danh mục permission (UI phân quyền của admin). ADM-L10: trước đây chỉ cần đăng nhập nên giảng viên
		// đọc được cả 28 permission; nay cần một trong CatalogueReadPermissions.
		permissions.Get("/", permChecker.RequireAnyPermission(CatalogueReadPermissions...), permissionHandler.GetAllPermissions)
		permissions.Get("/:id", permChecker.RequireAnyPermission(CatalogueReadPermissions...), permissionHandler.GetPermissionByID)
		// Sửa permission là thao tác quản trị RBAC hệ thống.
		permissions.Put("/:id", permChecker.RequirePermissions("ROLES_MANAGE_SYSTEM"), middleware.Audit(auditRec, model.AuditActionPermissionUpdate, "permission", "id"), permissionHandler.UpdatePermission)
	}
}
