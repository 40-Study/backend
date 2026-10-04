package service

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"study.com/v1/data"
	"study.com/v1/internal/dto"
	"study.com/v1/internal/model"
	"study.com/v1/internal/repository"
	"study.com/v1/internal/utils"
)

// ErrNotRoleOrgMember (C-03 residual, audit 260909 vòng 2): route "/org-roles/:id" dùng :id
// là Role.ID chứ không phải Organization.ID nên RequireOrgPermission (router) không đối
// chiếu được organization_id của role với active_org_id. Một ORG_OWNER của tổ chức A (có
// permission ORG_ROLES_MANAGE trong tổ chức mình) vẫn qua được middleware khi gọi PUT/DELETE
// "/org-roles/:id" với :id là role của tổ chức B — chỉ bị chặn ở đây, tầng service, sau khi
// đã fetch role và biết được role.OrganizationID thật.
//
// S6: mọi route /org-roles/* (đọc, tạo, khôi phục, đổi quyền) đều đi qua kiểm tra này, không chỉ
// Update/Delete như trước.
var ErrNotRoleOrgMember = errors.New("forbidden: role does not belong to your organization")

// ErrPermissionNotOrgScope (S6): org role chỉ được mang quyền THUỘC PHẠM VI TỔ CHỨC. Trước đây
// PUT /org-roles/:id/permissions nhận mọi permission id tồn tại, nên chủ tổ chức gán được
// SYSTEM_SETTINGS_MANAGE/PAYMENTS_MANAGE... cho role của mình rồi tự nhận role đó và thành admin nền tảng.
var ErrPermissionNotOrgScope = errors.New("forbidden: permission is outside organization scope")

// ErrRoleInUse (B-16): role tổ chức còn thành viên đang được gán nên không xoá được. Handler trả 409.
var ErrRoleInUse = errors.New("role is still assigned to members; revoke it from them first")

type RoleServiceInterface interface {
	// Mọi hàm nhận (activeOrgID, isAdmin) của người gọi: admin (SYSTEM_SETTINGS_MANAGE) làm việc trên
	// mọi tổ chức, người khác chỉ trong tổ chức đang active của mình (S6).
	CreateRole(ctx context.Context, activeOrgID *uuid.UUID, isAdmin bool, req dto.CreateRoleDTO) (*dto.RoleResponseDTO, error)
	GetRoleByID(ctx context.Context, id uuid.UUID, activeOrgID *uuid.UUID, isAdmin bool) (*dto.RoleDetailResponseDTO, error)
	GetAllRoles(ctx context.Context, page, pageSize int, keyword string, status string, organizationID *uuid.UUID, activeOrgID *uuid.UUID, isAdmin bool) (*dto.RoleListResponseDTO, error)
	UpdateRole(ctx context.Context, id uuid.UUID, activeOrgID *uuid.UUID, isAdmin bool, req dto.UpdateRoleDTO) (*dto.RoleResponseDTO, error)
	DeleteRole(ctx context.Context, id uuid.UUID, activeOrgID *uuid.UUID, isAdmin, hardDelete bool) error
	RestoreRole(ctx context.Context, id uuid.UUID, activeOrgID *uuid.UUID, isAdmin bool) error

	// Role-Permission management
	AddPermissionsToRole(ctx context.Context, roleID uuid.UUID, activeOrgID *uuid.UUID, isAdmin bool, req dto.AddPermissionsToRoleDTO) error
	RemovePermissionsFromRole(ctx context.Context, roleID uuid.UUID, activeOrgID *uuid.UUID, isAdmin bool, req dto.RemovePermissionsFromRoleDTO) error
	SetRolePermissions(ctx context.Context, roleID uuid.UUID, activeOrgID *uuid.UUID, isAdmin bool, req dto.AddPermissionsToRoleDTO) error
	GetRolePermissions(ctx context.Context, roleID uuid.UUID, activeOrgID *uuid.UUID, isAdmin bool) ([]dto.PermissionResponseDTO, error)
}

type RoleService struct {
	repo           repository.RoleRepositoryInterface
	permissionRepo repository.PermissionRepositoryInterface
}

func NewRoleService(repo repository.RoleRepositoryInterface, permissionRepo repository.PermissionRepositoryInterface) *RoleService {
	return &RoleService{repo: repo, permissionRepo: permissionRepo}
}

func (s *RoleService) CreateRole(ctx context.Context, activeOrgID *uuid.UUID, isAdmin bool, req dto.CreateRoleDTO) (*dto.RoleResponseDTO, error) {
	// S6: organization_id đến từ BODY nên router không kiểm được; trước đây chủ tổ chức A tạo được
	// role trong tổ chức B. Cùng luật với gán org role (requireOrgMatch, H-01).
	if err := requireOrgMatch(req.OrganizationID, activeOrgID, isAdmin); err != nil {
		return nil, ErrNotRoleOrgMember
	}

	role := &model.Role{
		Name:           req.Name,
		OrganizationID: &req.OrganizationID,
	}
	if req.Description != "" {
		role.Description.String = req.Description
		role.Description.Valid = true
	}

	if err := s.repo.CreateRole(ctx, role); err != nil {
		return nil, err
	}

	return toRoleResponseDTO(role), nil
}

func (s *RoleService) GetRoleByID(ctx context.Context, id uuid.UUID, activeOrgID *uuid.UUID, isAdmin bool) (*dto.RoleDetailResponseDTO, error) {
	role, err := s.loadRoleForActor(ctx, id, activeOrgID, isAdmin)
	if err != nil {
		return nil, err
	}

	permissions, err := s.repo.GetPermissionsByRoleID(ctx, role.ID)
	if err != nil {
		return nil, err
	}
	role.Permissions = permissions

	return toRoleDetailResponseDTO(role), nil
}

func (s *RoleService) GetAllRoles(ctx context.Context, page, pageSize int, keyword string, status string, organizationID *uuid.UUID, activeOrgID *uuid.UUID, isAdmin bool) (*dto.RoleListResponseDTO, error) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}

	// S6: trước đây ai có ORG_ROLES_MANAGE cũng liệt kê được role của MỌI tổ chức (bỏ qua
	// organization_id thì lấy hết). Người không phải admin luôn bị ép về tổ chức đang active.
	if !isAdmin {
		if activeOrgID == nil {
			return nil, ErrNotRoleOrgMember
		}
		if organizationID != nil && *organizationID != *activeOrgID {
			return nil, ErrNotRoleOrgMember
		}
		organizationID = activeOrgID
	}

	roles, total, err := s.repo.GetAllRoles(ctx, page, pageSize, keyword, status, organizationID)
	if err != nil {
		return nil, err
	}

	roleDTOs := make([]dto.RoleResponseDTO, len(roles))
	for i, role := range roles {
		roleDTOs[i] = *toRoleResponseDTO(&role)
	}

	return &dto.RoleListResponseDTO{
		Roles:    roleDTOs,
		Total:    total,
		Page:     page,
		PageSize: pageSize,
	}, nil
}

// requireRoleOrgMatch (C-03 residual): admin (đã xác thực SYSTEM_SETTINGS_MANAGE ở handler)
// bỏ qua kiểm tra; còn lại bắt buộc role.OrganizationID khớp đúng activeOrgID trong JWT.
func requireRoleOrgMatch(role *model.Role, activeOrgID *uuid.UUID, isAdmin bool) error {
	if isAdmin {
		return nil
	}
	if role.OrganizationID == nil || activeOrgID == nil || *role.OrganizationID != *activeOrgID {
		return ErrNotRoleOrgMember
	}
	return nil
}

// loadRoleForActor tải role rồi bắt buộc nó thuộc tổ chức của người gọi (S6): điểm vào chung của mọi
// route /org-roles/:id/* để không route nào quên kiểm tổ chức như Add/Set/RemovePermissions trước đây.
func (s *RoleService) loadRoleForActor(ctx context.Context, id uuid.UUID, activeOrgID *uuid.UUID, isAdmin bool) (*model.Role, error) {
	role, err := s.repo.GetRoleByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if role == nil {
		return nil, errors.New("role not found")
	}
	if err := requireRoleOrgMatch(role, activeOrgID, isAdmin); err != nil {
		return nil, err
	}
	return role, nil
}

func (s *RoleService) UpdateRole(ctx context.Context, id uuid.UUID, activeOrgID *uuid.UUID, isAdmin bool, req dto.UpdateRoleDTO) (*dto.RoleResponseDTO, error) {
	role, err := s.loadRoleForActor(ctx, id, activeOrgID, isAdmin)
	if err != nil {
		return nil, err
	}

	if req.Name != nil {
		role.Name = *req.Name
	}

	if req.Description != nil {
		role.Description.String = *req.Description
		role.Description.Valid = true
	}

	if err := s.repo.UpdateRole(ctx, role); err != nil {
		return nil, err
	}

	return toRoleResponseDTO(role), nil
}

func (s *RoleService) DeleteRole(ctx context.Context, id uuid.UUID, activeOrgID *uuid.UUID, isAdmin, hardDelete bool) error {
	if _, err := s.loadRoleForActor(ctx, id, activeOrgID, isAdmin); err != nil {
		return err
	}

	// B-16: xoá role còn người đang giữ trước đây trả 200 và để lại bản ghi gán "active" trỏ vào role đã mất
	// (treo, người đó vẫn đăng nhập chọn được vai trò rỗng). Phải gỡ vai trò khỏi các thành viên trước.
	// W2-A: đếm và xoá trong MỘT transaction có khoá dòng role. Đếm rồi xoá ở hai câu lệnh rời nhau thì một lượt gán
	// role song song chen vào giữa vẫn để lại bản ghi gán "active" trỏ vào role đã mất.
	assigned, err := s.repo.DeleteRoleIfUnused(ctx, id, hardDelete)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return errors.New("role not found")
	}
	if err != nil {
		return err
	}
	if assigned > 0 {
		return fmt.Errorf("%w: %d", ErrRoleInUse, assigned)
	}
	return nil
}

func (s *RoleService) RestoreRole(ctx context.Context, id uuid.UUID, activeOrgID *uuid.UUID, isAdmin bool) error {
	// Role đang bị xoá mềm nên phải tra cả dòng đã xoá; trước S6 route này không kiểm tổ chức nào.
	role, err := s.repo.GetRoleByIDIncludingDeleted(ctx, id)
	if err != nil {
		return err
	}
	if role == nil {
		return errors.New("role not found")
	}
	if err := requireRoleOrgMatch(role, activeOrgID, isAdmin); err != nil {
		return err
	}
	return s.repo.RestoreRole(ctx, id)
}

// ============ Role-Permission Management ============

// requireOrgScopePermissions kiểm mọi permission id tồn tại VÀ thuộc phạm vi tổ chức
// (data.IsOrgPermission, SSOT org_owner_permissions.json). Áp cho cả admin: org role không bao giờ
// được mang quyền hệ thống, vì quyền của org role được gộp vào quyền người dùng theo active_org_id.
func (s *RoleService) requireOrgScopePermissions(ctx context.Context, permissionIDs []uuid.UUID) error {
	perms, err := s.permissionRepo.GetPermissionsByIDs(ctx, permissionIDs)
	if err != nil {
		return err
	}
	if len(perms) != len(permissionIDs) {
		return errors.New("one or more permission IDs do not exist")
	}
	for _, p := range perms {
		if !data.IsOrgPermission(p.Name) {
			return fmt.Errorf("%w: %s", ErrPermissionNotOrgScope, p.Name)
		}
	}
	return nil
}

func (s *RoleService) AddPermissionsToRole(ctx context.Context, roleID uuid.UUID, activeOrgID *uuid.UUID, isAdmin bool, req dto.AddPermissionsToRoleDTO) error {
	if _, err := s.loadRoleForActor(ctx, roleID, activeOrgID, isAdmin); err != nil {
		return err
	}
	if err := s.requireOrgScopePermissions(ctx, req.PermissionIDs); err != nil {
		return err
	}

	return s.repo.AddPermissionsToRole(ctx, roleID, req.PermissionIDs)
}

// RemovePermissionsFromRole không giới hạn phạm vi quyền: gỡ luôn được (kể cả quyền hệ thống còn sót
// từ trước S6) — chỉ cần role thuộc tổ chức của người gọi.
func (s *RoleService) RemovePermissionsFromRole(ctx context.Context, roleID uuid.UUID, activeOrgID *uuid.UUID, isAdmin bool, req dto.RemovePermissionsFromRoleDTO) error {
	if _, err := s.loadRoleForActor(ctx, roleID, activeOrgID, isAdmin); err != nil {
		return err
	}

	return s.repo.RemovePermissionsFromRole(ctx, roleID, req.PermissionIDs)
}

func (s *RoleService) SetRolePermissions(ctx context.Context, roleID uuid.UUID, activeOrgID *uuid.UUID, isAdmin bool, req dto.AddPermissionsToRoleDTO) error {
	if _, err := s.loadRoleForActor(ctx, roleID, activeOrgID, isAdmin); err != nil {
		return err
	}
	if err := s.requireOrgScopePermissions(ctx, req.PermissionIDs); err != nil {
		return err
	}

	return s.repo.SetRolePermissions(ctx, roleID, req.PermissionIDs)
}

func (s *RoleService) GetRolePermissions(ctx context.Context, roleID uuid.UUID, activeOrgID *uuid.UUID, isAdmin bool) ([]dto.PermissionResponseDTO, error) {
	if _, err := s.loadRoleForActor(ctx, roleID, activeOrgID, isAdmin); err != nil {
		return nil, err
	}

	permissions, err := s.repo.GetPermissionsByRoleID(ctx, roleID)
	if err != nil {
		return nil, err
	}
	if permissions == nil {
		return nil, errors.New("role not found")
	}

	permDTOs := make([]dto.PermissionResponseDTO, len(permissions))
	for i, perm := range permissions {
		permDTOs[i] = *toPermissionResponseDTO(&perm)
	}

	return permDTOs, nil
}

func toRoleResponseDTO(role *model.Role) *dto.RoleResponseDTO {
	var desc *string
	if role.Description.Valid {
		desc = &role.Description.String
	}

	return &dto.RoleResponseDTO{
		ID:             role.ID,
		Name:           role.Name,
		OrganizationID: role.OrganizationID,
		Description:    desc,
		Status:         role.Status,
		CreatedAt:      utils.FormatTimestamp(role.CreatedAt),
		UpdatedAt:      utils.FormatTimestamp(role.UpdatedAt),
	}
}

func toRoleDetailResponseDTO(role *model.Role) *dto.RoleDetailResponseDTO {
	var desc *string
	if role.Description.Valid {
		desc = &role.Description.String
	}

	permDTOs := make([]dto.PermissionResponseDTO, len(role.Permissions))
	for i, perm := range role.Permissions {
		permDTOs[i] = *toPermissionResponseDTO(&perm)
	}

	return &dto.RoleDetailResponseDTO{
		ID:             role.ID,
		Name:           role.Name,
		OrganizationID: role.OrganizationID,
		Description:    desc,
		Status:         role.Status,
		Permissions:    permDTOs,
		CreatedAt:      utils.FormatTimestamp(role.CreatedAt),
		UpdatedAt:      utils.FormatTimestamp(role.UpdatedAt),
	}
}
