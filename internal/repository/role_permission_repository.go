package repository

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"study.com/v1/internal/model"
)

type RolePermissionRepositoryInterface interface {
	// Role CRUD
	CreateRole(ctx context.Context, role *model.Role) error
	GetRoleByID(ctx context.Context, id uuid.UUID) (*model.Role, error)
	GetRoleByName(ctx context.Context, name string) (*model.Role, error)
	GetAllRoles(ctx context.Context, page, pageSize int) ([]model.Role, int64, error)
	UpdateRole(ctx context.Context, role *model.Role) error
	DeleteRole(ctx context.Context, id uuid.UUID) error

	// Permission CRUD
	CreatePermission(ctx context.Context, permission *model.Permission) error
	GetPermissionByID(ctx context.Context, id uuid.UUID) (*model.Permission, error)
	GetPermissionByName(ctx context.Context, name string) (*model.Permission, error)
	GetAllPermissions(ctx context.Context, page, pageSize int) ([]model.Permission, int64, error)
	UpdatePermission(ctx context.Context, permission *model.Permission) error
	DeletePermission(ctx context.Context, id uuid.UUID) error

	// Role-Permission management
	AddPermissionsToRole(ctx context.Context, roleID uuid.UUID, permissionIDs []uuid.UUID) error
	RemovePermissionsFromRole(ctx context.Context, roleID uuid.UUID, permissionIDs []uuid.UUID) error
	GetPermissionsByRoleID(ctx context.Context, roleID uuid.UUID) ([]model.Permission, error)
	GetRoleWithPermissions(ctx context.Context, roleID uuid.UUID) (*model.Role, error)
	SetRolePermissions(ctx context.Context, roleID uuid.UUID, permissionIDs []uuid.UUID) error
}

type RolePermissionRepository struct {
	db *gorm.DB
}

func NewRolePermissionRepository(db *gorm.DB) *RolePermissionRepository {
	return &RolePermissionRepository{db: db}
}

// ============ Role CRUD ============

func (r *RolePermissionRepository) CreateRole(ctx context.Context, role *model.Role) error {
	return r.db.WithContext(ctx).Create(role).Error
}

func (r *RolePermissionRepository) GetRoleByID(ctx context.Context, id uuid.UUID) (*model.Role, error) {
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

func (r *RolePermissionRepository) GetRoleByName(ctx context.Context, name string) (*model.Role, error) {
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

func (r *RolePermissionRepository) GetAllRoles(ctx context.Context, page, pageSize int) ([]model.Role, int64, error) {
	var roles []model.Role
	var total int64

	offset := (page - 1) * pageSize

	if err := r.db.WithContext(ctx).Model(&model.Role{}).Count(&total).Error; err != nil {
		return nil, 0, err
	}

	if err := r.db.WithContext(ctx).
		Offset(offset).
		Limit(pageSize).
		Order("created_at DESC").
		Find(&roles).Error; err != nil {
		return nil, 0, err
	}

	return roles, total, nil
}

func (r *RolePermissionRepository) UpdateRole(ctx context.Context, role *model.Role) error {
	return r.db.WithContext(ctx).Save(role).Error
}

func (r *RolePermissionRepository) DeleteRole(ctx context.Context, id uuid.UUID) error {
	return r.db.WithContext(ctx).Delete(&model.Role{}, "id = ?", id).Error
}

// ============ Permission CRUD ============

func (r *RolePermissionRepository) CreatePermission(ctx context.Context, permission *model.Permission) error {
	return r.db.WithContext(ctx).Create(permission).Error
}

func (r *RolePermissionRepository) GetPermissionByID(ctx context.Context, id uuid.UUID) (*model.Permission, error) {
	var permission model.Permission
	err := r.db.WithContext(ctx).Where("id = ?", id).First(&permission).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &permission, nil
}

func (r *RolePermissionRepository) GetPermissionByName(ctx context.Context, name string) (*model.Permission, error) {
	var permission model.Permission
	err := r.db.WithContext(ctx).Where("name = ?", name).First(&permission).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &permission, nil
}

func (r *RolePermissionRepository) GetAllPermissions(ctx context.Context, page, pageSize int) ([]model.Permission, int64, error) {
	var permissions []model.Permission
	var total int64

	offset := (page - 1) * pageSize

	if err := r.db.WithContext(ctx).Model(&model.Permission{}).Count(&total).Error; err != nil {
		return nil, 0, err
	}

	if err := r.db.WithContext(ctx).
		Offset(offset).
		Limit(pageSize).
		Order("created_at DESC").
		Find(&permissions).Error; err != nil {
		return nil, 0, err
	}

	return permissions, total, nil
}

func (r *RolePermissionRepository) UpdatePermission(ctx context.Context, permission *model.Permission) error {
	return r.db.WithContext(ctx).Save(permission).Error
}

func (r *RolePermissionRepository) DeletePermission(ctx context.Context, id uuid.UUID) error {
	return r.db.WithContext(ctx).Delete(&model.Permission{}, "id = ?", id).Error
}

// ============ Role-Permission Management ============

func (r *RolePermissionRepository) AddPermissionsToRole(ctx context.Context, roleID uuid.UUID, permissionIDs []uuid.UUID) error {
	role, err := r.GetRoleByID(ctx, roleID)
	if err != nil {
		return err
	}
	if role == nil {
		return errors.New("role not found")
	}

	var permissions []model.Permission
	if err := r.db.WithContext(ctx).Where("id IN ?", permissionIDs).Find(&permissions).Error; err != nil {
		return err
	}

	return r.db.WithContext(ctx).Model(role).Association("Permissions").Append(permissions)
}

func (r *RolePermissionRepository) RemovePermissionsFromRole(ctx context.Context, roleID uuid.UUID, permissionIDs []uuid.UUID) error {
	role, err := r.GetRoleByID(ctx, roleID)
	if err != nil {
		return err
	}
	if role == nil {
		return errors.New("role not found")
	}

	var permissions []model.Permission
	if err := r.db.WithContext(ctx).Where("id IN ?", permissionIDs).Find(&permissions).Error; err != nil {
		return err
	}

	return r.db.WithContext(ctx).Model(role).Association("Permissions").Delete(permissions)
}

func (r *RolePermissionRepository) GetPermissionsByRoleID(ctx context.Context, roleID uuid.UUID) ([]model.Permission, error) {
	var role model.Role
	err := r.db.WithContext(ctx).
		Preload("Permissions").
		Where("id = ?", roleID).
		First(&role).Error

	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}

	return role.Permissions, nil
}

func (r *RolePermissionRepository) GetRoleWithPermissions(ctx context.Context, roleID uuid.UUID) (*model.Role, error) {
	var role model.Role
	err := r.db.WithContext(ctx).
		Preload("Permissions").
		Where("id = ?", roleID).
		First(&role).Error

	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}

	return &role, nil
}

func (r *RolePermissionRepository) SetRolePermissions(ctx context.Context, roleID uuid.UUID, permissionIDs []uuid.UUID) error {
	role, err := r.GetRoleByID(ctx, roleID)
	if err != nil {
		return err
	}
	if role == nil {
		return errors.New("role not found")
	}

	var permissions []model.Permission
	if len(permissionIDs) > 0 {
		if err := r.db.WithContext(ctx).Where("id IN ?", permissionIDs).Find(&permissions).Error; err != nil {
			return err
		}
	}

	return r.db.WithContext(ctx).Model(role).Association("Permissions").Replace(permissions)
}
