package middleware

import (
	"context"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"study.com/v1/internal/model"
	"study.com/v1/internal/repository"
)

// PermissionChecker tính tập permission thực tế của user (từ system role + org role
// theo active_org_id hiện tại) và cung cấp middleware để gate route theo permission.
//
// Đây là phần triển khai thật cho RequirePermissions — trước đây hàm này là no-op
// (không kiểm tra gì, cũng không gọi c.Next()) nên không route nào dùng được.
type PermissionChecker struct {
	userSystemRoleRepo repository.UserSystemRoleRepositoryInterface
	systemRoleRepo     repository.SystemRoleRepositoryInterface
	userOrgRoleRepo    repository.UserOrganizationRoleRepositoryInterface
	roleRepo           repository.RoleRepositoryInterface
}

// NewPermissionChecker khởi tạo PermissionChecker từ các repository đã có sẵn trong app.
func NewPermissionChecker(
	userSystemRoleRepo repository.UserSystemRoleRepositoryInterface,
	systemRoleRepo repository.SystemRoleRepositoryInterface,
	userOrgRoleRepo repository.UserOrganizationRoleRepositoryInterface,
	roleRepo repository.RoleRepositoryInterface,
) *PermissionChecker {
	return &PermissionChecker{
		userSystemRoleRepo: userSystemRoleRepo,
		systemRoleRepo:     systemRoleRepo,
		userOrgRoleRepo:    userOrgRoleRepo,
		roleRepo:           roleRepo,
	}
}

// hasWildcard trả về true nếu tập permission có quyền "*" (SYSTEM_ADMIN) — vượt qua mọi permission.
func hasWildcard(granted []string) bool {
	for _, p := range granted {
		if p == "*" {
			return true
		}
	}
	return false
}

// hasAllPermissions là hàm thuần (không side-effect, không DB) để unit test riêng —
// tách khỏi phần resolve permission từ DB theo yêu cầu "tách logic thuần ra hàm riêng".
func hasAllPermissions(granted []string, required []string) bool {
	if hasWildcard(granted) {
		return true
	}
	grantedSet := make(map[string]struct{}, len(granted))
	for _, p := range granted {
		grantedSet[p] = struct{}{}
	}
	for _, req := range required {
		if _, ok := grantedSet[req]; !ok {
			return false
		}
	}
	return true
}

// resolvePermissions gộp permission từ:
//  1. Mọi system role đang ACTIVE của user (không phụ thuộc active_org_id).
//  2. Org role user đang giữ trong activeOrgID hiện tại (nếu JWT có active_org_id).
//
// Role có permission "*" (SYSTEM_ADMIN) sẽ vượt qua mọi permission yêu cầu — xử lý ở hasAllPermissions.
func (pc *PermissionChecker) resolvePermissions(ctx context.Context, userID uuid.UUID, activeOrgID *uuid.UUID) ([]string, error) {
	var perms []string

	systemRoles, err := pc.userSystemRoleRepo.FindByUserID(ctx, userID, model.UserSystemRoleStatusActive)
	if err != nil {
		return nil, err
	}
	for _, usr := range systemRoles {
		rolePerms, err := pc.systemRoleRepo.GetPermissionsBySystemRoleID(ctx, usr.SystemRoleID)
		if err != nil {
			return nil, err
		}
		for _, p := range rolePerms {
			perms = append(perms, p.Name)
		}
	}

	if activeOrgID != nil {
		orgRoles, err := pc.userOrgRoleRepo.FindByUserAndOrganization(ctx, userID, *activeOrgID, model.UserOrgRoleStatusActive)
		if err != nil {
			return nil, err
		}
		for _, uor := range orgRoles {
			rolePerms, err := pc.roleRepo.GetPermissionsByRoleID(ctx, uor.RoleID)
			if err != nil {
				return nil, err
			}
			for _, p := range rolePerms {
				perms = append(perms, p.Name)
			}
		}
	}

	return perms, nil
}

// resolveSystemOnlyPermissions giống resolvePermissions nhưng CHỈ tính permission từ system
// role — bỏ qua org role dù activeOrgID có giá trị. Dùng để phân biệt "được cấp permission ở
// tầm hệ thống/toàn cục" (như SYSTEM_ADMIN) với "chỉ được cấp trong phạm vi 1 tổ chức cụ thể"
// (như ORG_OWNER) — quan trọng cho RequireOrgPermission bên dưới.
//
// Lưu ý: seeder (seeds/seeder.go) mở rộng permission "*" trong data/roles.json thành TOÀN BỘ
// permission cụ thể trong catalog rồi gán từng cái vào system_role_permissions — KHÔNG lưu một
// permission tên literal "*" trong DB. Vì vậy SYSTEM_ADMIN nắm mọi permission (kể cả permission
// vốn thuộc org role như ORG_ROLES_MANAGE/ORG_MEMBERS_MANAGE) qua system role, không cần
// wildcard string để nhận diện — hasWildcard() ở trên chỉ là fallback nếu tương lai có permission
// tên đúng "*" được seed/tạo tay.
func (pc *PermissionChecker) resolveSystemOnlyPermissions(ctx context.Context, userID uuid.UUID) ([]string, error) {
	var perms []string
	systemRoles, err := pc.userSystemRoleRepo.FindByUserID(ctx, userID, model.UserSystemRoleStatusActive)
	if err != nil {
		return nil, err
	}
	for _, usr := range systemRoles {
		rolePerms, err := pc.systemRoleRepo.GetPermissionsBySystemRoleID(ctx, usr.SystemRoleID)
		if err != nil {
			return nil, err
		}
		for _, p := range rolePerms {
			perms = append(perms, p.Name)
		}
	}
	return perms, nil
}

// HasPermission cho phép service/handler kiểm tra ad-hoc (không qua middleware fiber) —
// ví dụ để quyết định "override quyền owner" khi user có wildcard.
func (pc *PermissionChecker) HasPermission(ctx context.Context, userID uuid.UUID, activeOrgID *uuid.UUID, permission string) (bool, error) {
	granted, err := pc.resolvePermissions(ctx, userID, activeOrgID)
	if err != nil {
		return false, err
	}
	return hasAllPermissions(granted, []string{permission}), nil
}

func localsUUID(c *fiber.Ctx, key string) (uuid.UUID, bool) {
	v, ok := c.Locals(key).(uuid.UUID)
	return v, ok
}

// RequirePermissions chỉ cho qua khi user (từ Locals "user_id", do AuthMiddleware set) có
// ĐỦ toàn bộ các permission yêu cầu — thiếu bất kỳ permission nào trả 403.
// Route phải đứng SAU AuthMiddleware (không có "user_id" trong Locals -> 401).
func (pc *PermissionChecker) RequirePermissions(permissions ...string) fiber.Handler {
	return func(c *fiber.Ctx) error {
		userID, ok := localsUUID(c, "user_id")
		if !ok || userID == uuid.Nil {
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
				"success": false,
				"message": "unauthorized",
			})
		}

		var activeOrgID *uuid.UUID
		if orgID, ok := localsUUID(c, "active_org_id"); ok {
			activeOrgID = &orgID
		}

		granted, err := pc.resolvePermissions(c.Context(), userID, activeOrgID)
		if err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
				"success": false,
				"message": "failed to resolve permissions",
			})
		}

		for _, p := range permissions {
			if !hasAllPermissions(granted, []string{p}) {
				return c.Status(fiber.StatusForbidden).JSON(fiber.Map{
					"success": false,
					"message": "forbidden: missing permission " + p,
				})
			}
		}

		return c.Next()
	}
}

// RequireOrgPermission gate các route sửa/xóa tài nguyên thuộc một tổ chức cụ thể (param
// trên URL, ví dụ ":id" hoặc ":organization_id"). Cho qua khi:
//  1. User có permission wildcard "*" (SYSTEM_ADMIN), HOẶC
//  2. User có `orgPermission` VÀ active_org_id trong JWT khớp với org trên URL — tức user
//     đang "đứng vai" đúng tổ chức đang thao tác, không phải mượn quyền ORG_OWNER của tổ chức A
//     để sửa tổ chức B.
func (pc *PermissionChecker) RequireOrgPermission(paramName, orgPermission string) fiber.Handler {
	return func(c *fiber.Ctx) error {
		userID, ok := localsUUID(c, "user_id")
		if !ok || userID == uuid.Nil {
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
				"success": false,
				"message": "unauthorized",
			})
		}

		var activeOrgID *uuid.UUID
		if orgID, ok := localsUUID(c, "active_org_id"); ok {
			activeOrgID = &orgID
		}

		granted, err := pc.resolvePermissions(c.Context(), userID, activeOrgID)
		if err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
				"success": false,
				"message": "failed to resolve permissions",
			})
		}

		if hasWildcard(granted) {
			return c.Next()
		}

		if !hasAllPermissions(granted, []string{orgPermission}) {
			return c.Status(fiber.StatusForbidden).JSON(fiber.Map{
				"success": false,
				"message": "forbidden: missing permission " + orgPermission,
			})
		}

		// User được cấp orgPermission qua SYSTEM role (vd SYSTEM_ADMIN, vốn nắm mọi permission
		// kể cả permission gốc org qua "*" được seeder mở rộng) -> không cần khớp active_org_id,
		// vì họ không "mượn" quyền của 1 tổ chức cụ thể nào, họ có quyền trên MỌI tổ chức.
		systemPerms, err := pc.resolveSystemOnlyPermissions(c.Context(), userID)
		if err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
				"success": false,
				"message": "failed to resolve permissions",
			})
		}
		if hasAllPermissions(systemPerms, []string{orgPermission}) {
			return c.Next()
		}

		// Còn lại: orgPermission chỉ đến từ org role -> bắt buộc active_org_id (JWT) khớp
		// đúng tổ chức trên URL, tránh ORG_OWNER của tổ chức A dùng quyền của mình sửa tổ chức B.
		targetOrgID, err := uuid.Parse(c.Params(paramName))
		if err != nil {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
				"success": false,
				"message": "invalid " + paramName,
			})
		}

		if activeOrgID == nil || *activeOrgID != targetOrgID {
			return c.Status(fiber.StatusForbidden).JSON(fiber.Map{
				"success": false,
				"message": "forbidden: active organization does not match",
			})
		}

		return c.Next()
	}
}
