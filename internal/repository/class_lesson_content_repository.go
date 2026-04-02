package repository

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"study.com/v1/internal/model"
)

type ClassLessonContentRepositoryInterface interface {
	Create(ctx context.Context, clc *model.ClassLessonContent) error
	CreateBatch(ctx context.Context, clcs []*model.ClassLessonContent) error
	GetByID(ctx context.Context, id uuid.UUID) (*model.ClassLessonContent, error)
	GetByClassAndContent(ctx context.Context, classID, contentID uuid.UUID) (*model.ClassLessonContent, error)
	Update(ctx context.Context, clc *model.ClassLessonContent) error
	Delete(ctx context.Context, classID, contentID uuid.UUID) error
	GetByContentID(ctx context.Context, contentID uuid.UUID, page, pageSize int) ([]model.ClassLessonContent, int64, error)
	GetByClassID(ctx context.Context, classID uuid.UUID, page, pageSize int) ([]model.ClassLessonContent, int64, error)
	Exists(ctx context.Context, classID, contentID uuid.UUID) (bool, error)
	HasOverlappingLivestream(ctx context.Context, classID uuid.UUID, scheduledAt, endAt time.Time, excludeContentID *uuid.UUID) (bool, error)
	DeleteByContentID(ctx context.Context, contentID uuid.UUID) error
	DeleteByClassID(ctx context.Context, classID uuid.UUID) error
}

type ClassLessonContentRepository struct {
	db *gorm.DB
}

func NewClassLessonContentRepository(db *gorm.DB) *ClassLessonContentRepository {
	return &ClassLessonContentRepository{db: db}
}

func (r *ClassLessonContentRepository) Create(ctx context.Context, clc *model.ClassLessonContent) error {
	return r.db.WithContext(ctx).Create(clc).Error
}

func (r *ClassLessonContentRepository) CreateBatch(ctx context.Context, clcs []*model.ClassLessonContent) error {
	if len(clcs) == 0 {
		return nil
	}
	return r.db.WithContext(ctx).Create(&clcs).Error
}

func (r *ClassLessonContentRepository) GetByID(ctx context.Context, id uuid.UUID) (*model.ClassLessonContent, error) {
	var clc model.ClassLessonContent
	err := r.db.WithContext(ctx).
		Preload("Class").
		Preload("LessonContent").
		Where("id = ?", id).
		First(&clc).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &clc, nil
}

func (r *ClassLessonContentRepository) GetByClassAndContent(ctx context.Context, classID, contentID uuid.UUID) (*model.ClassLessonContent, error) {
	var clc model.ClassLessonContent
	err := r.db.WithContext(ctx).
		Preload("Class").
		Preload("LessonContent").
		Where("class_id = ? AND lesson_content_id = ?", classID, contentID).
		First(&clc).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &clc, nil
}

func (r *ClassLessonContentRepository) Update(ctx context.Context, clc *model.ClassLessonContent) error {
	return r.db.WithContext(ctx).Save(clc).Error
}

func (r *ClassLessonContentRepository) Delete(ctx context.Context, classID, contentID uuid.UUID) error {
	return r.db.WithContext(ctx).
		Where("class_id = ? AND lesson_content_id = ?", classID, contentID).
		Delete(&model.ClassLessonContent{}).Error
}

func (r *ClassLessonContentRepository) GetByContentID(ctx context.Context, contentID uuid.UUID, page, pageSize int) ([]model.ClassLessonContent, int64, error) {
	var items []model.ClassLessonContent
	var total int64

	query := r.db.WithContext(ctx).Model(&model.ClassLessonContent{}).Where("lesson_content_id = ?", contentID)

	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	err := r.db.WithContext(ctx).
		Preload("Class").
		Preload("LessonContent").
		Where("lesson_content_id = ?", contentID).
		Offset((page - 1) * pageSize).
		Limit(pageSize).
		Order("created_at DESC").
		Find(&items).Error

	return items, total, err
}

func (r *ClassLessonContentRepository) GetByClassID(ctx context.Context, classID uuid.UUID, page, pageSize int) ([]model.ClassLessonContent, int64, error) {
	var items []model.ClassLessonContent
	var total int64

	query := r.db.WithContext(ctx).Model(&model.ClassLessonContent{}).Where("class_id = ?", classID)

	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	err := r.db.WithContext(ctx).
		Preload("Class").
		Preload("LessonContent").
		Where("class_id = ?", classID).
		Offset((page - 1) * pageSize).
		Limit(pageSize).
		Order("created_at DESC").
		Find(&items).Error

	return items, total, err
}

func (r *ClassLessonContentRepository) Exists(ctx context.Context, classID, contentID uuid.UUID) (bool, error) {
	var count int64
	err := r.db.WithContext(ctx).
		Model(&model.ClassLessonContent{}).
		Where("class_id = ? AND lesson_content_id = ?", classID, contentID).
		Count(&count).Error
	return count > 0, err
}

// HasOverlappingLivestream checks if a class has any livestream content scheduled
// in an overlapping time range. excludeContentID can exclude the current record during updates.
func (r *ClassLessonContentRepository) HasOverlappingLivestream(ctx context.Context, classID uuid.UUID, scheduledAt, endAt time.Time, excludeContentID *uuid.UUID) (bool, error) {
	var count int64

	query := r.db.WithContext(ctx).
		Model(&model.ClassLessonContent{}).
		Joins("JOIN lesson_contents ON lesson_contents.id = class_lesson_contents.lesson_content_id").
		Where("class_lesson_contents.class_id = ?", classID).
		Where("lesson_contents.type = ?", "livestream").
		Where("class_lesson_contents.scheduled_at IS NOT NULL").
		Where("class_lesson_contents.end_at IS NOT NULL").
		Where("class_lesson_contents.scheduled_at < ? AND class_lesson_contents.end_at > ?", endAt, scheduledAt)

	if excludeContentID != nil {
		query = query.Where("class_lesson_contents.lesson_content_id != ?", *excludeContentID)
	}

	err := query.Count(&count).Error
	return count > 0, err
}

func (r *ClassLessonContentRepository) DeleteByContentID(ctx context.Context, contentID uuid.UUID) error {
	return r.db.WithContext(ctx).
		Where("lesson_content_id = ?", contentID).
		Delete(&model.ClassLessonContent{}).Error
}

func (r *ClassLessonContentRepository) DeleteByClassID(ctx context.Context, classID uuid.UUID) error {
	return r.db.WithContext(ctx).
		Where("class_id = ?", classID).
		Delete(&model.ClassLessonContent{}).Error
}
