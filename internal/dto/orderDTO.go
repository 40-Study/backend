package dto

import (
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

// ===== Order DTOs =====

// M-01 (audit 260909 vòng 2): thêm validate tag cho Source (bắt buộc, chỉ 2 giá trị hợp lệ)
// và CourseIDs (mỗi phần tử phải là UUID hợp lệ khi source=buy_now) — trước đây không có
// tag nào nên ValidateStruct (nếu handler có gọi) cũng không kiểm tra được gì trên DTO này.
type CreateOrderRequest struct {
	Source         string   `json:"source" validate:"required,oneof=cart buy_now"` // "cart" or "buy_now"
	CourseIDs      []string `json:"course_ids,omitempty" validate:"omitempty,dive,uuid"`
	CouponCode     string   `json:"coupon_code,omitempty"`
	Note           string   `json:"note,omitempty"`
	IdempotencyKey string   `json:"idempotency_key,omitempty"`
}

type OrderResponse struct {
	ID             uuid.UUID           `json:"id"`
	OrderNumber    string              `json:"order_number"`
	Subtotal       decimal.Decimal     `json:"subtotal"`
	DiscountAmount decimal.Decimal     `json:"discount_amount"`
	TaxAmount      decimal.Decimal     `json:"tax_amount"`
	TotalAmount    decimal.Decimal     `json:"total_amount"`
	Currency       string              `json:"currency"` // e.g., "VND"
	Status         string              `json:"status"`
	PaymentMethod  *string             `json:"payment_method,omitempty"`
	PaymentGateway *string             `json:"payment_gateway,omitempty"`
	PaidAt         *time.Time          `json:"paid_at,omitempty"`
	CouponID       *uuid.UUID          `json:"coupon_id,omitempty"`
	Notes          *string             `json:"notes,omitempty"`
	Items          []OrderItemResponse `json:"items"`
	CreatedAt      time.Time           `json:"created_at"`
	ExpiresAt      *time.Time          `json:"expires_at,omitempty"`
}

type OrderItemResponse struct {
	ID             uuid.UUID       `json:"id"`
	CourseID       uuid.UUID       `json:"course_id"`
	CourseName     string          `json:"course_name"`
	Price          decimal.Decimal `json:"price"`
	DiscountAmount decimal.Decimal `json:"discount_amount"`
	FinalPrice     decimal.Decimal `json:"final_price"`
}

type OrderListResponse struct {
	Orders     []OrderResponse `json:"orders"`
	TotalCount int64           `json:"total_count"`
	Page       int             `json:"page"`
	Limit      int             `json:"limit"`
	TotalPages int             `json:"total_pages"`
}

type CancelOrderRequest struct {
	Reason string `json:"reason"`
}

// ===== Payment DTOs =====

type CreatePaymentIntentRequest struct {
	PaymentMethod  string `json:"payment_method" validate:"required,oneof=qr_transfer bank_transfer"`
	IdempotencyKey string `json:"idempotency_key,omitempty"`
}

type PaymentIntentResponse struct {
	OrderID          uuid.UUID         `json:"order_id"`
	PaymentCode      string            `json:"payment_code"`
	QRContent        string            `json:"qr_content,omitempty"`
	BankTransferInfo *BankTransferInfo `json:"bank_transfer_info,omitempty"`
	Amount           decimal.Decimal   `json:"amount"`
	Currency         string            `json:"currency"`
	ExpiredAt        time.Time         `json:"expired_at"`
}

type BankTransferInfo struct {
	BankName      string `json:"bank_name"`
	AccountNumber string `json:"account_number"`
	AccountName   string `json:"account_name"`
	Content       string `json:"content"`
}

type PaymentStatusResponse struct {
	OrderID uuid.UUID       `json:"order_id"`
	Status  string          `json:"status"`
	PaidAt  *time.Time      `json:"paid_at,omitempty"`
	Amount  decimal.Decimal `json:"amount"`
}

type PaymentWebhookRequest struct {
	Provider      string `json:"provider"`
	EventID       string `json:"event_id"`
	TransactionID string `json:"transaction_id"`
	Amount        string `json:"amount"`
	Currency      string `json:"currency"`
	OrderNumber   string `json:"order_number"`
	Status        string `json:"status"`
	Signature     string `json:"signature"`
	Timestamp     int64  `json:"timestamp"`
	RawPayload    string `json:"-"`
	Headers       string `json:"-"`
}

// ===== Cart DTOs =====

type AddCartItemRequest struct {
	CourseID string `json:"course_id"`
}

type CartItemResponse struct {
	ID         uuid.UUID       `json:"id"`
	CourseID   uuid.UUID       `json:"course_id"`
	CourseName string          `json:"course_name"`
	Price      decimal.Decimal `json:"price"`
	CreatedAt  time.Time       `json:"created_at"`
}

type CartListResponse struct {
	Items    []CartItemResponse `json:"items"`
	Subtotal decimal.Decimal    `json:"subtotal"`
}

// ===== Coupon DTOs =====

type ValidateCouponRequest struct {
	CouponCode string          `json:"coupon_code"`
	CourseIDs  []string        `json:"course_ids"`
	Subtotal   decimal.Decimal `json:"subtotal"`
}

type ValidateCouponResponse struct {
	Valid          bool             `json:"valid"`
	CouponCode     string           `json:"coupon_code,omitempty"`
	DiscountType   string           `json:"discount_type,omitempty"`
	DiscountValue  decimal.Decimal  `json:"discount_value,omitempty"`
	MinPurchase    *decimal.Decimal `json:"min_purchase,omitempty"`
	MaxDiscount    *decimal.Decimal `json:"max_discount,omitempty"`
	DiscountAmount decimal.Decimal  `json:"discount_amount"`
	Message        string           `json:"message,omitempty"`
}

// ===== Admin Orders / Refund / Revenue Report =====
// Tính năng đơn hàng+hoàn tiền+doanh thu nền tảng (quyết định chủ dự án 27/09/2026).

// AdminOrderItemBrief — 1 dòng khoá học trong đơn, hiển thị ở danh sách admin.
type AdminOrderItemBrief struct {
	CourseID    uuid.UUID       `json:"course_id"`
	CourseTitle string          `json:"course_title"`
	FinalPrice  decimal.Decimal `json:"final_price"`
}

// AdminOrderListItem — 1 dòng đơn hàng trong GET /orders/admin.
type AdminOrderListItem struct {
	ID            uuid.UUID             `json:"id"`
	OrderNumber   string                `json:"order_number"`
	UserID        uuid.UUID             `json:"user_id"`
	UserEmail     string                `json:"user_email"`
	TotalAmount   decimal.Decimal       `json:"total_amount"`
	Currency      string                `json:"currency"`
	Status        string                `json:"status"`
	PaymentMethod *string               `json:"payment_method,omitempty"`
	CreatedAt     time.Time             `json:"created_at"`
	PaidAt        *time.Time            `json:"paid_at,omitempty"`
	Items         []AdminOrderItemBrief `json:"items"`
}

// AdminOrderListResponse — envelope phân trang (mẫu OrderListResponse chuẩn mới của 4 phase).
type AdminOrderListResponse struct {
	Items      []AdminOrderListItem `json:"items"`
	TotalCount int64                `json:"total_count"`
	Page       int                  `json:"page"`
	Limit      int                  `json:"limit"`
	TotalPages int                  `json:"total_pages"`
}

// RefundOrderRequest — POST /orders/admin/:id/refund.
type RefundOrderRequest struct {
	Reason       string `json:"reason" validate:"required,max=500"`
	RefundMethod string `json:"refund_method" validate:"required,oneof=manual_bank_transfer wallet_credit"`
}

// RefundOrderResponse — 200 của POST /orders/admin/:id/refund.
type RefundOrderResponse struct {
	ID         uuid.UUID `json:"id"`
	Status     string    `json:"status"`
	RefundedAt time.Time `json:"refunded_at"`
}

// RevenueReportResponse — GET /admin/reports/revenue.
//
// PlatformFeeAmount/TeacherShareAmount (LỆCH so với ví dụ JSON gốc ở phase-02-orders-refund.md —
// xem ghi chú "Deviation" đầu file đó): quyết định chủ dự án #2 yêu cầu báo cáo hiện "gộp / phí
// nền tảng / phần giảng viên" — 2 field này bổ sung, KHÔNG có trong bản phác thảo ban đầu.
type RevenueReportResponse struct {
	GrossRevenue       decimal.Decimal  `json:"gross_revenue"`
	RefundAmount       decimal.Decimal  `json:"refund_amount"`
	NetRevenue         decimal.Decimal  `json:"net_revenue"`
	PlatformFeeAmount  decimal.Decimal  `json:"platform_fee_amount"`
	TeacherShareAmount decimal.Decimal  `json:"teacher_share_amount"`
	TransactionCount   int64            `json:"transaction_count"`
	CompletedCount     int64            `json:"completed_count"`
	RefundedCount      int64            `json:"refunded_count"`
	SuccessRate        float64          `json:"success_rate"`
	Currency           string           `json:"currency"`
	ByStatus           map[string]int64 `json:"by_status"`
}

// PlatformFeeSettingResponse — GET/PUT /admin/settings/platform-fee.
type PlatformFeeSettingResponse struct {
	PlatformFeePercent decimal.Decimal `json:"platform_fee_percent"`
}

// UpdatePlatformFeeRequest — PUT /admin/settings/platform-fee. Không dùng tag "required": 0%
// (giá trị mặc định/hợp lệ) là input HỢP LỆ — validator coi struct decimal.Decimal zero-value là
// "rỗng" nên "required" sẽ từ chối nhầm 0%. Khoảng hợp lệ [0,100] được kiểm ở service layer
// (PlatformSettingService.SetPlatformFeePercent).
type UpdatePlatformFeeRequest struct {
	PlatformFeePercent decimal.Decimal `json:"platform_fee_percent"`
}

// ===== Error Response =====

type ErrorResponse struct {
	Code      string      `json:"code"`
	Message   string      `json:"message"`
	Details   interface{} `json:"details,omitempty"`
	RequestID string      `json:"request_id"`
}
