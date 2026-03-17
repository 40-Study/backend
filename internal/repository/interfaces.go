package repository

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"gorm.io/gorm"
	"study.com/v1/internal/model"
)

// OrderRepositoryInterface defines the interface for Order operations
type OrderRepositoryInterface interface {
	Create(order *model.Order) error
	GetByID(id uuid.UUID) (*model.Order, error)
	GetByOrderNumber(orderNumber string) (*model.Order, error)
	GetByUserID(userID uuid.UUID, page, limit int, status string) ([]model.Order, int64, error)
	GetByUserIDAndStatus(userID uuid.UUID, status string) ([]model.Order, error)
	UpdateStatus(orderID uuid.UUID, status string) error
	UpdatePaymentInfo(orderID uuid.UUID, paymentMethod, paymentGateway, transactionID string, paidAt time.Time) error
	Update(order *model.Order) error
	Delete(id uuid.UUID) error
	WithTransaction(fn func(repo *OrderRepository) error) error
	GetForUpdate(tx *gorm.DB, id uuid.UUID) (*model.Order, error)
	UpdateStatusWithTx(tx *gorm.DB, orderID uuid.UUID, status string) error
	CheckOrderNumberExists(orderNumber string) (bool, error)
	GetPendingOrders(expiredBefore time.Time) ([]model.Order, error)
	CalculateUserSpent(userID uuid.UUID) (decimal.Decimal, error)
}

// OrderItemRepositoryInterface defines the interface for OrderItem operations
type OrderItemRepositoryInterface interface {
	Create(item *model.OrderItem) error
	CreateBatch(items []model.OrderItem) error
	GetByID(id uuid.UUID) (*model.OrderItem, error)
	GetByOrderID(orderID uuid.UUID) ([]model.OrderItem, error)
	GetByOrderIDAndCourseID(orderID, courseID uuid.UUID) (*model.OrderItem, error)
	GetCourseIDsByOrderID(orderID uuid.UUID) ([]uuid.UUID, error)
	DeleteByOrderID(orderID uuid.UUID) error
	Delete(id uuid.UUID) error
	WithTransaction(fn func(repo *OrderItemRepository) error) error
	CheckDuplicate(orderID, courseID uuid.UUID) (bool, error)
	CalculateTotal(orderID uuid.UUID) (interface{}, error)
}

// CouponRepositoryInterface defines the interface for Coupon operations
type CouponRepositoryInterface interface {
	Create(coupon *model.Coupon) error
	GetByID(id uuid.UUID) (*model.Coupon, error)
	GetByCode(code string) (*model.Coupon, error)
	ValidateCoupon(code string, userID uuid.UUID, courseIDs []uuid.UUID, subtotal decimal.Decimal) (*model.Coupon, decimal.Decimal, error)
	IncrementUsageCount(couponID uuid.UUID) error
	CreateUsage(usage *model.CouponUsage) error
	GetUsageByUserAndCoupon(couponID, userID uuid.UUID) ([]model.CouponUsage, error)
	GetAll(page, limit int, isActive *bool) ([]model.Coupon, int64, error)
	Update(coupon *model.Coupon) error
	Delete(id uuid.UUID) error
}

// OrderStatusHistoryRepositoryInterface defines the interface for OrderStatusHistory operations
type OrderStatusHistoryRepositoryInterface interface {
	Create(history *model.OrderStatusHistory) error
	GetByOrderID(orderID uuid.UUID) ([]model.OrderStatusHistory, error)
	GetLatestByOrderID(orderID uuid.UUID) (*model.OrderStatusHistory, error)
}

// PaymentEventRepositoryInterface defines the interface for PaymentEvent operations
type PaymentEventRepositoryInterface interface {
	Create(event *model.PaymentEvent) error
	GetByProviderAndEventID(provider, eventID string) (*model.PaymentEvent, error)
	GetByProviderAndTransactionID(provider, transactionID string) (*model.PaymentEvent, error)
	UpdateStatus(id uuid.UUID, status string, errorMessage *string) error
	IncrementRetryCount(id uuid.UUID) error
	GetRetryableEvents(limit int) ([]model.PaymentEvent, error)
}

// IdempotencyKeyRepositoryInterface defines the interface for IdempotencyKey operations
type IdempotencyKeyRepositoryInterface interface {
	Create(key *model.IdempotencyKey) error
	GetByScopeAndKey(scope, key string) (*model.IdempotencyKey, error)
	UpdateResponse(id uuid.UUID, responseCode int, responseBody string) error
	DeleteExpired(before time.Time) error
}

// OrderLockRepositoryInterface defines the interface for OrderLock operations
type OrderLockRepositoryInterface interface {
	AcquireLock(orderID uuid.UUID, owner string, duration time.Duration) (bool, error)
	ReleaseLock(orderID uuid.UUID, owner string) error
	GetLockOwner(orderID uuid.UUID) (*string, error)
}

// CartItemRepositoryInterface defines the interface for CartItem operations
type CartItemRepositoryInterface interface {
	Create(ctx context.Context, item *model.CartItem) error
	Delete(ctx context.Context, userID, courseID uuid.UUID) error
	DeleteByUserID(ctx context.Context, userID uuid.UUID) error
	GetByUserID(ctx context.Context, userID uuid.UUID) ([]model.CartItem, error)
	Exists(ctx context.Context, userID, courseID uuid.UUID) (bool, error)
}
