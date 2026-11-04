package router

import (
	"github.com/gofiber/fiber/v2"
	"github.com/redis/go-redis/v9"
	"study.com/v1/internal/config"
	"study.com/v1/internal/handler"
	"study.com/v1/internal/middleware"
	"study.com/v1/internal/model"
)

func SetupOrgRoleRoutes(
	api fiber.Router,
	cfg *config.Config,
	roleHandler *handler.RoleHandler,
	redis *redis.Client,
	permChecker *middleware.PermissionChecker,
	auditRec middleware.AuditRecorder,
) {
	// Quản lý org role (Role) là thao tác dành cho ORG_OWNER của tổ chức đang active (hoặc
	// system admin có "*"). Lưu ý: :id ở đây là Role.ID, không phải Organization.ID nên chưa
	// đối chiếu được organization_id của chính role đó với active_org_id ở tầng router — residual
	// gap này được ghi lại trong báo cáo bàn giao.
	orgRoles := api.Group("/org-roles", middleware.AuthMiddleware(cfg, redis), permChecker.RequirePermissions("ORG_ROLES_MANAGE"))
	{
		// Id vai trò mới nằm trong response, handler đặt đích bằng SetAuditTarget.
		orgRoles.Post("/", middleware.Audit(auditRec, model.AuditActionOrgRoleCreate, "org_role", ""), roleHandler.CreateRole)
		orgRoles.Get("/", roleHandler.GetAllRoles)
		orgRoles.Get("/:id", roleHandler.GetRole)
		orgRoles.Put("/:id", middleware.Audit(auditRec, model.AuditActionOrgRoleUpdate, "org_role", "id"), roleHandler.UpdateRole)
		orgRoles.Delete("/:id", middleware.Audit(auditRec, model.AuditActionOrgRoleDelete, "org_role", "id"), roleHandler.DeleteRole)
		orgRoles.Patch("/:id/restore", middleware.Audit(auditRec, model.AuditActionOrgRoleRestore, "org_role", "id"), roleHandler.RestoreRole)

		orgRoles.Get("/:id/permissions", roleHandler.GetRolePermissions)
		orgRoles.Post("/:id/permissions", middleware.Audit(auditRec, model.AuditActionOrgRolePermissions, "org_role", "id"), roleHandler.AddPermissionsToRole)
		orgRoles.Put("/:id/permissions", middleware.Audit(auditRec, model.AuditActionOrgRolePermissions, "org_role", "id"), roleHandler.SetRolePermissions)
		orgRoles.Delete("/:id/permissions", middleware.Audit(auditRec, model.AuditActionOrgRolePermissions, "org_role", "id"), roleHandler.RemovePermissionsFromRole)
	}
}
