package repository

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"study.com/v1/internal/model"
	"study.com/v1/internal/utils"
)

type CategoryRepositoryInterface interface {
	Create(ctx context.Context, category *model.Category) error
	GetAll(ctx context.Context, keyword string) ([]model.Category, int64, error)
	GetByID(ctx context.Context, id uuid.UUID) (*model.Category, error)
	GetBySlug(ctx context.Context, slug string) (*model.Category, error)
	Update(ctx context.Context, category *model.Category) error
	Delete(ctx context.Context, id uuid.UUID) error
	Exists(ctx context.Context, id uuid.UUID) (bool, error)
	GetChildren(ctx context.Context, parentID uuid.UUID) ([]model.Category, error)
}

type CategoryRepository struct {
	db *gorm.DB
}

func NewCategoryRepository(db *gorm.DB) *CategoryRepository {
	return &CategoryRepository{db: db}
}

func (r *CategoryRepository) Create(ctx context.Context, category *model.Category) error {
	return r.db.WithContext(ctx).Create(category).Error
}

func (r *CategoryRepository) GetAll(ctx context.Context, keyword string) ([]model.Category, int64, error) {
	var categories []model.Category
	var total int64

	query := r.db.WithContext(ctx).Model(&model.Category{})
	query = utils.ApplyKeywordSearch(query, keyword, "categories.name")

	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	if err := query.Order("display_order ASC, name ASC").Find(&categories).Error; err != nil {
		return nil, 0, err
	}

	return categories, total, nil
}

func (r *CategoryRepository) GetByID(ctx context.Context, id uuid.UUID) (*model.Category, error) {
	var category model.Category
	err := r.db.WithContext(ctx).Where("id = ?", id).First(&category).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &category, nil
}

func (r *CategoryRepository) GetBySlug(ctx context.Context, slug string) (*model.Category, error) {
	var category model.Category
	err := r.db.WithContext(ctx).Where("slug = ?", slug).First(&category).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &category, nil
}

func (r *CategoryRepository) Update(ctx context.Context, category *model.Category) error {
	return r.db.WithContext(ctx).Save(category).Error
}

func (r *CategoryRepository) Delete(ctx context.Context, id uuid.UUID) error {
	return r.db.WithContext(ctx).Delete(&model.Category{}, "id = ?", id).Error
}

func (r *CategoryRepository) Exists(ctx context.Context, id uuid.UUID) (bool, error) {
	var count int64
	err := r.db.WithContext(ctx).Model(&model.Category{}).Where("id = ?", id).Count(&count).Error
	return count > 0, err
}

func (r *CategoryRepository) GetChildren(ctx context.Context, parentID uuid.UUID) ([]model.Category, error) {
	var children []model.Category
	err := r.db.WithContext(ctx).
		Where("parent_id = ?", parentID).
		Order("display_order ASC, name ASC").
		Find(&children).Error
	return children, err
}
