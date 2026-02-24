package repository

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"study.com/v1/internal/model"
	"study.com/v1/internal/utils"
)

type TeacherProfileRepositoryInterface interface {
	Create(ctx context.Context, profile *model.TeacherProfile) error
	GetAll(ctx context.Context, page, pageSize int, keyword string, status string) ([]model.TeacherProfile, int64, error)
	GetByID(ctx context.Context, id uuid.UUID) (*model.TeacherProfile, error)
	GetByUserID(ctx context.Context, userID uuid.UUID) (*model.TeacherProfile, error)
	Update(ctx context.Context, profile *model.TeacherProfile) error
	Delete(ctx context.Context, id uuid.UUID, hardDelete bool) error
}

type TeacherProfileRepository struct {
	db *gorm.DB
}

func NewTeacherProfileRepository(db *gorm.DB) *TeacherProfileRepository {
	return &TeacherProfileRepository{db: db}
}

func (r *TeacherProfileRepository) Create(ctx context.Context, profile *model.TeacherProfile) error {
	return r.db.WithContext(ctx).Create(profile).Error
}

func (r *TeacherProfileRepository) GetAll(ctx context.Context, page, pageSize int, keyword string, status string) ([]model.TeacherProfile, int64, error) {
	var profiles []model.TeacherProfile
	var total int64

	query := r.db.WithContext(ctx).Model(&model.TeacherProfile{}).
		Joins("JOIN users ON users.id = teacher_profiles.user_id")
	query = utils.ApplySoftDeleteStatus(query, status)
	query = utils.ApplyKeywordSearch(query, keyword, "users.user_name", "users.email", "teacher_profiles.specialization", "teacher_profiles.department")

	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	if err := utils.ApplyPagination(query, page, pageSize).
		Select("teacher_profiles.*").
		Order("teacher_profiles.created_at DESC").
		Find(&profiles).Error; err != nil {
		return nil, 0, err
	}

	return profiles, total, nil
}

func (r *TeacherProfileRepository) GetByID(ctx context.Context, id uuid.UUID) (*model.TeacherProfile, error) {
	var profile model.TeacherProfile
	err := r.db.WithContext(ctx).Where("id = ?", id).First(&profile).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &profile, nil
}

func (r *TeacherProfileRepository) GetByUserID(ctx context.Context, userID uuid.UUID) (*model.TeacherProfile, error) {
	var profile model.TeacherProfile
	err := r.db.WithContext(ctx).Where("user_id = ?", userID).First(&profile).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &profile, nil
}

func (r *TeacherProfileRepository) Update(ctx context.Context, profile *model.TeacherProfile) error {
	return r.db.WithContext(ctx).Save(profile).Error
}

func (r *TeacherProfileRepository) Delete(ctx context.Context, id uuid.UUID, hardDelete bool) error {
	if hardDelete {
		return r.db.WithContext(ctx).Unscoped().Delete(&model.TeacherProfile{}, "id = ?", id).Error
	}
	return r.db.WithContext(ctx).Delete(&model.TeacherProfile{}, "id = ?", id).Error
}
