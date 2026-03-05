package repository

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"study.com/v1/internal/model"
)

type UserOrganizationRoleRepositoryInterface interface {
	// CRUD
	Create(ctx context.Context, userOrgRole *model.UserOrganizationRole) error
	CreateBatch(ctx context.Context, userOrgRoles []model.UserOrganizationRole) error
	FindByID(ctx context.Context, id uuid.UUID) (*model.UserOrganizationRole, error)
	FindByIDWithDetails(ctx context.Context, id uuid.UUID) (*model.UserOrganizationRole, error)
	Update(ctx context.Context, userOrgRole *model.UserOrganizationRole) error
	Delete(ctx context.Context, id uuid.UUID) error
	HardDelete(ctx context.Context, id uuid.UUID) error

	// Truy van
	FindByUserID(ctx context.Context, userID uuid.UUID, status string) ([]model.UserOrganizationRole, error)
	FindByUserIDWithDetails(ctx context.Context, userID uuid.UUID, status string) ([]model.UserOrganizationRole, error)
	FindByRoleID(ctx context.Context, roleID uuid.UUID, page, pageSize int, status string) ([]model.UserOrganizationRole, int64, error)
	FindByRoleIDAndOrgID(ctx context.Context, roleID, organizationID uuid.UUID, page, pageSize int, status string) ([]model.UserOrganizationRole, int64, error)
	FindByOrganizationID(ctx context.Context, organizationID uuid.UUID, page, pageSize int, status string) ([]model.UserOrganizationRole, int64, error)
	FindByUserAndOrganization(ctx context.Context, userID, organizationID uuid.UUID, status string) ([]model.UserOrganizationRole, error)
	FindByUserRoleAndOrg(ctx context.Context, userID, roleID, organizationID uuid.UUID) (*model.UserOrganizationRole, error)
	FindByUserAndRoleIDsInOrg(ctx context.Context, userID, orgID uuid.UUID, roleIDs []uuid.UUID) ([]model.UserOrganizationRole, error)
	ExistsByUserRoleAndOrg(ctx context.Context, userID, roleID, organizationID uuid.UUID) (bool, error)

	// Trang thai
	UpdateStatus(ctx context.Context, id uuid.UUID, status string, revokedBy *uuid.UUID) error

	// Hang loat
	DeleteByUserID(ctx context.Context, userID uuid.UUID) error
	DeleteByRoleID(ctx context.Context, roleID uuid.UUID) error
	DeleteByOrganizationID(ctx context.Context, organizationID uuid.UUID) error

	// Transaction
	AssignRolesWithTx(ctx context.Context, toReactivate []*model.UserOrganizationRole, toCreate []model.UserOrganizationRole) error

	// Tuong thich nguoc
	CreateUserRoles(ctx context.Context, userRoles []model.UserOrganizationRole) error
	CreateUserRole(ctx context.Context, userRole *model.UserOrganizationRole) error
	FindByUserIDWithRoles(ctx context.Context, userID uuid.UUID) ([]model.UserOrganizationRole, error)
}

type UserOrganizationRoleRepository struct {
	db *gorm.DB
}

func NewUserOrganizationRoleRepository(db *gorm.DB) *UserOrganizationRoleRepository {
	return &UserOrganizationRoleRepository{db: db}
}

// CRUD

func (r *UserOrganizationRoleRepository) Create(ctx context.Context, userOrgRole *model.UserOrganizationRole) error {
	return r.db.WithContext(ctx).Create(userOrgRole).Error
}

func (r *UserOrganizationRoleRepository) CreateBatch(ctx context.Context, userOrgRoles []model.UserOrganizationRole) error {
	if len(userOrgRoles) == 0 {
		return nil
	}
	return r.db.WithContext(ctx).Create(&userOrgRoles).Error
}

func (r *UserOrganizationRoleRepository) FindByID(ctx context.Context, id uuid.UUID) (*model.UserOrganizationRole, error) {
	var userOrgRole model.UserOrganizationRole
	err := r.db.WithContext(ctx).Where("id = ?", id).First(&userOrgRole).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &userOrgRole, nil
}

func (r *UserOrganizationRoleRepository) FindByIDWithDetails(ctx context.Context, id uuid.UUID) (*model.UserOrganizationRole, error) {
	var userOrgRole model.UserOrganizationRole
	err := r.db.WithContext(ctx).
		Preload("User").
		Preload("Role").
		Preload("Organization").
		Preload("Granter").
		Preload("Revoker").
		Where("id = ?", id).
		First(&userOrgRole).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &userOrgRole, nil
}

func (r *UserOrganizationRoleRepository) Update(ctx context.Context, userOrgRole *model.UserOrganizationRole) error {
	return r.db.WithContext(ctx).Save(userOrgRole).Error
}

func (r *UserOrganizationRoleRepository) Delete(ctx context.Context, id uuid.UUID) error {
	return r.db.WithContext(ctx).Delete(&model.UserOrganizationRole{}, "id = ?", id).Error
}

func (r *UserOrganizationRoleRepository) HardDelete(ctx context.Context, id uuid.UUID) error {
	return r.db.WithContext(ctx).Unscoped().Delete(&model.UserOrganizationRole{}, "id = ?", id).Error
}

// Truy van

func (r *UserOrganizationRoleRepository) FindByUserID(ctx context.Context, userID uuid.UUID, status string) ([]model.UserOrganizationRole, error) {
	var userOrgRoles []model.UserOrganizationRole
	query := r.db.WithContext(ctx).Where("user_id = ?", userID)

	if status != "" {
		query = query.Where("status = ?", status)
	}

	err := query.Order("granted_at DESC").Find(&userOrgRoles).Error
	if err != nil {
		return nil, err
	}
	return userOrgRoles, nil
}

func (r *UserOrganizationRoleRepository) FindByUserIDWithDetails(ctx context.Context, userID uuid.UUID, status string) ([]model.UserOrganizationRole, error) {
	var userOrgRoles []model.UserOrganizationRole
	query := r.db.WithContext(ctx).
		Preload("Role").
		Preload("Organization").
		Preload("Granter").
		Where("user_id = ?", userID)

	if status != "" {
		query = query.Where("status = ?", status)
	}

	err := query.Order("granted_at DESC").Find(&userOrgRoles).Error
	if err != nil {
		return nil, err
	}
	return userOrgRoles, nil
}

func (r *UserOrganizationRoleRepository) FindByRoleID(ctx context.Context, roleID uuid.UUID, page, pageSize int, status string) ([]model.UserOrganizationRole, int64, error) {
	var userOrgRoles []model.UserOrganizationRole
	var total int64

	offset := (page - 1) * pageSize

	query := r.db.WithContext(ctx).Model(&model.UserOrganizationRole{}).Where("role_id = ?", roleID)

	if status != "" {
		query = query.Where("status = ?", status)
	}

	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	err := query.
		Preload("User").
		Preload("Organization").
		Preload("Granter").
		Offset(offset).
		Limit(pageSize).
		Order("granted_at DESC").
		Find(&userOrgRoles).Error
	if err != nil {
		return nil, 0, err
	}

	return userOrgRoles, total, nil
}

func (r *UserOrganizationRoleRepository) FindByRoleIDAndOrgID(ctx context.Context, roleID, organizationID uuid.UUID, page, pageSize int, status string) ([]model.UserOrganizationRole, int64, error) {
	var userOrgRoles []model.UserOrganizationRole
	var total int64

	offset := (page - 1) * pageSize

	query := r.db.WithContext(ctx).Model(&model.UserOrganizationRole{}).
		Where("role_id = ? AND organization_id = ?", roleID, organizationID)

	if status != "" {
		query = query.Where("status = ?", status)
	}

	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	err := query.
		Preload("User").
		Preload("Role").
		Preload("Organization").
		Preload("Granter").
		Offset(offset).
		Limit(pageSize).
		Order("granted_at DESC").
		Find(&userOrgRoles).Error
	if err != nil {
		return nil, 0, err
	}

	return userOrgRoles, total, nil
}

func (r *UserOrganizationRoleRepository) FindByOrganizationID(ctx context.Context, organizationID uuid.UUID, page, pageSize int, status string) ([]model.UserOrganizationRole, int64, error) {
	var userOrgRoles []model.UserOrganizationRole
	var total int64

	offset := (page - 1) * pageSize

	query := r.db.WithContext(ctx).Model(&model.UserOrganizationRole{}).Where("organization_id = ?", organizationID)

	if status != "" {
		query = query.Where("status = ?", status)
	}

	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	err := query.
		Preload("User").
		Preload("Role").
		Preload("Granter").
		Offset(offset).
		Limit(pageSize).
		Order("granted_at DESC").
		Find(&userOrgRoles).Error
	if err != nil {
		return nil, 0, err
	}

	return userOrgRoles, total, nil
}

func (r *UserOrganizationRoleRepository) FindByUserAndOrganization(ctx context.Context, userID, organizationID uuid.UUID, status string) ([]model.UserOrganizationRole, error) {
	var userOrgRoles []model.UserOrganizationRole
	query := r.db.WithContext(ctx).
		Preload("Role").
		Where("user_id = ? AND organization_id = ?", userID, organizationID)

	if status != "" {
		query = query.Where("status = ?", status)
	}

	err := query.Order("granted_at DESC").Find(&userOrgRoles).Error
	if err != nil {
		return nil, err
	}
	return userOrgRoles, nil
}

func (r *UserOrganizationRoleRepository) FindByUserRoleAndOrg(ctx context.Context, userID, roleID, organizationID uuid.UUID) (*model.UserOrganizationRole, error) {
	var userOrgRole model.UserOrganizationRole
	err := r.db.WithContext(ctx).
		Where("user_id = ? AND role_id = ? AND organization_id = ?", userID, roleID, organizationID).
		First(&userOrgRole).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &userOrgRole, nil
}

func (r *UserOrganizationRoleRepository) ExistsByUserRoleAndOrg(ctx context.Context, userID, roleID, organizationID uuid.UUID) (bool, error) {
	var count int64
	// Dung Unscoped de bao gom ca soft-deleted
	err := r.db.WithContext(ctx).
		Unscoped().
		Model(&model.UserOrganizationRole{}).
		Where("user_id = ? AND role_id = ? AND organization_id = ?", userID, roleID, organizationID).
		Count(&count).Error
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

// FindByUserAndRoleIDsInOrg tim cac mapping theo user va danh sach role IDs trong org
// Bao gom ca soft-deleted de kiem tra reactivation
func (r *UserOrganizationRoleRepository) FindByUserAndRoleIDsInOrg(ctx context.Context, userID, orgID uuid.UUID, roleIDs []uuid.UUID) ([]model.UserOrganizationRole, error) {
	if len(roleIDs) == 0 {
		return []model.UserOrganizationRole{}, nil
	}
	var userOrgRoles []model.UserOrganizationRole
	err := r.db.WithContext(ctx).
		Unscoped().
		Where("user_id = ? AND organization_id = ? AND role_id IN ?", userID, orgID, roleIDs).
		Find(&userOrgRoles).Error
	if err != nil {
		return nil, err
	}
	return userOrgRoles, nil
}

// Trang thai

func (r *UserOrganizationRoleRepository) UpdateStatus(ctx context.Context, id uuid.UUID, status string, revokedBy *uuid.UUID) error {
	updates := map[string]interface{}{
		"status": status,
	}

	// Xu ly revoke/inactive
	if (status == model.UserOrgRoleStatusInactive || status == "revoked") && revokedBy != nil {
		updates["revoked_by"] = revokedBy
		updates["revoked_at"] = gorm.Expr("NOW()")
	}

	// Xu ly reactivate
	if status == model.UserOrgRoleStatusActive {
		updates["revoked_by"] = nil
		updates["revoked_at"] = nil
	}

	return r.db.WithContext(ctx).
		Model(&model.UserOrganizationRole{}).
		Where("id = ?", id).
		Updates(updates).Error
}

// Hang loat

func (r *UserOrganizationRoleRepository) DeleteByUserID(ctx context.Context, userID uuid.UUID) error {
	return r.db.WithContext(ctx).
		Where("user_id = ?", userID).
		Delete(&model.UserOrganizationRole{}).Error
}

func (r *UserOrganizationRoleRepository) DeleteByRoleID(ctx context.Context, roleID uuid.UUID) error {
	return r.db.WithContext(ctx).
		Where("role_id = ?", roleID).
		Delete(&model.UserOrganizationRole{}).Error
}

func (r *UserOrganizationRoleRepository) DeleteByOrganizationID(ctx context.Context, organizationID uuid.UUID) error {
	return r.db.WithContext(ctx).
		Where("organization_id = ?", organizationID).
		Delete(&model.UserOrganizationRole{}).Error
}

// Transaction

// AssignRolesWithTx reactivate va tao mappings trong 1 transaction
func (r *UserOrganizationRoleRepository) AssignRolesWithTx(ctx context.Context, toReactivate []*model.UserOrganizationRole, toCreate []model.UserOrganizationRole) error {
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

// Tuong thich nguoc

func (r *UserOrganizationRoleRepository) CreateUserRoles(ctx context.Context, userRoles []model.UserOrganizationRole) error {
	return r.CreateBatch(ctx, userRoles)
}

func (r *UserOrganizationRoleRepository) CreateUserRole(ctx context.Context, userRole *model.UserOrganizationRole) error {
	return r.Create(ctx, userRole)
}

func (r *UserOrganizationRoleRepository) FindByUserIDWithRoles(ctx context.Context, userID uuid.UUID) ([]model.UserOrganizationRole, error) {
	return r.FindByUserIDWithDetails(ctx, userID, "")
}
