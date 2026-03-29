package repository

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"study.com/v1/internal/model"
)

type SectionRepositoryInterface interface {
	Create(ctx context.Context, section *model.Section) error
	GetAllByCourseID(ctx context.Context, courseID uuid.UUID) ([]model.Section, error)
	GetByID(ctx context.Context, id uuid.UUID) (*model.Section, error)
	Update(ctx context.Context, section *model.Section) error
	Delete(ctx context.Context, id uuid.UUID) error
	GetMaxDisplayOrder(ctx context.Context, courseID uuid.UUID) (int, error)
	BelongsToCourse(ctx context.Context, sectionID, courseID uuid.UUID) (bool, error)
	Reorder(ctx context.Context, items []ReorderItem) error
	Exists(ctx context.Context, id uuid.UUID) (bool, error)
}

type ReorderItem struct {
	ID           uuid.UUID
	DisplayOrder int
}

type SectionRepository struct {
	db *gorm.DB
}

func NewSectionRepository(db *gorm.DB) *SectionRepository {
	return &SectionRepository{db: db}
}

func (r *SectionRepository) Create(ctx context.Context, section *model.Section) error {
	return r.db.WithContext(ctx).Create(section).Error
}

func (r *SectionRepository) GetAllByCourseID(ctx context.Context, courseID uuid.UUID) ([]model.Section, error) {
	var sections []model.Section
	err := r.db.WithContext(ctx).
		Where("course_id = ?", courseID).
		Preload("Lessons", func(db *gorm.DB) *gorm.DB {
			return db.Order("display_order ASC")
		}).
		Order("display_order ASC").
		Find(&sections).Error
	return sections, err
}

func (r *SectionRepository) GetByID(ctx context.Context, id uuid.UUID) (*model.Section, error) {
	var section model.Section
	err := r.db.WithContext(ctx).Where("id = ?", id).First(&section).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &section, nil
}

func (r *SectionRepository) Update(ctx context.Context, section *model.Section) error {
	return r.db.WithContext(ctx).Save(section).Error
}

func (r *SectionRepository) Delete(ctx context.Context, id uuid.UUID) error {
	return r.db.WithContext(ctx).Delete(&model.Section{}, "id = ?", id).Error
}

func (r *SectionRepository) GetMaxDisplayOrder(ctx context.Context, courseID uuid.UUID) (int, error) {
	var maxOrder *int
	err := r.db.WithContext(ctx).
		Model(&model.Section{}).
		Where("course_id = ?", courseID).
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

func (r *SectionRepository) BelongsToCourse(ctx context.Context, sectionID, courseID uuid.UUID) (bool, error) {
	var count int64
	err := r.db.WithContext(ctx).
		Model(&model.Section{}).
		Where("id = ? AND course_id = ?", sectionID, courseID).
		Count(&count).Error
	return count > 0, err
}

func (r *SectionRepository) Reorder(ctx context.Context, items []ReorderItem) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		for _, item := range items {
			if err := tx.Model(&model.Section{}).
				Where("id = ?", item.ID).
				Update("display_order", item.DisplayOrder).Error; err != nil {
				return err
			}
		}
		return nil
	})
}

func (r *SectionRepository) Exists(ctx context.Context, id uuid.UUID) (bool, error) {
	var count int64
	err := r.db.WithContext(ctx).Model(&model.Section{}).Where("id = ?", id).Count(&count).Error
	return count > 0, err
}
