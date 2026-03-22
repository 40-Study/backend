package repository

import (
	"context"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"study.com/v1/internal/model"
)

// CartItemRepository handles cart item persistence operations.
type CartItemRepository struct {
	db *gorm.DB
}

func NewCartItemRepository(db *gorm.DB) *CartItemRepository {
	return &CartItemRepository{db: db}
}

func (r *CartItemRepository) Create(ctx context.Context, item *model.CartItem) error {
	return r.db.WithContext(ctx).Create(item).Error
}

func (r *CartItemRepository) Delete(ctx context.Context, userID, courseID uuid.UUID) error {
	return r.db.WithContext(ctx).
		Where("user_id = ? AND course_id = ?", userID, courseID).
		Delete(&model.CartItem{}).Error
}

func (r *CartItemRepository) DeleteByUserID(ctx context.Context, userID uuid.UUID) error {
	return r.db.WithContext(ctx).
		Where("user_id = ?", userID).
		Delete(&model.CartItem{}).Error
}

func (r *CartItemRepository) GetByUserID(ctx context.Context, userID uuid.UUID) ([]model.CartItem, error) {
	var items []model.CartItem
	err := r.db.WithContext(ctx).
		Where("user_id = ?", userID).
		Order("created_at ASC").
		Find(&items).Error
	if err != nil {
		return nil, err
	}
	return items, nil
}

func (r *CartItemRepository) Exists(ctx context.Context, userID, courseID uuid.UUID) (bool, error) {
	var count int64
	err := r.db.WithContext(ctx).
		Model(&model.CartItem{}).
		Where("user_id = ? AND course_id = ?", userID, courseID).
		Count(&count).Error
	if err != nil {
		return false, err
	}
	return count > 0, nil
}
