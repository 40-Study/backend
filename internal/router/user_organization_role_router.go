package router

import (
	"github.com/gofiber/fiber/v2"
	"github.com/redis/go-redis/v9"
	"study.com/v1/internal/config"
	"study.com/v1/internal/handler"
	"study.com/v1/internal/middleware"
)

func SetupUserOrganizationRoleRoutes(
	api fiber.Router,
	cfg *config.Config,
	userOrgRoleHandler *handler.UserOrganizationRoleHandler,
	redis *redis.Client,
	permChecker *middleware.PermissionChecker,
) {
	authMiddleware := middleware.AuthMiddleware(cfg, redis)
	requireMembersManage := permChecker.RequirePermissions("ORG_MEMBERS_MANAGE")

	users := api.Group("/users", authMiddleware)
	{
		// Gán/gỡ org role cho user là thao tác quản trị thành viên tổ chức.
		userOrgRoles := users.Group("/:user_id/org-roles")
		userOrgRoles.Get("/", requireMembersManage, userOrgRoleHandler.GetUserOrgRoles)
		userOrgRoles.Post("/", requireMembersManage, userOrgRoleHandler.AssignOrgRolesToUser)
		userOrgRoles.Delete("/:org_role_id", requireMembersManage, userOrgRoleHandler.RevokeOrgRoleFromUser)
	}

	orgRoles := api.Group("/org-roles", authMiddleware)
	orgRoles.Get("/:role_id/users", requireMembersManage, userOrgRoleHandler.GetUsersWithOrgRoleSimple)

	// Các route theo :organization_id có org trong path → dùng RequireOrgPermission để đối
	// chiếu active_org_id (JWT) với đúng tổ chức đang thao tác, tránh ORG_OWNER của tổ chức A
	// dùng quyền của mình để xem thành viên tổ chức B.
	organizations := api.Group("/organizations", authMiddleware)
	{
		orgGroup := organizations.Group("/:organization_id")
		orgGroup.Get("/members", permChecker.RequireOrgPermission("organization_id", "ORG_MEMBERS_MANAGE"), userOrgRoleHandler.GetOrganizationMembers)
		orgGroup.Get("/roles/:role_id/users", permChecker.RequireOrgPermission("organization_id", "ORG_MEMBERS_MANAGE"), userOrgRoleHandler.GetUsersWithOrgRole)
	}
}
