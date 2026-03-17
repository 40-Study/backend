package model

import (
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

// OrderStatusHistory - Audit trail for order status transitions
type OrderStatusHistory struct {
	ID         uuid.UUID `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	CreatedAt  time.Time `json:"created_at"`
	OrderID    uuid.UUID `gorm:"type:uuid;not null;index" json:"order_id"`
	FromStatus string    `gorm:"type:varchar(20)" json:"from_status"`
	ToStatus   string    `gorm:"type:varchar(20);not null" json:"to_status"`
	Reason     string    `gorm:"type:text" json:"reason"`
	Actor      *string   `gorm:"type:varchar(100)" json:"actor"`

	// Relationships
	Order Order `gorm:"foreignKey:OrderID;constraint:OnDelete:CASCADE" json:"-"`
}

func (OrderStatusHistory) TableName() string {
	return "order_status_histories"
}

// PaymentEvent - Store payment webhook events for idempotency
type PaymentEvent struct {
	ID                 uuid.UUID       `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	CreatedAt          time.Time       `json:"created_at"`
	Provider           string          `gorm:"type:varchar(30);not null;index" json:"provider"`
	EventID            string          `gorm:"type:varchar(255);not null" json:"event_id"`
	OrderID            *uuid.UUID      `gorm:"type:uuid;index" json:"order_id"`
	TransactionID      string          `gorm:"type:varchar(255);not null" json:"transaction_id"`
	Amount             decimal.Decimal `gorm:"type:decimal(12,2);not null" json:"amount"`
	Currency           string          `gorm:"type:varchar(3);default:'VND'" json:"currency"`
	RawPayload         string          `gorm:"type:text" json:"raw_payload"`
	Headers            string          `gorm:"type:text" json:"headers"`
	ProcessedAt        *time.Time      `json:"processed_at"`
	Status             string          `gorm:"type:varchar(20);default:'pending';check:status IN ('pending', 'processed', 'failed', 'duplicate')" json:"status"`
	ErrorMessage       *string         `gorm:"type:text" json:"error_message,omitempty"`
	RetryCount         int             `gorm:"default:0" json:"retry_count"`
	IsRetryable        bool            `gorm:"default:true" json:"is_retryable"`
}

func (PaymentEvent) TableName() string {
	return "payment_events"
}

// IdempotencyKey - Store idempotency keys for API requests
type IdempotencyKey struct {
	ID              uuid.UUID  `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	CreatedAt       time.Time  `json:"created_at"`
	Key             string     `gorm:"type:varchar(255);not null" json:"key"`
	Scope           string     `gorm:"type:varchar(50);not null" json:"scope"` // e.g., "create_order", "payment_intent"
	RequestHash     string     `gorm:"type:varchar(64);not null" json:"request_hash"`
	ResponseCode    int        `gorm:"type:int" json:"response_code"`
	ResponseBody    string     `gorm:"type:text" json:"response_body"`
	ExpiresAt       time.Time  `gorm:"type:timestamp;not null;index" json:"expires_at"`
	UserID          *uuid.UUID `gorm:"type:uuid;index" json:"user_id"`
}

func (IdempotencyKey) TableName() string {
	return "idempotency_keys"
}

// OrderLock - Advisory lock for optimistic concurrency control
type OrderLock struct {
	ID         uuid.UUID  `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	CreatedAt  time.Time  `json:"created_at"`
	OrderID    uuid.UUID  `gorm:"type:uuid;uniqueIndex;not null" json:"order_id"`
	LockOwner  string     `gorm:"type:varchar(100);not null" json:"lock_owner"`
	LockUntil  *time.Time `json:"lock_until"`
}

func (OrderLock) TableName() string {
	return "order_locks"
}
