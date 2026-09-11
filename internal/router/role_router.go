package router

import (
	"github.com/gofiber/fiber/v2"
	"github.com/redis/go-redis/v9"
	"study.com/v1/internal/config"
	"study.com/v1/internal/handler"
	"study.com/v1/internal/middleware"
)

func SetupOrgRoleRoutes(
	api fiber.Router,
	cfg *config.Config,
	roleHandler *handler.RoleHandler,
	redis *redis.Client,
	permChecker *middleware.PermissionChecker,
) {
	// Quản lý org role (Role) là thao tác dành cho ORG_OWNER của tổ chức đang active (hoặc
	// system admin có "*"). Lưu ý: :id ở đây là Role.ID, không phải Organization.ID nên chưa
	// đối chiếu được organization_id của chính role đó với active_org_id ở tầng router — residual
	// gap này được ghi lại trong báo cáo bàn giao.
	orgRoles := api.Group("/org-roles", middleware.AuthMiddleware(cfg, redis), permChecker.RequirePermissions("ORG_ROLES_MANAGE"))
	{
		orgRoles.Post("/", roleHandler.CreateRole)
		orgRoles.Get("/", roleHandler.GetAllRoles)
		orgRoles.Get("/:id", roleHandler.GetRole)
		orgRoles.Put("/:id", roleHandler.UpdateRole)
		orgRoles.Delete("/:id", roleHandler.DeleteRole)
		orgRoles.Patch("/:id/restore", roleHandler.RestoreRole)

		orgRoles.Get("/:id/permissions", roleHandler.GetRolePermissions)
		orgRoles.Post("/:id/permissions", roleHandler.AddPermissionsToRole)
		orgRoles.Put("/:id/permissions", roleHandler.SetRolePermissions)
		orgRoles.Delete("/:id/permissions", roleHandler.RemovePermissionsFromRole)
	}
}
