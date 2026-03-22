package repository

import (
	"errors"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"study.com/v1/internal/model"
)

var (
	ErrOrderItemNotFound   = errors.New("order item not found")
	ErrDuplicateOrderItem  = errors.New("duplicate order item")
)

// OrderItemRepository - Repository for OrderItem
type OrderItemRepository struct {
	db *gorm.DB
}

func NewOrderItemRepository(db *gorm.DB) *OrderItemRepository {
	return &OrderItemRepository{db: db}
}

// Create - Create new order item
func (r *OrderItemRepository) Create(item *model.OrderItem) error {
	return r.db.Create(item).Error
}

// CreateBatch - Create multiple order items in batch
func (r *OrderItemRepository) CreateBatch(items []model.OrderItem) error {
	if len(items) == 0 {
		return nil
	}
	return r.db.Create(&items).Error
}

// GetByID - Get order item by ID
func (r *OrderItemRepository) GetByID(id uuid.UUID) (*model.OrderItem, error) {
	var item model.OrderItem
	if err := r.db.First(&item, "id = ?", id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrOrderItemNotFound
		}
		return nil, err
	}
	return &item, nil
}

// GetByOrderID - Get all items for an order
func (r *OrderItemRepository) GetByOrderID(orderID uuid.UUID) ([]model.OrderItem, error) {
	var items []model.OrderItem
	if err := r.db.Where("order_id = ?", orderID).Find(&items).Error; err != nil {
		return nil, err
	}
	return items, nil
}

// GetByOrderIDAndCourseID - Get order item by order ID and course ID
func (r *OrderItemRepository) GetByOrderIDAndCourseID(orderID, courseID uuid.UUID) (*model.OrderItem, error) {
	var item model.OrderItem
	if err := r.db.First(&item, "order_id = ? AND course_id = ?", orderID, courseID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrOrderItemNotFound
		}
		return nil, err
	}
	return &item, nil
}

// GetCourseIDsByOrderID - Get all course IDs for an order
func (r *OrderItemRepository) GetCourseIDsByOrderID(orderID uuid.UUID) ([]uuid.UUID, error) {
	var courseIDs []uuid.UUID
	if err := r.db.Model(&model.OrderItem{}).
		Where("order_id = ?", orderID).
		Pluck("course_id", &courseIDs).Error; err != nil {
		return nil, err
	}
	return courseIDs, nil
}

// DeleteByOrderID - Delete all items for an order
func (r *OrderItemRepository) DeleteByOrderID(orderID uuid.UUID) error {
	return r.db.Where("order_id = ?", orderID).Delete(&model.OrderItem{}).Error
}

// Delete - Delete order item
func (r *OrderItemRepository) Delete(id uuid.UUID) error {
	return r.db.Delete(&model.OrderItem{}, "id = ?", id).Error
}

// WithTransaction - Execute within transaction
func (r *OrderItemRepository) WithTransaction(fn func(repo *OrderItemRepository) error) error {
	return r.db.Transaction(func(tx *gorm.DB) error {
		txRepo := &OrderItemRepository{db: tx}
		return fn(txRepo)
	})
}

// CheckDuplicate - Check if (order_id, course_id) already exists
func (r *OrderItemRepository) CheckDuplicate(orderID, courseID uuid.UUID) (bool, error) {
	var count int64
	err := r.db.Model(&model.OrderItem{}).
		Where("order_id = ? AND course_id = ?", orderID, courseID).
		Count(&count).Error
	return count > 0, err
}

// CalculateTotal - Calculate total of final prices
func (r *OrderItemRepository) CalculateTotal(orderID uuid.UUID) (interface{}, error) {
	var total interface{}
	err := r.db.Model(&model.OrderItem{}).
		Where("order_id = ?", orderID).
		Select("COALESCE(SUM(final_price), 0)").
		Scan(&total).Error
	return total, err
}
