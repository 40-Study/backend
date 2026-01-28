package repository

import (
	"context"
	"errors"

	"gorm.io/gorm"
	"study.com/v1/internal/model"
)

type UserRoleRepositoryInterface interface {
	FindByCode(ctx context.Context, code string) (*model.UserRole, error)
	FindByID(ctx context.Context, id uint) (*model.UserRole, error)
	GetAll(ctx context.Context) ([]model.UserRole, error)
}

type UserRoleRepository struct {
	db *gorm.DB
}

func NewUserRoleRepository(db *gorm.DB) *UserRoleRepository {
	return &UserRoleRepository{db: db}
}

func (r *UserRoleRepository) FindByCode(ctx context.Context, code string) (*model.UserRole, error) {
	var role model.UserRole
	err := r.db.WithContext(ctx).Where("code = ? AND is_active = ?", code, true).First(&role).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &role, nil
}

func (r *UserRoleRepository) FindByID(ctx context.Context, id uint) (*model.UserRole, error) {
	var role model.UserRole
	err := r.db.WithContext(ctx).Where("id = ? AND is_active = ?", id, true).First(&role).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &role, nil
}

func (r *UserRoleRepository) GetAll(ctx context.Context) ([]model.UserRole, error) {
	var roles []model.UserRole
	err := r.db.WithContext(ctx).Where("is_active = ?", true).Find(&roles).Error
	if err != nil {
		return nil, err
	}
	return roles, nil
}
