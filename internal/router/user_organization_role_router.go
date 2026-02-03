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
) {
	authMiddleware := middleware.AuthMiddleware(cfg, redis)

	// Quản lý vai trò tổ chức người dùng (dưới /users/:user_id)
	users := api.Group("/users")
	{
		// Các vai trò tổ chức của một người dùng
		userOrgRoles := users.Group("/:user_id/organization-roles", authMiddleware)
		{
			// GET /users/:user_id/organization-roles - Lấy tất cả vai trò tổ chức của người dùng
			userOrgRoles.Get("/", userOrgRoleHandler.GetUserOrgRoles)

			// POST /users/:user_id/organization-roles - Gán vai trò tổ chức cho người dùng (đơn hoặc đa)
			userOrgRoles.Post("/", userOrgRoleHandler.AssignOrgRolesToUser)

			// DELETE /users/:user_id/organization-roles - Xóa tất cả vai trò tổ chức khỏi người dùng
			userOrgRoles.Delete("/", userOrgRoleHandler.RemoveAllOrgRolesFromUser)
		}

		// Vai trò người dùng trong một tổ chức cụ thể
		userOrgs := users.Group("/:user_id/organizations", authMiddleware)
		{
			// GET /users/:user_id/organizations/:organization_id/roles - Lấy vai trò người dùng trong tổ chức
			userOrgs.Get("/:organization_id/roles", userOrgRoleHandler.GetUserOrgRolesInOrganization)

			// GET /users/:user_id/organizations/:organization_id/roles/:role_id/check - Kiểm tra xem người dùng có vai trò trong tổ chức không
			userOrgs.Get("/:organization_id/roles/:role_id/check", userOrgRoleHandler.CheckUserHasOrgRole)

			// DELETE /users/:user_id/organizations/:organization_id - Xóa người dùng khỏi tổ chức
			userOrgs.Delete("/:organization_id", userOrgRoleHandler.RemoveUserFromOrganization)
		}
	}

	// Quản lý thành viên tổ chức (dưới /organizations/:organization_id)
	organizations := api.Group("/organizations")
	{
		orgMembers := organizations.Group("/:organization_id", authMiddleware)
		{
			// GET /organizations/:organization_id/members - Lấy tất cả thành viên của tổ chức
			orgMembers.Get("/members", userOrgRoleHandler.GetOrganizationMembers)

			// GET /organizations/:organization_id/roles/:role_id/users - Lấy người dùng có vai trò trong tổ chức
			orgMembers.Get("/roles/:role_id/users", userOrgRoleHandler.GetUsersWithOrgRole)
		}
	}

	// Các thao tác trực tiếp với vai trò tổ chức người dùng (dưới /user-organization-roles)
	userOrgRolesGroup := api.Group("/user-organization-roles", authMiddleware)
	{
		// GET /user-organization-roles/:id - Lấy vai trò tổ chức người dùng theo ID
		userOrgRolesGroup.Get("/:id", userOrgRoleHandler.GetUserOrgRoleByID)

		// PATCH /user-organization-roles/:id/status - Cập nhật trạng thái
		userOrgRolesGroup.Patch("/:id/status", userOrgRoleHandler.UpdateUserOrgRoleStatus)

		// DELETE /user-organization-roles/:id - Xóa một vai trò tổ chức cụ thể khỏi người dùng
		userOrgRolesGroup.Delete("/:id", userOrgRoleHandler.RemoveOrgRoleFromUser)
	}
}
