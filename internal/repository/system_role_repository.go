package repository

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"study.com/v1/internal/model"
)

type SystemRoleRepositoryInterface interface {
	CreateSystemRole(ctx context.Context, role *model.SystemRole) error
	GetSystemRoleByID(ctx context.Context, id uuid.UUID) (*model.SystemRole, error)
	GetSystemRoleByIDs(ctx context.Context, ids []uuid.UUID) ([]model.SystemRole, error)
	GetSystemRoleByName(ctx context.Context, name string) (*model.SystemRole, error)
	GetAllSystemRoles(ctx context.Context, page, pageSize int, keyword string, status string) ([]model.SystemRole, int64, error)
	UpdateSystemRole(ctx context.Context, role *model.SystemRole) error
	DeleteSystemRole(ctx context.Context, id uuid.UUID, hardDelete bool) error

	// SystemRole-Permission management
	AddPermissionsToSystemRole(ctx context.Context, roleID uuid.UUID, permissionIDs []uuid.UUID) error
	RemovePermissionsFromSystemRole(ctx context.Context, roleID uuid.UUID, permissionIDs []uuid.UUID) error
	GetPermissionsBySystemRoleID(ctx context.Context, roleID uuid.UUID) ([]model.Permission, error)
	GetSystemRoleWithPermissions(ctx context.Context, roleID uuid.UUID) (*model.SystemRole, error)
	SetSystemRolePermissions(ctx context.Context, roleID uuid.UUID, permissionIDs []uuid.UUID) error
}

type SystemRoleRepository struct {
	db *gorm.DB
}

func NewSystemRoleRepository(db *gorm.DB) *SystemRoleRepository {
	return &SystemRoleRepository{db: db}
}

func (r *SystemRoleRepository) CreateSystemRole(ctx context.Context, role *model.SystemRole) error {
	return r.db.WithContext(ctx).Create(role).Error
}

func (r *SystemRoleRepository) GetSystemRoleByID(ctx context.Context, id uuid.UUID) (*model.SystemRole, error) {
	var role model.SystemRole
	err := r.db.WithContext(ctx).Where("id = ?", id).First(&role).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &role, nil
}

// GetSystemRoleByIDs finds multiple system roles by IDs (single query)
func (r *SystemRoleRepository) GetSystemRoleByIDs(ctx context.Context, ids []uuid.UUID) ([]model.SystemRole, error) {
	if len(ids) == 0 {
		return []model.SystemRole{}, nil
	}
	var roles []model.SystemRole
	err := r.db.WithContext(ctx).Where("id IN ?", ids).Find(&roles).Error
	if err != nil {
		return nil, err
	}
	return roles, nil
}

func (r *SystemRoleRepository) GetSystemRoleByName(ctx context.Context, name string) (*model.SystemRole, error) {
	var role model.SystemRole
	err := r.db.WithContext(ctx).Where("name = ?", name).First(&role).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &role, nil
}

func (r *SystemRoleRepository) GetAllSystemRoles(ctx context.Context, page, pageSize int, keyword string, status string) ([]model.SystemRole, int64, error) {
	var roles []model.SystemRole
	var total int64

	offset := (page - 1) * pageSize

	query := r.db.WithContext(ctx).Model(&model.SystemRole{})
	switch status {
	case "deleted":
		query = query.Unscoped().Where("deleted_at IS NOT NULL")
	case "all":
		query = query.Unscoped()
	default:
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

func (r *SystemRoleRepository) UpdateSystemRole(ctx context.Context, role *model.SystemRole) error {
	return r.db.WithContext(ctx).Save(role).Error
}

func (r *SystemRoleRepository) DeleteSystemRole(ctx context.Context, id uuid.UUID, hardDelete bool) error {
	if hardDelete {
		return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
			if err := tx.Where("system_role_id = ?", id).Delete(&model.SystemRolePermission{}).Error; err != nil {
				return err
			}
			return tx.Unscoped().Delete(&model.SystemRole{}, "id = ?", id).Error
		})
	}
	return r.db.WithContext(ctx).Delete(&model.SystemRole{}, "id = ?", id).Error
}

// ============ SystemRole-Permission Management ============

func (r *SystemRoleRepository) AddPermissionsToSystemRole(ctx context.Context, roleID uuid.UUID, permissionIDs []uuid.UUID) error {
	rolePermissions := make([]model.SystemRolePermission, len(permissionIDs))
	for i, permID := range permissionIDs {
		rolePermissions[i] = model.SystemRolePermission{
			SystemRoleID: roleID,
			PermissionID: permID,
		}
	}
	return r.db.WithContext(ctx).Create(&rolePermissions).Error
}

func (r *SystemRoleRepository) RemovePermissionsFromSystemRole(ctx context.Context, roleID uuid.UUID, permissionIDs []uuid.UUID) error {
	return r.db.WithContext(ctx).
		Where("system_role_id = ? AND permission_id IN ?", roleID, permissionIDs).
		Delete(&model.SystemRolePermission{}).Error
}

func (r *SystemRoleRepository) GetPermissionsBySystemRoleID(ctx context.Context, roleID uuid.UUID) ([]model.Permission, error) {
	role, err := r.GetSystemRoleByID(ctx, roleID)
	if err != nil {
		return nil, err
	}
	if role == nil {
		return nil, nil
	}

	var permissions []model.Permission
	err = r.db.WithContext(ctx).
		Joins("JOIN system_role_permissions ON system_role_permissions.permission_id = permissions.id").
		Where("system_role_permissions.system_role_id = ?", roleID).
		Find(&permissions).Error
	if err != nil {
		return nil, err
	}

	return permissions, nil
}

func (r *SystemRoleRepository) GetSystemRoleWithPermissions(ctx context.Context, roleID uuid.UUID) (*model.SystemRole, error) {
	role, err := r.GetSystemRoleByID(ctx, roleID)
	if err != nil {
		return nil, err
	}
	if role == nil {
		return nil, nil
	}

	permissions, err := r.GetPermissionsBySystemRoleID(ctx, roleID)
	if err != nil {
		return nil, err
	}
	role.Permissions = permissions

	return role, nil
}

func (r *SystemRoleRepository) SetSystemRolePermissions(ctx context.Context, roleID uuid.UUID, permissionIDs []uuid.UUID) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("system_role_id = ?", roleID).Delete(&model.SystemRolePermission{}).Error; err != nil {
			return err
		}

		if len(permissionIDs) > 0 {
			rolePermissions := make([]model.SystemRolePermission, len(permissionIDs))
			for i, permID := range permissionIDs {
				rolePermissions[i] = model.SystemRolePermission{
					SystemRoleID: roleID,
					PermissionID: permID,
				}
			}
			return tx.Create(&rolePermissions).Error
		}

		return nil
	})
}
