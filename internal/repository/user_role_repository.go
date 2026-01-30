package repository

import (
	"context"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"study.com/v1/internal/model"
)

type UserRoleRepositoryInterface interface {
	// CreateUserRoles creates multiple user-role associations in a single transaction
	CreateUserRoles(ctx context.Context, userRoles []model.UserRole) error
	// CreateUserRole creates a single user-role association
	CreateUserRole(ctx context.Context, userRole *model.UserRole) error
	// FindByUserID finds all roles for a specific user
	FindByUserID(ctx context.Context, userID uuid.UUID) ([]model.UserRole, error)
	// FindByUserIDWithRoles finds all user_roles with preloaded Role data
	FindByUserIDWithRoles(ctx context.Context, userID uuid.UUID) ([]model.UserRole, error)
	// DeleteByUserID deletes all user-role associations for a user
	DeleteByUserID(ctx context.Context, userID uuid.UUID) error
}

type UserRoleRepository struct {
	db *gorm.DB
}

func NewUserRoleRepository(db *gorm.DB) *UserRoleRepository {
	return &UserRoleRepository{db: db}
}

// CreateUserRoles creates multiple user-role associations
func (r *UserRoleRepository) CreateUserRoles(ctx context.Context, userRoles []model.UserRole) error {
	if len(userRoles) == 0 {
		return nil
	}
	return r.db.WithContext(ctx).Create(&userRoles).Error
}

// CreateUserRole creates a single user-role association
func (r *UserRoleRepository) CreateUserRole(ctx context.Context, userRole *model.UserRole) error {
	return r.db.WithContext(ctx).Create(userRole).Error
}

// FindByUserID finds all user_roles entries for a user
func (r *UserRoleRepository) FindByUserID(ctx context.Context, userID uuid.UUID) ([]model.UserRole, error) {
	var userRoles []model.UserRole
	err := r.db.WithContext(ctx).
		Where("user_id = ?", userID).
		Find(&userRoles).Error
	if err != nil {
		return nil, err
	}
	return userRoles, nil
}

// FindByUserIDWithRoles finds all user_roles with preloaded Role data
func (r *UserRoleRepository) FindByUserIDWithRoles(ctx context.Context, userID uuid.UUID) ([]model.UserRole, error) {
	var userRoles []model.UserRole
	err := r.db.WithContext(ctx).
		Preload("Role").
		Where("user_id = ?", userID).
		Find(&userRoles).Error
	if err != nil {
		return nil, err
	}
	return userRoles, nil
}

// DeleteByUserID deletes all user-role associations for a user
func (r *UserRoleRepository) DeleteByUserID(ctx context.Context, userID uuid.UUID) error {
	return r.db.WithContext(ctx).
		Where("user_id = ?", userID).
		Delete(&model.UserRole{}).Error
}
