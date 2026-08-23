package dto

import (
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

// ===== Order DTOs =====

type CreateOrderRequest struct {
	Source      string   `json:"source"` // "cart" or "buy_now" 
	CourseIDs   []string `json:"course_ids,omitempty"`
	CouponCode  string   `json:"coupon_code,omitempty"`
	Note        string   `json:"note,omitempty"`
	IdempotencyKey string `json:"idempotency_key,omitempty"`
}

type OrderResponse struct {
	ID              uuid.UUID       `json:"id"`
	OrderNumber     string          `json:"order_number"`
	Subtotal       decimal.Decimal `json:"subtotal"`
	DiscountAmount  decimal.Decimal `json:"discount_amount"`
	TaxAmount       decimal.Decimal `json:"tax_amount"`
	TotalAmount     decimal.Decimal `json:"total_amount"`
	Currency        string          `json:"currency"` // e.g., "VND"
	Status          string          `json:"status"`
	PaymentMethod   *string         `json:"payment_method,omitempty"`
	PaymentGateway  *string         `json:"payment_gateway,omitempty"`
	PaidAt          *time.Time      `json:"paid_at,omitempty"`
	CouponID        *uuid.UUID      `json:"coupon_id,omitempty"`
	Notes           *string         `json:"notes,omitempty"`
	Items           []OrderItemResponse `json:"items"`
	CreatedAt       time.Time       `json:"created_at"`
	ExpiresAt       *time.Time      `json:"expires_at,omitempty"`

	// Payment capture - included when order is pending/processing
	PaymentCapture *PaymentCaptureInfo `json:"payment_capture,omitempty"`
}

type PaymentCaptureInfo struct {
	PaymentCode      string            `json:"payment_code,omitempty"`
	QRContent        string            `json:"qr_content,omitempty"`
	BankTransferInfo *BankTransferInfo `json:"bank_transfer_info,omitempty"`
	PaymentExpiredAt *time.Time        `json:"payment_expired_at,omitempty"`
	IsExpired        bool              `json:"is_expired"`
}

type OrderItemResponse struct {
	ID               uuid.UUID       `json:"id"`
	CourseID         uuid.UUID       `json:"course_id"`
	CourseName       string          `json:"course_name"`
	CourseThumbnail  string          `json:"course_thumbnail,omitempty"`
	CourseInstructor string          `json:"course_instructor,omitempty"`
	Price            decimal.Decimal `json:"price"`
	DiscountAmount   decimal.Decimal `json:"discount_amount"`
	FinalPrice       decimal.Decimal `json:"final_price"`
}

type OrderListResponse struct {
	Orders      []OrderResponse `json:"orders"`
	TotalCount int64           `json:"total_count"`
	Page        int            `json:"page"`
	Limit       int            `json:"limit"`
	TotalPages  int            `json:"total_pages"`
}

type CancelOrderRequest struct {
	Reason string `json:"reason"`
}

// ===== Payment DTOs =====

type CreatePaymentIntentRequest struct {
	PaymentMethod string `json:"payment_method"` // "qr_transfer", "bank_transfer"
	IdempotencyKey string `json:"idempotency_key,omitempty"`
}

type PaymentIntentResponse struct {
	OrderID        uuid.UUID  `json:"order_id"`
	PaymentCode    string     `json:"payment_code"`
	QRContent      string     `json:"qr_content,omitempty"`
	BankTransferInfo *BankTransferInfo `json:"bank_transfer_info,omitempty"`
	Amount         decimal.Decimal `json:"amount"`
	Currency       string     `json:"currency"`
	ExpiredAt      time.Time  `json:"expired_at"`
}

type BankTransferInfo struct {
	BankName      string `json:"bank_name"`
	AccountNumber string `json:"account_number"`
	AccountName   string `json:"account_name"`
	Content       string `json:"content"`
}

type PaymentStatusResponse struct {
	OrderID      uuid.UUID `json:"order_id"`
	Status       string    `json:"status"`
	PaidAt       *time.Time `json:"paid_at,omitempty"`
	Amount       decimal.Decimal `json:"amount"`
}

type PaymentWebhookRequest struct {
	Provider           string `json:"provider"`
	EventID            string `json:"event_id"`
	TransactionID      string `json:"transaction_id"`
	Amount             string `json:"amount"`
	Currency           string `json:"currency"`
	OrderNumber        string `json:"order_number"`
	Status             string `json:"status"`
	Signature          string `json:"signature"`
	Timestamp          int64  `json:"timestamp"`
	RawPayload         string `json:"-"`
	Headers            string `json:"-"`
}

// ===== Cart DTOs =====

type AddCartItemRequest struct {
	CourseID string `json:"course_id"`
}

type CartItemResponse struct {
	ID        uuid.UUID `json:"id"`
	CourseID  uuid.UUID `json:"course_id"`
	CourseName string  `json:"course_name"`
	Price     decimal.Decimal `json:"price"`
	CreatedAt time.Time `json:"created_at"`
}

type CartListResponse struct {
	Items     []CartItemResponse `json:"items"`
	Subtotal  decimal.Decimal   `json:"subtotal"`
}

// ===== Coupon DTOs =====

type ValidateCouponRequest struct {
	CouponCode string          `json:"coupon_code"`
	CourseIDs  []string       `json:"course_ids"`
	Subtotal   decimal.Decimal `json:"subtotal"`
}

type ValidateCouponResponse struct {
	Valid           bool            `json:"valid"`
	CouponCode     string          `json:"coupon_code,omitempty"`
	DiscountType   string          `json:"discount_type,omitempty"`
	DiscountValue  decimal.Decimal `json:"discount_value,omitempty"`
	MinPurchase    *decimal.Decimal `json:"min_purchase,omitempty"`
	MaxDiscount    *decimal.Decimal `json:"max_discount,omitempty"`
	DiscountAmount decimal.Decimal `json:"discount_amount"`
	Message        string          `json:"message,omitempty"`
}

// ===== Error Response =====

type ErrorResponse struct {
	Code      string      `json:"code"`
	Message   string      `json:"message"`
	Details   interface{} `json:"details,omitempty"`
	RequestID string      `json:"request_id"`
}
