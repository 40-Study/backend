package repository

import (
	"context"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"study.com/v1/internal/model"
)

type CourseRepositoryInterface interface {
	Exists(ctx context.Context, id uuid.UUID) (bool, error)
}

type CourseRepository struct {
	db *gorm.DB
}

func NewCourseRepository(db *gorm.DB) *CourseRepository {
	return &CourseRepository{db: db}
}

func (r *CourseRepository) Exists(ctx context.Context, id uuid.UUID) (bool, error) {
	var count int64
	err := r.db.WithContext(ctx).Model(&model.Course{}).Where("id = ?", id).Count(&count).Error
	return count > 0, err
}
