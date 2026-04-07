package repository

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"study.com/v1/internal/model"
)

type UserSystemRoleRepositoryInterface interface {
	// CRUD
	Create(ctx context.Context, userSystemRole *model.UserSystemRole) error
	CreateBatch(ctx context.Context, userSystemRoles []model.UserSystemRole) error
	FindByID(ctx context.Context, id uuid.UUID) (*model.UserSystemRole, error)
	FindByIDWithDetails(ctx context.Context, id uuid.UUID) (*model.UserSystemRole, error)
	Update(ctx context.Context, userSystemRole *model.UserSystemRole) error
	Delete(ctx context.Context, id uuid.UUID) error
	HardDelete(ctx context.Context, id uuid.UUID) error

	// Truy van
	FindByUserID(ctx context.Context, userID uuid.UUID, status string) ([]model.UserSystemRole, error)
	FindByUserIDWithDetails(ctx context.Context, userID uuid.UUID, status string) ([]model.UserSystemRole, error)
	FindBySystemRoleID(ctx context.Context, systemRoleID uuid.UUID, page, pageSize int, status string) ([]model.UserSystemRole, int64, error)
	FindByUserAndSystemRole(ctx context.Context, userID, systemRoleID uuid.UUID) (*model.UserSystemRole, error)
	FindByUserAndSystemRoleIDs(ctx context.Context, userID uuid.UUID, systemRoleIDs []uuid.UUID) ([]model.UserSystemRole, error)
	ExistsByUserAndSystemRole(ctx context.Context, userID, systemRoleID uuid.UUID) (bool, error)

	// Trang thai
	UpdateStatus(ctx context.Context, id uuid.UUID, status string, revokedBy *uuid.UUID) error

	// Hang loat
	DeleteByUserID(ctx context.Context, userID uuid.UUID) error
	DeleteBySystemRoleID(ctx context.Context, systemRoleID uuid.UUID) error

	// Transaction
	AssignRolesWithTx(ctx context.Context, toReactivate []*model.UserSystemRole, toCreate []model.UserSystemRole) error
	GetDefaultSystemRoleID(ctx context.Context) (uuid.UUID, error)
}

type UserSystemRoleRepository struct {
	db *gorm.DB
}

func NewUserSystemRoleRepository(db *gorm.DB) *UserSystemRoleRepository {
	return &UserSystemRoleRepository{db: db}
}

// CRUD

func (r *UserSystemRoleRepository) Create(ctx context.Context, userSystemRole *model.UserSystemRole) error {
	return r.db.WithContext(ctx).Create(userSystemRole).Error
}

func (r *UserSystemRoleRepository) CreateBatch(ctx context.Context, userSystemRoles []model.UserSystemRole) error {
	if len(userSystemRoles) == 0 {
		return nil
	}
	return r.db.WithContext(ctx).Create(&userSystemRoles).Error
}

func (r *UserSystemRoleRepository) FindByID(ctx context.Context, id uuid.UUID) (*model.UserSystemRole, error) {
	var userSystemRole model.UserSystemRole
	err := r.db.WithContext(ctx).Where("id = ?", id).First(&userSystemRole).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &userSystemRole, nil
}

func (r *UserSystemRoleRepository) FindByIDWithDetails(ctx context.Context, id uuid.UUID) (*model.UserSystemRole, error) {
	var userSystemRole model.UserSystemRole
	err := r.db.WithContext(ctx).
		Preload("User").
		Preload("SystemRole").
		Preload("Granter").
		Preload("Revoker").
		Where("id = ?", id).
		First(&userSystemRole).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &userSystemRole, nil
}

func (r *UserSystemRoleRepository) Update(ctx context.Context, userSystemRole *model.UserSystemRole) error {
	return r.db.WithContext(ctx).Save(userSystemRole).Error
}

func (r *UserSystemRoleRepository) Delete(ctx context.Context, id uuid.UUID) error {
	return r.db.WithContext(ctx).Delete(&model.UserSystemRole{}, "id = ?", id).Error
}

func (r *UserSystemRoleRepository) HardDelete(ctx context.Context, id uuid.UUID) error {
	return r.db.WithContext(ctx).Unscoped().Delete(&model.UserSystemRole{}, "id = ?", id).Error
}

// Truy van

func (r *UserSystemRoleRepository) FindByUserID(ctx context.Context, userID uuid.UUID, status string) ([]model.UserSystemRole, error) {
	var userSystemRoles []model.UserSystemRole
	query := r.db.WithContext(ctx).Where("user_id = ?", userID)

	if status != "" {
		query = query.Where("status = ?", status)
	}

	err := query.Order("granted_at DESC").Find(&userSystemRoles).Error
	if err != nil {
		return nil, err
	}
	return userSystemRoles, nil
}

func (r *UserSystemRoleRepository) FindByUserIDWithDetails(ctx context.Context, userID uuid.UUID, status string) ([]model.UserSystemRole, error) {
	var userSystemRoles []model.UserSystemRole
	query := r.db.WithContext(ctx).
		Preload("SystemRole").
		Preload("Granter").
		Where("user_id = ?", userID)

	if status != "" {
		query = query.Where("status = ?", status)
	}

	err := query.Order("granted_at DESC").Find(&userSystemRoles).Error
	if err != nil {
		return nil, err
	}
	return userSystemRoles, nil
}

func (r *UserSystemRoleRepository) FindBySystemRoleID(ctx context.Context, systemRoleID uuid.UUID, page, pageSize int, status string) ([]model.UserSystemRole, int64, error) {
	var userSystemRoles []model.UserSystemRole
	var total int64

	offset := (page - 1) * pageSize

	query := r.db.WithContext(ctx).Model(&model.UserSystemRole{}).Where("system_role_id = ?", systemRoleID)

	if status != "" {
		query = query.Where("status = ?", status)
	}

	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	err := query.
		Preload("User").
		Preload("Granter").
		Offset(offset).
		Limit(pageSize).
		Order("granted_at DESC").
		Find(&userSystemRoles).Error
	if err != nil {
		return nil, 0, err
	}

	return userSystemRoles, total, nil
}

func (r *UserSystemRoleRepository) FindByUserAndSystemRole(ctx context.Context, userID, systemRoleID uuid.UUID) (*model.UserSystemRole, error) {
	var userSystemRole model.UserSystemRole
	err := r.db.WithContext(ctx).
		Where("user_id = ? AND system_role_id = ?", userID, systemRoleID).
		First(&userSystemRole).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &userSystemRole, nil
}

// FindByUserAndSystemRoleIDs tim cac mapping theo user va danh sach role IDs
// Bao gom ca soft-deleted de kiem tra reactivation
func (r *UserSystemRoleRepository) FindByUserAndSystemRoleIDs(ctx context.Context, userID uuid.UUID, systemRoleIDs []uuid.UUID) ([]model.UserSystemRole, error) {
	if len(systemRoleIDs) == 0 {
		return []model.UserSystemRole{}, nil
	}
	var userSystemRoles []model.UserSystemRole
	err := r.db.WithContext(ctx).
		Unscoped().
		Where("user_id = ? AND system_role_id IN ?", userID, systemRoleIDs).
		Find(&userSystemRoles).Error
	if err != nil {
		return nil, err
	}
	return userSystemRoles, nil
}

func (r *UserSystemRoleRepository) ExistsByUserAndSystemRole(ctx context.Context, userID, systemRoleID uuid.UUID) (bool, error) {
	var count int64
	err := r.db.WithContext(ctx).
		Unscoped().
		Model(&model.UserSystemRole{}).
		Where("user_id = ? AND system_role_id = ?", userID, systemRoleID).
		Count(&count).Error
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

// Trang thai

func (r *UserSystemRoleRepository) UpdateStatus(ctx context.Context, id uuid.UUID, status string, revokedBy *uuid.UUID) error {
	updates := map[string]interface{}{
		"status": status,
	}

	if status == "revoked" && revokedBy != nil {
		updates["revoked_by"] = revokedBy
		updates["revoked_at"] = gorm.Expr("NOW()")
	}

	return r.db.WithContext(ctx).
		Model(&model.UserSystemRole{}).
		Where("id = ?", id).
		Updates(updates).Error
}

// Hang loat

func (r *UserSystemRoleRepository) DeleteByUserID(ctx context.Context, userID uuid.UUID) error {
	return r.db.WithContext(ctx).
		Where("user_id = ?", userID).
		Delete(&model.UserSystemRole{}).Error
}

func (r *UserSystemRoleRepository) DeleteBySystemRoleID(ctx context.Context, systemRoleID uuid.UUID) error {
	return r.db.WithContext(ctx).
		Where("system_role_id = ?", systemRoleID).
		Delete(&model.UserSystemRole{}).Error
}

// Transaction

// AssignRolesWithTx reactivate va tao mappings trong 1 transaction
func (r *UserSystemRoleRepository) AssignRolesWithTx(ctx context.Context, toReactivate []*model.UserSystemRole, toCreate []model.UserSystemRole) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// Reactivate cac mapping inactive
		for _, mapping := range toReactivate {
			if err := tx.Save(mapping).Error; err != nil {
				return err
			}
		}

		// Batch create cac mapping moi
		if len(toCreate) > 0 {
			if err := tx.Create(&toCreate).Error; err != nil {
				return err
			}
		}

		return nil
	})
}

func (r *UserSystemRoleRepository) GetDefaultSystemRoleID(ctx context.Context) (uuid.UUID, error) {
	var systemRole model.SystemRole
	err := r.db.WithContext(ctx).Where("name = ?", "student").First(&systemRole).Error
	if err != nil {
		return uuid.Nil, err
	}
	return systemRole.ID, nil
}
