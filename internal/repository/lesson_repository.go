package repository

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"study.com/v1/internal/model"
)

type LessonRepositoryInterface interface {
	Create(ctx context.Context, lesson *model.Lesson) error
	GetAllBySectionID(ctx context.Context, sectionID uuid.UUID) ([]model.Lesson, error)
	GetByID(ctx context.Context, id uuid.UUID) (*model.Lesson, error)
	Update(ctx context.Context, lesson *model.Lesson) error
	Delete(ctx context.Context, id uuid.UUID) error
	GetMaxDisplayOrder(ctx context.Context, sectionID uuid.UUID) (int, error)
	BelongsToSection(ctx context.Context, lessonID, sectionID uuid.UUID) (bool, error)
	Exists(ctx context.Context, id uuid.UUID) (bool, error)
	Reorder(ctx context.Context, items []ReorderItem) error
}

type LessonRepository struct {
	db *gorm.DB
}

func NewLessonRepository(db *gorm.DB) *LessonRepository {
	return &LessonRepository{db: db}
}

func (r *LessonRepository) Create(ctx context.Context, lesson *model.Lesson) error {
	return r.db.WithContext(ctx).Create(lesson).Error
}

func (r *LessonRepository) GetAllBySectionID(ctx context.Context, sectionID uuid.UUID) ([]model.Lesson, error) {
	var lessons []model.Lesson
	err := r.db.WithContext(ctx).
		Where("section_id = ?", sectionID).
		Order("display_order ASC").
		Find(&lessons).Error
	return lessons, err
}

func (r *LessonRepository) GetByID(ctx context.Context, id uuid.UUID) (*model.Lesson, error) {
	var lesson model.Lesson
	err := r.db.WithContext(ctx).Where("id = ?", id).First(&lesson).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &lesson, nil
}

func (r *LessonRepository) Update(ctx context.Context, lesson *model.Lesson) error {
	return r.db.WithContext(ctx).Save(lesson).Error
}

func (r *LessonRepository) Delete(ctx context.Context, id uuid.UUID) error {
	return r.db.WithContext(ctx).Unscoped().Delete(&model.Lesson{}, "id = ?", id).Error
}

func (r *LessonRepository) GetMaxDisplayOrder(ctx context.Context, sectionID uuid.UUID) (int, error) {
	var maxOrder *int
	err := r.db.WithContext(ctx).
		Model(&model.Lesson{}).
		Where("section_id = ?", sectionID).
		Select("COALESCE(MAX(display_order), -1)").
		Scan(&maxOrder).Error
	if err != nil {
		return -1, err
	}
	if maxOrder == nil {
		return -1, nil
	}
	return *maxOrder, nil
}

func (r *LessonRepository) BelongsToSection(ctx context.Context, lessonID, sectionID uuid.UUID) (bool, error) {
	var count int64
	err := r.db.WithContext(ctx).
		Model(&model.Lesson{}).
		Where("id = ? AND section_id = ?", lessonID, sectionID).
		Count(&count).Error
	return count > 0, err
}

func (r *LessonRepository) Exists(ctx context.Context, id uuid.UUID) (bool, error) {
	var count int64
	err := r.db.WithContext(ctx).Model(&model.Lesson{}).Where("id = ?", id).Count(&count).Error
	return count > 0, err
}

func (r *LessonRepository) Reorder(ctx context.Context, items []ReorderItem) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		for _, item := range items {
			if err := tx.Model(&model.Lesson{}).
				Where("id = ?", item.ID).
				Update("display_order", item.DisplayOrder).Error; err != nil {
				return err
			}
		}
		return nil
	})
}
