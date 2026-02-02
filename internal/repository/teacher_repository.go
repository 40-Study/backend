package repository

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"study.com/v1/internal/model"
	"study.com/v1/internal/utils"
)

type TeacherRepositoryInterface interface {
	GetAllTeachers(ctx context.Context, page, pageSize int, keyword string, status string) ([]model.User, int64, error)
	GetTeacherByID(ctx context.Context, id uuid.UUID) (*model.User, error)
	DeleteTeacher(ctx context.Context, id uuid.UUID, hardDelete bool) error
}

type TeacherRepository struct {
	db *gorm.DB
}

func NewTeacherRepository(db *gorm.DB) *TeacherRepository {
	return &TeacherRepository{db: db}
}

func (r *TeacherRepository) teacherQuery(ctx context.Context) *gorm.DB {
	return r.db.WithContext(ctx).
		Model(&model.User{}).
		Joins("JOIN user_roles ON user_roles.user_id = users.id").
		Joins("JOIN system_roles ON system_roles.id = user_roles.role_id").
		Where("system_roles.name = ?", "TEACHER")
}

func (r *TeacherRepository) GetAllTeachers(ctx context.Context, page, pageSize int, keyword string, status string) ([]model.User, int64, error) {
	var teachers []model.User
	var total int64

	query := r.teacherQuery(ctx)
	query = utils.ApplySoftDeleteStatus(query, status)
	query = utils.ApplyKeywordSearch(query, keyword, "users.user_name", "users.email", "users.full_name")

	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	if err := utils.ApplyPagination(query, page, pageSize).
		Select("users.*").
		Order("users.created_at DESC").
		Find(&teachers).Error; err != nil {
		return nil, 0, err
	}

	return teachers, total, nil
}

func (r *TeacherRepository) GetTeacherByID(ctx context.Context, id uuid.UUID) (*model.User, error) {
	var teacher model.User
	err := r.teacherQuery(ctx).
		Select("users.*").
		Where("users.id = ?", id).
		First(&teacher).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &teacher, nil
}

func (r *TeacherRepository) DeleteTeacher(ctx context.Context, id uuid.UUID, hardDelete bool) error {
	if hardDelete {
		return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
			if err := tx.Where("user_id = ?", id).Delete(&model.UserRole{}).Error; err != nil {
				return err
			}
			return tx.Unscoped().Delete(&model.User{}, "id = ?", id).Error
		})
	}
	return r.db.WithContext(ctx).
		Model(&model.User{}).
		Where("id = ?", id).
		Update("is_active", false).Error
}
