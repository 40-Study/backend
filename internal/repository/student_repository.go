package repository

import (
	"context"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"study.com/v1/internal/model"
)

type StudentRepositoryInterface interface {
	Exists(ctx context.Context, id uuid.UUID) (bool, error)
}

type StudentRepository struct {
	db *gorm.DB
}

func NewStudentRepository(db *gorm.DB) *StudentRepository {
	return &StudentRepository{db: db}
}

func (r *StudentRepository) studentQuery(ctx context.Context) *gorm.DB {
	return r.db.WithContext(ctx).
		Model(&model.User{}).
		Joins("JOIN user_roles ON user_roles.user_id = users.id").
		Joins("JOIN system_roles ON system_roles.id = user_roles.role_id").
		Where("system_roles.name = ?", "STUDENT")
}

func (r *StudentRepository) Exists(ctx context.Context, id uuid.UUID) (bool, error) {
	var count int64
	err := r.studentQuery(ctx).
		Where("users.id = ?", id).
		Count(&count).Error
	return count > 0, err
}
