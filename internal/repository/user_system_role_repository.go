package repository

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"study.com/v1/internal/model"
)

type UserSystemRoleRepositoryInterface interface {
	// Các thao tác CRUD
	Create(ctx context.Context, userSystemRole *model.UserSystemRole) error
	CreateBatch(ctx context.Context, userSystemRoles []model.UserSystemRole) error
	FindByID(ctx context.Context, id uuid.UUID) (*model.UserSystemRole, error)
	FindByIDWithDetails(ctx context.Context, id uuid.UUID) (*model.UserSystemRole, error)
	Update(ctx context.Context, userSystemRole *model.UserSystemRole) error
	Delete(ctx context.Context, id uuid.UUID) error
	HardDelete(ctx context.Context, id uuid.UUID) error

	// Các thao tác truy vấn
	FindByUserID(ctx context.Context, userID uuid.UUID, status string) ([]model.UserSystemRole, error)
	FindByUserIDWithDetails(ctx context.Context, userID uuid.UUID, status string) ([]model.UserSystemRole, error)
	FindBySystemRoleID(ctx context.Context, systemRoleID uuid.UUID, page, pageSize int, status string) ([]model.UserSystemRole, int64, error)
	FindByUserAndSystemRole(ctx context.Context, userID, systemRoleID uuid.UUID) (*model.UserSystemRole, error)
	ExistsByUserAndSystemRole(ctx context.Context, userID, systemRoleID uuid.UUID) (bool, error)

	// Các thao tác trạng thái
	UpdateStatus(ctx context.Context, id uuid.UUID, status string, revokedBy *uuid.UUID) error

	// Các thao tác hàng loạt
	DeleteByUserID(ctx context.Context, userID uuid.UUID) error
	DeleteBySystemRoleID(ctx context.Context, systemRoleID uuid.UUID) error
}

type UserSystemRoleRepository struct {
	db *gorm.DB
}

func NewUserSystemRoleRepository(db *gorm.DB) *UserSystemRoleRepository {
	return &UserSystemRoleRepository{db: db}
}

// ============ Các Thao Tác CRUD ============

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

// ============ Các Thao Tác Truy Vấn ============

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

func (r *UserSystemRoleRepository) ExistsByUserAndSystemRole(ctx context.Context, userID, systemRoleID uuid.UUID) (bool, error) {
	var count int64
	// Sử dụng Unscoped() để bao gồm các bản ghi đã xóa mềm vì ràng buộc duy nhất bao gồm chúng
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

// ============ Các Thao Tác Trạng Thái ============

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

// ============ Các Thao Tác Hàng Loạt ============

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
