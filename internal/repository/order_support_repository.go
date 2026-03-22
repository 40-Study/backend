package repository

import (
	"errors"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"study.com/v1/internal/model"
)

var (
	ErrHistoryNotFound = errors.New("history not found")
)

// OrderStatusHistoryRepository - Repository for OrderStatusHistory
type OrderStatusHistoryRepository struct {
	db *gorm.DB
}

func NewOrderStatusHistoryRepository(db *gorm.DB) *OrderStatusHistoryRepository {
	return &OrderStatusHistoryRepository{db: db}
}

// Create - Create new history entry
func (r *OrderStatusHistoryRepository) Create(history *model.OrderStatusHistory) error {
	return r.db.Create(history).Error
}

// GetByOrderID - Get history by order ID
func (r *OrderStatusHistoryRepository) GetByOrderID(orderID uuid.UUID) ([]model.OrderStatusHistory, error) {
	var histories []model.OrderStatusHistory
	if err := r.db.Where("order_id = ?", orderID).
		Order("created_at DESC").
		Find(&histories).Error; err != nil {
		return nil, err
	}
	return histories, nil
}

// GetLatestByOrderID - Get latest status for an order
func (r *OrderStatusHistoryRepository) GetLatestByOrderID(orderID uuid.UUID) (*model.OrderStatusHistory, error) {
	var history model.OrderStatusHistory
	if err := r.db.Where("order_id = ?", orderID).
		Order("created_at DESC").
		First(&history).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrHistoryNotFound
		}
		return nil, err
	}
	return &history, nil
}

// ===== PaymentEventRepository =====

var (
	ErrPaymentEventNotFound = errors.New("payment event not found")
)

type PaymentEventRepository struct {
	db *gorm.DB
}

func NewPaymentEventRepository(db *gorm.DB) *PaymentEventRepository {
	return &PaymentEventRepository{db: db}
}

// Create - Create new payment event
func (r *PaymentEventRepository) Create(event *model.PaymentEvent) error {
	return r.db.Create(event).Error
}

// GetByProviderAndEventID - Get event by provider and event ID (for idempotency)
func (r *PaymentEventRepository) GetByProviderAndEventID(provider, eventID string) (*model.PaymentEvent, error) {
	var event model.PaymentEvent
	if err := r.db.First(&event, "provider = ? AND event_id = ?", provider, eventID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrPaymentEventNotFound
		}
		return nil, err
	}
	return &event, nil
}

// GetByProviderAndTransactionID - Get event by provider and transaction ID
func (r *PaymentEventRepository) GetByProviderAndTransactionID(provider, transactionID string) (*model.PaymentEvent, error) {
	var event model.PaymentEvent
	if err := r.db.First(&event, "provider = ? AND transaction_id = ?", provider, transactionID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrPaymentEventNotFound
		}
		return nil, err
	}
	return &event, nil
}

// UpdateStatus - Update payment event status
func (r *PaymentEventRepository) UpdateStatus(id uuid.UUID, status string, errorMessage *string) error {
	updates := map[string]interface{}{
		"status":        status,
		"processed_at":  time.Now(),
	}
	if errorMessage != nil {
		updates["error_message"] = *errorMessage
	}
	return r.db.Model(&model.PaymentEvent{}).Where("id = id", id).Updates(updates).Error
}

// IncrementRetryCount - Increment retry count
func (r *PaymentEventRepository) IncrementRetryCount(id uuid.UUID) error {
	return r.db.Model(&model.PaymentEvent{}).
		Where("id = ?", id).
		Update("retry_count", gorm.Expr("retry_count + 1")).Error
}

// GetRetryableEvents - Get events that are retryable
func (r *PaymentEventRepository) GetRetryableEvents(limit int) ([]model.PaymentEvent, error) {
	var events []model.PaymentEvent
	if err := r.db.Where("status = ? AND is_retryable = ? AND retry_count < ?", "pending", true, 5).
		Order("created_at ASC").
		Limit(limit).
		Find(&events).Error; err != nil {
		return nil, err
	}
	return events, nil
}

// ===== IdempotencyKeyRepository =====

var (
	ErrIdempotencyKeyNotFound = errors.New("idempotency key not found")
)

type IdempotencyKeyRepository struct {
	db *gorm.DB
}

func NewIdempotencyKeyRepository(db *gorm.DB) *IdempotencyKeyRepository {
	return &IdempotencyKeyRepository{db: db}
}

// Create - Create new idempotency key
func (r *IdempotencyKeyRepository) Create(key *model.IdempotencyKey) error {
	return r.db.Create(key).Error
}

// GetByScopeAndKey - Get idempotency key by scope and key
func (r *IdempotencyKeyRepository) GetByScopeAndKey(scope, key string) (*model.IdempotencyKey, error) {
	var idemKey model.IdempotencyKey
	if err := r.db.First(&idemKey, "scope = ? AND key = ?", scope, key).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrIdempotencyKeyNotFound
		}
		return nil, err
	}
	return &idemKey, nil
}

// UpdateResponse - Update response for idempotency key
func (r *IdempotencyKeyRepository) UpdateResponse(id uuid.UUID, responseCode int, responseBody string) error {
	return r.db.Model(&model.IdempotencyKey{}).
		Where("id = ?", id).
		Updates(map[string]interface{}{
			"response_code": responseCode,
			"response_body": responseBody,
		}).Error
}

// DeleteExpired - Delete expired idempotency keys
func (r *IdempotencyKeyRepository) DeleteExpired(before time.Time) error {
	return r.db.Where("expires_at < ?", before).Delete(&model.IdempotencyKey{}).Error
}

// ===== OrderLockRepository =====

type OrderLockRepository struct {
	db *gorm.DB
}

func NewOrderLockRepository(db *gorm.DB) *OrderLockRepository {
	return &OrderLockRepository{db: db}
}

// AcquireLock - Try to acquire lock for order
func (r *OrderLockRepository) AcquireLock(orderID uuid.UUID, owner string, duration time.Duration) (bool, error) {
	lockUntil := time.Now().Add(duration)

	var existingLock model.OrderLock
	err := r.db.First(&existingLock, "order_id = ?", orderID).Error

	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			// Create new lock
			lock := model.OrderLock{
				OrderID:   orderID,
				LockOwner: owner,
				LockUntil: &lockUntil,
			}
			if err := r.db.Create(&lock).Error; err != nil {
				return false, err
			}
			return true, nil
		}
		return false, err
	}

	// Check if lock is expired
	if existingLock.LockUntil != nil && time.Now().After(*existingLock.LockUntil) {
		// Update lock
		existingLock.LockOwner = owner
		existingLock.LockUntil = &lockUntil
		if err := r.db.Save(&existingLock).Error; err != nil {
			return false, err
		}
		return true, nil
	}

	// Lock is held by another owner
	return false, nil
}

// ReleaseLock - Release lock for order
func (r *OrderLockRepository) ReleaseLock(orderID uuid.UUID, owner string) error {
	return r.db.Where("order_id = ? AND lock_owner = ?", orderID, owner).Delete(&model.OrderLock{}).Error
}

// GetLockOwner - Get current lock owner
func (r *OrderLockRepository) GetLockOwner(orderID uuid.UUID) (*string, error) {
	var lock model.OrderLock
	if err := r.db.First(&lock, "order_id = ?", orderID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}

	if lock.LockUntil != nil && time.Now().After(*lock.LockUntil) {
		return nil, nil
	}

	return &lock.LockOwner, nil
}
