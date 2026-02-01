package repository

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"study.com/v1/internal/model"
)

type RoleRepositoryInterface interface {
	FindByCode(ctx context.Context, code string) (*model.Role, error)
	FindByID(ctx context.Context, id uuid.UUID) (*model.Role, error)
	FindByCodes(ctx context.Context, codes []string) ([]model.Role, error)
	FindByIDs(ctx context.Context, ids []uuid.UUID) ([]model.Role, error)
	GetAll(ctx context.Context) ([]model.Role, error)
}

type RoleRepository struct {
	db *gorm.DB
}

func NewRoleRepository(db *gorm.DB) *RoleRepository {
	return &RoleRepository{db: db}
}

// FindByCode finds a single active role by its code
func (r *RoleRepository) FindByCode(ctx context.Context, code string) (*model.Role, error) {
	var role model.Role
	err := r.db.WithContext(ctx).
		Where("code = ? AND is_active = ?", code, true).
		First(&role).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &role, nil
}

// FindByID finds a single active role by its ID
func (r *RoleRepository) FindByID(ctx context.Context, id uuid.UUID) (*model.Role, error) {
	var role model.Role
	err := r.db.WithContext(ctx).
		Where("id = ? AND is_active = ?", id, true).
		First(&role).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &role, nil
}

// FindByCodes finds multiple active roles by their codes
func (r *RoleRepository) FindByCodes(ctx context.Context, codes []string) ([]model.Role, error) {
	var roles []model.Role
	err := r.db.WithContext(ctx).
		Where("code IN ? AND is_active = ?", codes, true).
		Find(&roles).Error
	if err != nil {
		return nil, err
	}
	return roles, nil
}

// FindByIDs finds multiple active roles by their IDs
func (r *RoleRepository) FindByIDs(ctx context.Context, ids []uuid.UUID) ([]model.Role, error) {
	var roles []model.Role
	err := r.db.WithContext(ctx).
		Where("id IN ? AND is_active = ?", ids, true).
		Find(&roles).Error
	if err != nil {
		return nil, err
	}
	return roles, nil
}

// GetAll returns all active roles
func (r *RoleRepository) GetAll(ctx context.Context) ([]model.Role, error) {
	var roles []model.Role
	err := r.db.WithContext(ctx).
		Where("is_active = ?", true).
		Find(&roles).Error
	if err != nil {
		return nil, err
	}
	return roles, nil
}
