package repository

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"study.com/v1/internal/model"
	"study.com/v1/internal/utils"
)

type TagRepositoryInterface interface {
	Create(ctx context.Context, tag *model.Tag) error
	GetAll(ctx context.Context, page, pageSize int, keyword string) ([]model.Tag, int64, error)
	GetByID(ctx context.Context, id uuid.UUID) (*model.Tag, error)
	GetByIDs(ctx context.Context, ids []uuid.UUID) ([]model.Tag, error)
	Update(ctx context.Context, tag *model.Tag) error
	Delete(ctx context.Context, id uuid.UUID) error
	Exists(ctx context.Context, id uuid.UUID) (bool, error)
}

type TagRepository struct {
	db *gorm.DB
}

func NewTagRepository(db *gorm.DB) *TagRepository {
	return &TagRepository{db: db}
}

func (r *TagRepository) Create(ctx context.Context, tag *model.Tag) error {
	return r.db.WithContext(ctx).Create(tag).Error
}

func (r *TagRepository) GetAll(ctx context.Context, page, pageSize int, keyword string) ([]model.Tag, int64, error) {
	var tags []model.Tag
	var total int64

	query := r.db.WithContext(ctx).Model(&model.Tag{})
	query = utils.ApplyKeywordSearch(query, keyword, "tags.name")

	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	if err := utils.ApplyPagination(query, page, pageSize).
		Order("tags.created_at DESC").
		Find(&tags).Error; err != nil {
		return nil, 0, err
	}

	return tags, total, nil
}

func (r *TagRepository) GetByID(ctx context.Context, id uuid.UUID) (*model.Tag, error) {
	var tag model.Tag
	err := r.db.WithContext(ctx).Where("id = ?", id).First(&tag).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &tag, nil
}

func (r *TagRepository) GetByIDs(ctx context.Context, ids []uuid.UUID) ([]model.Tag, error) {
	var tags []model.Tag
	if len(ids) == 0 {
		return tags, nil
	}
	err := r.db.WithContext(ctx).Where("id IN ?", ids).Find(&tags).Error
	return tags, err
}

func (r *TagRepository) Update(ctx context.Context, tag *model.Tag) error {
	return r.db.WithContext(ctx).Save(tag).Error
}

func (r *TagRepository) Delete(ctx context.Context, id uuid.UUID) error {
	return r.db.WithContext(ctx).Unscoped().Delete(&model.Tag{}, "id = ?", id).Error
}

func (r *TagRepository) Exists(ctx context.Context, id uuid.UUID) (bool, error) {
	var count int64
	err := r.db.WithContext(ctx).Model(&model.Tag{}).Where("id = ?", id).Count(&count).Error
	return count > 0, err
}
