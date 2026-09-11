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

	// H-01 (review vòng 1): trước đây dùng api.Group("/users", authMiddleware) — cũng là một
	// group.Use() khớp PREFIX "/users" giống lỗi ở user_system_role_router.go, sẽ đè luôn lên
	// route công khai GET /users/:id/public-profile (đăng ký sau, route.go) bắt nó phải đăng
	// nhập. Gắn authMiddleware trực tiếp lên từng route thay vì ở mức group.
	users := api.Group("/users")
	{
		// Gán/gỡ org role cho user là thao tác quản trị thành viên tổ chức.
		userOrgRoles := users.Group("/:user_id/org-roles")
		userOrgRoles.Get("/", authMiddleware, requireMembersManage, userOrgRoleHandler.GetUserOrgRoles)
		userOrgRoles.Post("/", authMiddleware, requireMembersManage, userOrgRoleHandler.AssignOrgRolesToUser)
		userOrgRoles.Delete("/:org_role_id", authMiddleware, requireMembersManage, userOrgRoleHandler.RevokeOrgRoleFromUser)
	}

	orgRoles := api.Group("/org-roles", authMiddleware)
	orgRoles.Get("/:role_id/users", requireMembersManage, userOrgRoleHandler.GetUsersWithOrgRoleSimple)

	// Các route theo :organization_id có org trong path → dùng RequireOrgPermission để đối
	// chiếu active_org_id (JWT) với đúng tổ chức đang thao tác, tránh ORG_OWNER của tổ chức A
	// dùng quyền của mình để xem thành viên tổ chức B.
	//
	// M2-01 (review vòng 3): TRƯỚC ĐÂY api.Group("/organizations", authMiddleware) — cùng lỗi
	// group.Use() khớp PREFIX "/organizations" như H-01 (xem comment ở "/users" phía trên) —
	// đè luôn lên 2 route CÔNG KHAI GET /organizations và GET /organizations/:id đăng ký ở
	// organization_router.go (SetupOrganizationRoutes), bắt cả 2 phải đăng nhập dù handler
	// (GetAllOrganizations/GetOrganization) không hề đòi user_id. Gỡ authMiddleware khỏi group,
	// gắn trực tiếp vào 2 route quản trị thành viên bên dưới.
	organizations := api.Group("/organizations")
	{
		orgGroup := organizations.Group("/:organization_id")
		orgGroup.Get("/members", authMiddleware, permChecker.RequireOrgPermission("organization_id", "ORG_MEMBERS_MANAGE"), userOrgRoleHandler.GetOrganizationMembers)
		orgGroup.Get("/roles/:role_id/users", authMiddleware, permChecker.RequireOrgPermission("organization_id", "ORG_MEMBERS_MANAGE"), userOrgRoleHandler.GetUsersWithOrgRole)
	}
}
