package repository

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"study.com/v1/internal/model"
)

type RoleRepositoryInterface interface {
	// Role CRUD
	CreateRole(ctx context.Context, role *model.Role) error
	GetRoleByID(ctx context.Context, id uuid.UUID) (*model.Role, error)
	GetRoleByName(ctx context.Context, name string) (*model.Role, error)
	GetAllRoles(ctx context.Context, page, pageSize int, keyword string, status string) ([]model.Role, int64, error)
	UpdateRole(ctx context.Context, role *model.Role) error
	DeleteRole(ctx context.Context, id uuid.UUID, hardDelete bool) error

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

func (r *RoleRepository) GetAllRoles(ctx context.Context, page, pageSize int, keyword string, status string) ([]model.Role, int64, error) {
	var roles []model.Role
	var total int64

	offset := (page - 1) * pageSize

	query := r.db.WithContext(ctx).Model(&model.Role{})
	switch status {
	case "deleted":
		query = query.Unscoped().Where("deleted_at IS NOT NULL")
	case "all":
		query = query.Unscoped()
	default: // "active" or empty
	}
	if keyword != "" {
		query = query.Where("name ILIKE ?", "%"+keyword+"%")
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
	query := r.db.WithContext(ctx)
	if hardDelete {
		query = query.Unscoped()
	}
	return query.Delete(&model.Role{}, "id = ?", id).Error
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
	// Kiểm tra role tồn tại
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
	// Xóa tất cả permission cũ của role
	if err := r.db.WithContext(ctx).
		Where("role_id = ?", roleID).
		Delete(&model.RolePermission{}).Error; err != nil {
		return err
	}

	// Thêm permission mới
	if len(permissionIDs) > 0 {
		rolePermissions := make([]model.RolePermission, len(permissionIDs))
		for i, permID := range permissionIDs {
			rolePermissions[i] = model.RolePermission{
				RoleID:       roleID,
				PermissionID: permID,
			}
		}
		return r.db.WithContext(ctx).Create(&rolePermissions).Error
	}

	return nil
}
