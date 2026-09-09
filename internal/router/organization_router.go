package router

import (
	"github.com/gofiber/fiber/v2"
	"github.com/redis/go-redis/v9"
	"study.com/v1/internal/config"
	"study.com/v1/internal/handler"
	"study.com/v1/internal/middleware"
)

// SetupOrganizationRoutes đăng ký route quản lý tổ chức.
//
// C-04 (audit 260909): trước đây hàm này không nhận cfg/redis nên KHÔNG THỂ gắn
// AuthMiddleware — mọi request ẩn danh đều tạo/sửa/xóa được tổ chức. Đã thêm cfg, redis
// để gắn auth + permission cho các route ghi; GET danh sách/chi tiết vẫn public.
func SetupOrganizationRoutes(
	api fiber.Router,
	cfg *config.Config,
	organizationHandler *handler.OrganizationHandler,
	redis *redis.Client,
	permChecker *middleware.PermissionChecker,
) {
	authMiddleware := middleware.AuthMiddleware(cfg, redis)

	organizations := api.Group("/organizations")
	{
		// Tạo tổ chức mới: cần đăng nhập + quyền ORG_CREATE (TEACHER/SYSTEM_ADMIN theo data/roles.json).
		organizations.Post("/", authMiddleware, permChecker.RequirePermissions("ORG_CREATE"), organizationHandler.CreateOrganization)
		organizations.Get("/", organizationHandler.GetAllOrganizations)
		organizations.Get("/:id", organizationHandler.GetOrganization)
		// Sửa/xóa tổ chức: Organization model hiện không có cột owner (OwnerID/CreatedBy) nên
		// không thể so khớp "chủ sở hữu" trực tiếp trong DB. Dùng RequireOrgPermission: chỉ
		// system admin ("*"), hoặc user đang giữ ORG_ROLES_MANAGE (vai ORG_OWNER) VÀ đang active
		// đúng tổ chức đó (active_org_id == :id) mới được sửa/xóa.
		organizations.Put("/:id", authMiddleware, permChecker.RequireOrgPermission("id", "ORG_ROLES_MANAGE"), organizationHandler.UpdateOrganization)
		organizations.Delete("/:id", authMiddleware, permChecker.RequireOrgPermission("id", "ORG_ROLES_MANAGE"), organizationHandler.DeleteOrganization)
	}
}
