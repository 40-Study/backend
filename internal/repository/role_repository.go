package repository

import (
	"context"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"study.com/v1/internal/model"
)

type RoleRepositoryInterface interface {
	GetRoleByIDs(ctx context.Context, ids []uuid.UUID) ([]model.Role, error)
}

type RoleRepository struct {
	db *gorm.DB
}

func NewRoleRepository(db *gorm.DB) *RoleRepository {
	return &RoleRepository{db: db}
}

// GetRoleByIDs finds multiple active roles by their IDs
func (r *RoleRepository) GetRoleByIDs(ctx context.Context, ids []uuid.UUID) ([]model.Role, error) {
	var roles []model.Role
	err := r.db.WithContext(ctx).
		Where("id IN ? AND is_active = ?", ids, true).
		Find(&roles).Error
	if err != nil {
		return nil, err
	}
	return roles, nil
}
