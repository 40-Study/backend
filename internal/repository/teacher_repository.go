package repository

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"study.com/v1/internal/model"
)

type TeacherRepositoryInterface interface {
	GetAllTeachers(ctx context.Context, page, pageSize int, keyword string, status string) ([]model.User, int64, error)
	GetTeacherByID(ctx context.Context, id uuid.UUID) (*model.User, error)
	CreateTeacher(ctx context.Context, user *model.User, systemRoleName string) error
	UpdateTeacher(ctx context.Context, user *model.User) error
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
		Where("system_roles.name = ?", "teacher")
}

func (r *TeacherRepository) GetAllTeachers(ctx context.Context, page, pageSize int, keyword string, status string) ([]model.User, int64, error) {
	var teachers []model.User
	var total int64

	offset := (page - 1) * pageSize

	query := r.teacherQuery(ctx)

	switch status {
	case "inactive":
		query = query.Where("users.is_active = ?", false)
	case "all":
		// no filter
	default: // "active" or empty
		query = query.Where("users.is_active = ?", true)
	}

	if keyword != "" {
		query = query.Where("users.user_name ILIKE ? OR users.email ILIKE ? OR users.full_name ILIKE ?", "%"+keyword+"%", "%"+keyword+"%", "%"+keyword+"%")
	}

	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	if err := query.
		Select("users.*").
		Offset(offset).
		Limit(pageSize).
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

func (r *TeacherRepository) CreateTeacher(ctx context.Context, user *model.User, systemRoleName string) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(user).Error; err != nil {
			return err
		}

		var systemRole model.SystemRole
		if err := tx.Where("name = ?", systemRoleName).First(&systemRole).Error; err != nil {
			return err
		}

		userRole := model.UserRole{
			UserID: user.ID,
			RoleID: systemRole.ID,
		}
		return tx.Create(&userRole).Error
	})
}

func (r *TeacherRepository) UpdateTeacher(ctx context.Context, user *model.User) error {
	return r.db.WithContext(ctx).Save(user).Error
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
