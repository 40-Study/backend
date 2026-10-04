package repository

import (
	"context"
	"errors"
	"sort"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"study.com/v1/internal/model"
	"study.com/v1/internal/utils"
)

type RoleRepositoryInterface interface {
	// Role CRUD
	CreateRole(ctx context.Context, role *model.Role) error
	GetRoleByID(ctx context.Context, id uuid.UUID) (*model.Role, error)
	GetRoleByIDIncludingDeleted(ctx context.Context, id uuid.UUID) (*model.Role, error)
	GetRoleByIDs(ctx context.Context, ids []uuid.UUID) ([]model.Role, error)
	GetRoleByName(ctx context.Context, name string) (*model.Role, error)
	GetAllRoles(ctx context.Context, page, pageSize int, keyword string, status string, organizationID *uuid.UUID) ([]model.Role, int64, error)
	UpdateRole(ctx context.Context, role *model.Role) error
	DeleteRole(ctx context.Context, id uuid.UUID, hardDelete bool) error
	// DeleteRoleIfUnused (W2-A): xoá role CHỈ KHI không còn ai giữ, kiểm đếm và xoá trong MỘT transaction có khoá
	// dòng role (FOR UPDATE), đối ứng với khoá FOR SHARE của gán role (LockRolesForShare). Còn người giữ thì không
	// xoá gì và trả số người giữ (>0) để service báo ErrRoleInUse.
	DeleteRoleIfUnused(ctx context.Context, id uuid.UUID, hardDelete bool) (assigned int64, err error)
	// CountActiveAssignments (B-16): số người đang được gán role này (user_organization_roles.status = active).
	CountActiveAssignments(ctx context.Context, roleID uuid.UUID) (int64, error)
	RestoreRole(ctx context.Context, id uuid.UUID) error

	// Role-Permission management
	AddPermissionsToRole(ctx context.Context, roleID uuid.UUID, permissionIDs []uuid.UUID) error
	RemovePermissionsFromRole(ctx context.Context, roleID uuid.UUID, permissionIDs []uuid.UUID) error
	GetPermissionsByRoleID(ctx context.Context, roleID uuid.UUID) ([]model.Permission, error)
	GetRoleWithPermissions(ctx context.Context, roleID uuid.UUID) (*model.Role, error)
	SetRolePermissions(ctx context.Context, roleID uuid.UUID, permissionIDs []uuid.UUID) error
}

type RoleRepository struct {
	db *gorm.DB
}

func NewRoleRepository(db *gorm.DB) *RoleRepository {
	return &RoleRepository{db: db}
}

func (r *RoleRepository) CreateRole(ctx context.Context, role *model.Role) error {
	return r.db.WithContext(ctx).Create(role).Error
}

func (r *RoleRepository) GetRoleByID(ctx context.Context, id uuid.UUID) (*model.Role, error) {
	var role model.Role
	err := r.db.WithContext(ctx).Where("id = ?", id).First(&role).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &role, nil
}

// GetRoleByIDIncludingDeleted tìm cả role đã xoá mềm — RestoreRole cần biết role thuộc tổ chức
// nào TRƯỚC khi khôi phục (GetRoleByID bỏ qua dòng đã xoá nên không dùng được cho việc này).
func (r *RoleRepository) GetRoleByIDIncludingDeleted(ctx context.Context, id uuid.UUID) (*model.Role, error) {
	var role model.Role
	err := r.db.WithContext(ctx).Unscoped().Where("id = ?", id).First(&role).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &role, nil
}

func (r *RoleRepository) GetRoleByIDs(ctx context.Context, ids []uuid.UUID) ([]model.Role, error) {
	var roles []model.Role
	err := r.db.WithContext(ctx).
		Where("id IN ?", ids).
		Find(&roles).Error
	if err != nil {
		return nil, err
	}
	return roles, nil
}

func (r *RoleRepository) GetRoleByName(ctx context.Context, name string) (*model.Role, error) {
	var role model.Role
	err := r.db.WithContext(ctx).Where("name = ?", name).First(&role).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &role, nil
}

func (r *RoleRepository) GetAllRoles(ctx context.Context, page, pageSize int, keyword string, status string, organizationID *uuid.UUID) ([]model.Role, int64, error) {
	var roles []model.Role
	var total int64

	offset := (page - 1) * pageSize

	query := r.db.WithContext(ctx).Model(&model.Role{})
	switch status {
	case "deleted":
		query = query.Unscoped().Where("deleted_at IS NOT NULL")
	case "all":
		query = query.Unscoped()
	default:
	}
	if organizationID != nil {
		query = query.Where("organization_id = ?", *organizationID)
	}
	if keyword != "" {
		query = query.Where("name ILIKE ?", utils.ContainsLikePattern(keyword))
	}

	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	if err := query.
		Offset(offset).
		Limit(pageSize).
		Order("created_at DESC").
		Find(&roles).Error; err != nil {
		return nil, 0, err
	}

	return roles, total, nil
}

func (r *RoleRepository) UpdateRole(ctx context.Context, role *model.Role) error {
	return r.db.WithContext(ctx).Save(role).Error
}

func (r *RoleRepository) DeleteRole(ctx context.Context, id uuid.UUID, hardDelete bool) error {
	if hardDelete {
		return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
			if err := tx.Where("role_id = ?", id).Delete(&model.RolePermission{}).Error; err != nil {
				return err
			}
			return tx.Unscoped().Delete(&model.Role{}, "id = ?", id).Error
		})
	}
	return r.db.WithContext(ctx).Delete(&model.Role{}, "id = ?", id).Error
}

func (r *RoleRepository) DeleteRoleIfUnused(ctx context.Context, id uuid.UUID, hardDelete bool) (int64, error) {
	var assigned int64
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// Khoá dòng role: gán role song song (AssignRolesWithTx giữ FOR SHARE trên cùng dòng) phải chờ xoá xong hoặc
		// ngược lại, nên không thể chen một bản ghi gán "active" vào giữa lúc đếm và lúc xoá.
		var locked model.Role
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ?", id).First(&locked).Error; err != nil {
			return err
		}
		if err := tx.Model(&model.UserOrganizationRole{}).
			Where("role_id = ? AND status = ?", id, model.UserOrgRoleStatusActive).
			Count(&assigned).Error; err != nil {
			return err
		}
		if assigned > 0 {
			return nil
		}
		if hardDelete {
			if err := tx.Where("role_id = ?", id).Delete(&model.RolePermission{}).Error; err != nil {
				return err
			}
			return tx.Unscoped().Delete(&model.Role{}, "id = ?", id).Error
		}
		return tx.Delete(&model.Role{}, "id = ?", id).Error
	})
	return assigned, err
}

// LockRolesForShare khoá FOR SHARE các dòng role (theo thứ tự id để hai transaction không khoá chéo nhau) và
// trả lỗi nếu một role đã bị xoá (mềm hoặc cứng): gán vào role đã mất sẽ tạo bản ghi gán treo (B-16). Chạy trong
// transaction của người gọi; khoá giữ tới khi transaction kết thúc.
func LockRolesForShare(tx *gorm.DB, roleIDs []uuid.UUID) error {
	if len(roleIDs) == 0 {
		return nil
	}
	ids := append([]uuid.UUID(nil), roleIDs...)
	sort.Slice(ids, func(i, j int) bool { return ids[i].String() < ids[j].String() })
	var live []model.Role
	if err := tx.Clauses(clause.Locking{Strength: "SHARE"}).Where("id IN ?", ids).Order("id").Find(&live).Error; err != nil {
		return err
	}
	seen := make(map[uuid.UUID]struct{}, len(live))
	for _, r := range live {
		seen[r.ID] = struct{}{}
	}
	for _, id := range ids {
		if _, ok := seen[id]; !ok {
			return ErrRoleGone
		}
	}
	return nil
}

// ErrRoleGone: role định gán đã bị xoá giữa lúc kiểm và lúc gán.
var ErrRoleGone = errors.New("role no longer exists")

func (r *RoleRepository) CountActiveAssignments(ctx context.Context, roleID uuid.UUID) (int64, error) {
	var n int64
	err := r.db.WithContext(ctx).Model(&model.UserOrganizationRole{}).
		Where("role_id = ? AND status = ?", roleID, model.UserOrgRoleStatusActive).
		Count(&n).Error
	return n, err
}

func (r *RoleRepository) RestoreRole(ctx context.Context, id uuid.UUID) error {
	return r.db.WithContext(ctx).Unscoped().Model(&model.Role{}).Where("id = ?", id).Update("deleted_at", nil).Error
}

// ============ Role-Permission Management ============

func (r *RoleRepository) AddPermissionsToRole(ctx context.Context, roleID uuid.UUID, permissionIDs []uuid.UUID) error {
	rolePermissions := make([]model.RolePermission, len(permissionIDs))
	for i, permID := range permissionIDs {
		rolePermissions[i] = model.RolePermission{
			RoleID:       roleID,
			PermissionID: permID,
		}
	}
	return r.db.WithContext(ctx).Create(&rolePermissions).Error
}

func (r *RoleRepository) RemovePermissionsFromRole(ctx context.Context, roleID uuid.UUID, permissionIDs []uuid.UUID) error {
	return r.db.WithContext(ctx).
		Where("role_id = ? AND permission_id IN ?", roleID, permissionIDs).
		Delete(&model.RolePermission{}).Error
}

func (r *RoleRepository) GetPermissionsByRoleID(ctx context.Context, roleID uuid.UUID) ([]model.Permission, error) {
	role, err := r.GetRoleByID(ctx, roleID)
	if err != nil {
		return nil, err
	}
	if role == nil {
		return nil, nil
	}

	var permissions []model.Permission
	err = r.db.WithContext(ctx).
		Joins("JOIN role_permissions ON role_permissions.permission_id = permissions.id").
		Where("role_permissions.role_id = ?", roleID).
		Find(&permissions).Error
	if err != nil {
		return nil, err
	}

	return permissions, nil
}

func (r *RoleRepository) GetRoleWithPermissions(ctx context.Context, roleID uuid.UUID) (*model.Role, error) {
	role, err := r.GetRoleByID(ctx, roleID)
	if err != nil {
		return nil, err
	}
	if role == nil {
		return nil, nil
	}

	permissions, err := r.GetPermissionsByRoleID(ctx, roleID)
	if err != nil {
		return nil, err
	}
	role.Permissions = permissions

	return role, nil
}

func (r *RoleRepository) SetRolePermissions(ctx context.Context, roleID uuid.UUID, permissionIDs []uuid.UUID) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("role_id = ?", roleID).Delete(&model.RolePermission{}).Error; err != nil {
			return err
		}

		if len(permissionIDs) > 0 {
			rolePermissions := make([]model.RolePermission, len(permissionIDs))
			for i, permID := range permissionIDs {
				rolePermissions[i] = model.RolePermission{
					RoleID:       roleID,
					PermissionID: permID,
				}
			}
			return tx.Create(&rolePermissions).Error
		}

		return nil
	})
}
