package repository

import (
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"study.com/v1/internal/model"
)

var (
	ErrOrderNotFound = errors.New("order not found")
	ErrOrderConflict = errors.New("order conflict")
)

// OrderRepository - Repository for Order
type OrderRepository struct {
	db *gorm.DB
}

func NewOrderRepository(db *gorm.DB) *OrderRepository {
	return &OrderRepository{db: db}
}

// Create - Create new order
func (r *OrderRepository) Create(order *model.Order) error {
	return r.db.Create(order).Error
}

// GetByID - Get order by ID
func (r *OrderRepository) GetByID(id uuid.UUID) (*model.Order, error) {
	var order model.Order
	if err := r.db.Preload("Items").Preload("Coupon").First(&order, "id = ?", id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrOrderNotFound
		}
		return nil, err
	}
	return &order, nil
}

// GetByOrderNumber - Get order by order number
func (r *OrderRepository) GetByOrderNumber(orderNumber string) (*model.Order, error) {
	var order model.Order
	if err := r.db.Preload("Items").Preload("Coupon").First(&order, "order_number = ?", orderNumber).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrOrderNotFound
		}
		return nil, err
	}
	return &order, nil
}

// GetByUserID - Get orders by user ID with pagination
func (r *OrderRepository) GetByUserID(userID uuid.UUID, page, limit int, status string) ([]model.Order, int64, error) {
	var orders []model.Order
	var total int64

	query := r.db.Model(&model.Order{}).Where("user_id = ?", userID)

	if status != "" {
		query = query.Where("status = ?", status)
	}

	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	offset := (page - 1) * limit
	if err := query.Preload("Items").Preload("Coupon").
		Order("created_at DESC").
		Offset(offset).Limit(limit).
		Find(&orders).Error; err != nil {
		return nil, 0, err
	}

	return orders, total, nil
}

// GetByUserIDAndStatus - Get orders by user ID and status
func (r *OrderRepository) GetByUserIDAndStatus(userID uuid.UUID, status string) ([]model.Order, error) {
	var orders []model.Order
	if err := r.db.Preload("Items").Where("user_id = ? AND status = ?", userID, status).Find(&orders).Error; err != nil {
		return nil, err
	}
	return orders, nil
}

// UpdateStatus - Update order status
func (r *OrderRepository) UpdateStatus(orderID uuid.UUID, status string) error {
	return r.db.Model(&model.Order{}).Where("id = ?", orderID).Update("status", status).Error
}

// UpdatePaymentInfo - Update payment information
func (r *OrderRepository) UpdatePaymentInfo(orderID uuid.UUID, paymentMethod, paymentGateway, transactionID string, paidAt time.Time) error {
	updates := map[string]interface{}{
		"payment_method":         paymentMethod,
		"payment_gateway":       paymentGateway,
		"payment_transaction_id": transactionID,
		"paid_at":              paidAt,
		"status":               "completed",
	}
	return r.db.Model(&model.Order{}).Where("id = ?", orderID).Updates(updates).Error
}

// UpdatePaymentCode - Update payment code with created timestamp
func (r *OrderRepository) UpdatePaymentCode(orderID uuid.UUID, paymentCode string, createdAt time.Time) error {
	updates := map[string]interface{}{
		"payment_transaction_id":  paymentCode,
		"payment_code_created_at": createdAt,
	}
	return r.db.Model(&model.Order{}).Where("id = ?", orderID).Updates(updates).Error
}

// Update - Update order
func (r *OrderRepository) Update(order *model.Order) error {
	return r.db.Save(order).Error
}

// Delete - Delete order (soft delete)
func (r *OrderRepository) Delete(id uuid.UUID) error {
	return r.db.Delete(&model.Order{}, "id = ?", id).Error
}

// WithTransaction - Execute within transaction
func (r *OrderRepository) WithTransaction(fn func(repo *OrderRepository) error) error {
	return r.db.Transaction(func(tx *gorm.DB) error {
		txRepo := &OrderRepository{db: tx}
		return fn(txRepo)
	})
}

// GetForUpdate - Get order with row lock for update
func (r *OrderRepository) GetForUpdate(tx *gorm.DB, id uuid.UUID) (*model.Order, error) {
	var order model.Order
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
		First(&order, "id = ?", id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrOrderNotFound
		}
		return nil, err
	}
	return &order, nil
}

// UpdateStatusWithTx - Update order status within transaction
func (r *OrderRepository) UpdateStatusWithTx(tx *gorm.DB, orderID uuid.UUID, status string) error {
	return tx.Model(&model.Order{}).Where("id = ?", orderID).Update("status", status).Error
}

// CheckOrderNumberExists - Check if order number exists
func (r *OrderRepository) CheckOrderNumberExists(orderNumber string) (bool, error) {
	var count int64
	if err := r.db.Model(&model.Order{}).Where("order_number = ?", orderNumber).Count(&count).Error; err != nil {
		return false, err
	}
	return count > 0, nil
}

// GetPendingOrders - Get all pending orders (for cleanup)
func (r *OrderRepository) GetPendingOrders(expiredBefore time.Time) ([]model.Order, error) {
	var orders []model.Order
	if err := r.db.Where("status = ? AND created_at < ?", "pending", expiredBefore).Find(&orders).Error; err != nil {
		return nil, err
	}
	return orders, nil
}

// CalculateUserSpent - Calculate total amount spent by user
func (r *OrderRepository) CalculateUserSpent(userID uuid.UUID) (decimal.Decimal, error) {
	var total decimal.Decimal
	err := r.db.Model(&model.Order{}).
		Where("user_id = ? AND status = ?", userID, "completed").
		Select("COALESCE(SUM(total_amount), 0)").
		Scan(&total).Error
	return total, err
}

// CreateOrderItems - Create order items within the same transaction
func (r *OrderRepository) CreateOrderItems(items []model.OrderItem) error {
	if len(items) == 0 {
		return nil
	}
	return r.db.Create(&items).Error
}

// CreateOrderHistory - Create order status history within the same transaction
func (r *OrderRepository) CreateOrderHistory(history *model.OrderStatusHistory) error {
	return r.db.Create(history).Error
}
