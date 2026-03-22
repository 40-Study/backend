package dto

import (
	"time"

	"github.com/google/uuid"
)

// ============================================
// VOUCHER MANAGEMENT DTOs
// ============================================

type CreateVoucherRequest struct {
	Code        string `json:"code" validate:"required,min=3,max=50"`
	Name        string `json:"name" validate:"required,min=3,max=255"`
	Description string `json:"description"`

	DiscountUnit   string  `json:"discount_unit" validate:"required,oneof=MONEY POINT"`
	DiscountMethod string  `json:"discount_method" validate:"required,oneof=FIXED PERCENT"`

	DiscountAmountMoney  *float64 `json:"discount_amount_money" validate:"omitempty,min=0"`  // MONEY + FIXED
	DiscountAmountPoints *int32   `json:"discount_amount_points" validate:"omitempty,min=0"` // POINT + FIXED

	DiscountPercent *float64 `json:"discount_percent" validate:"omitempty,min=0,max=100"` // 0-100

	MaxDiscountMoney  *float64 `json:"max_discount_money" validate:"omitempty,min=0"`  // Cap cho PERCENT + MONEY
	MaxDiscountPoints *int32   `json:"max_discount_points" validate:"omitempty,min=0"` // Cap cho PERCENT + POINT

	// Minimum thresholds
	MinPurchaseMoney        *float64 `json:"min_purchase_money" validate:"omitempty,min=0"`
	MinPurchasePoints       *int32   `json:"min_purchase_points" validate:"omitempty,min=0"`
	AcceptAllPaymentMethods bool     `json:"accept_all_payment_methods"`
	PaymentMethodsAccepted  []string `json:"payment_methods_accepted"`

	UsageLimit              *int32   `json:"usage_limit" validate:"omitempty,min=0"`    // Tổng số lần dùng
	UsagePerUser            *int32   `json:"usage_per_user" validate:"omitempty,min=0"` // Giới hạn per user
	CanStack                *bool    `json:"can_stack"`                                 // Có được dùng cùng voucher khác không

	StartDate *time.Time `json:"start_date"`
	EndDate   *time.Time `json:"end_date"`
	IsActive  *bool      `json:"is_active"`
}

type UpdateVoucherRequest struct {
	Name        *string `json:"name" validate:"omitempty,min=3,max=255"`
	Description *string `json:"description"`

	DiscountAmountMoney  *float64 `json:"discount_amount_money" validate:"omitempty,min=0"`
	DiscountAmountPoints *int32   `json:"discount_amount_points" validate:"omitempty,min=0"`
	DiscountPercent      *float64 `json:"discount_percent" validate:"omitempty,min=0,max=100"`

	MaxDiscountMoney  *float64 `json:"max_discount_money" validate:"omitempty,min=0"`
	MaxDiscountPoints *int32   `json:"max_discount_points" validate:"omitempty,min=0"`

	MinPurchaseMoney  *float64 `json:"min_purchase_money" validate:"omitempty,min=0"`
	MinPurchasePoints *int32   `json:"min_purchase_points" validate:"omitempty,min=0"`

	UsageLimit   *int32 `json:"usage_limit" validate:"omitempty,min=0"`
	UsagePerUser *int32 `json:"usage_per_user" validate:"omitempty,min=0"`
	CanStack     *bool  `json:"can_stack"`

	StartDate *time.Time `json:"start_date"`
	EndDate   *time.Time `json:"end_date"`
	IsActive  *bool      `json:"is_active"`
}

type GetVouchersRequest struct {
	Limit     int        `json:"limit" validate:"omitempty,min=1,max=100"`
	Offset    int        `json:"offset" validate:"omitempty,min=0"`
	Keyword   string     `json:"keyword"`
	StartTime *time.Time `json:"start_time"`
	EndTime   *time.Time `json:"end_time"`
	IsActive  *bool      `json:"is_active"`
	DelMode   int        `json:"del_mode"`
}

type SaveVoucherRequest struct {
	VoucherCode string `json:"voucher_code" validate:"required"`
	Source      string `json:"source"` // manual, admin_grant, event_reward, etc.
	Notes       string `json:"notes"`
}

type UserSavedVoucherResponse struct {
	ID        uuid.UUID `json:"id"`
	UserID    uuid.UUID `json:"user_id"`
	VoucherID uuid.UUID `json:"voucher_id"`
	SavedAt   time.Time `json:"saved_at"`
	Source    string    `json:"source"`
	Notes     string    `json:"notes"`
	// Voucher   *model.Voucher `json:"voucher,omitempty"` // Preload voucher data
}

type ValidateVoucherRequest struct {
	UserID      uuid.UUID   `json:"user_id" validate:"required"`
	VoucherCode string      `json:"voucher_code" validate:"required"`
	OrderType   string      `json:"order_type" validate:"required"` // food, game_rental, vip, etc.
	OrderAmount int64       `json:"order_amount" validate:"required,min=0"`
	OrderPoints *int32      `json:"order_points"` // Nếu order dùng points
	OrderItems  []OrderItem `json:"order_items" validate:"required,dive,min=1"` // Items trong order để check applicability
}

type OrderItem struct {
	ItemID   uuid.UUID `json:"item_id" validate:"required"`
	ItemType string    `json:"item_type" validate:"required"` // food, game_account, vip_package, etc.
	Quantity int32     `json:"quantity" validate:"required,min=1"`
	Price    int64     `json:"price" validate:"required,min=0"`
}

type VoucherValidationResult struct {
	IsValid            bool        `json:"is_valid"`
	ErrorCode          string      `json:"error_code,omitempty"` // voucher_expired, usage_limit_reached, min_purchase_not_met, etc.
	ErrorMessage       string      `json:"error_message,omitempty"`
	// Voucher            *model.Voucher `json:"voucher,omitempty"`
	DiscountAmount     int64   `json:"discount_amount"`                // Số tiền/points sẽ được giảm (preview)
	FinalAmount        int64   `json:"final_amount"`                  // Tổng sau khi giảm
	CanApply           bool    `json:"can_apply"`                     // Có thể apply không
	RemainingUsage     *int32  `json:"remaining_usage,omitempty"`      // Số lần còn lại có thể dùng
	RemainingUserUsage *int32  `json:"remaining_user_usage,omitempty"` // Số lần user còn dùng được
}

type ApplyVoucherRequest struct {
	UserID      uuid.UUID   `json:"user_id" validate:"required"`
	VoucherCode string      `json:"voucher_code" validate:"required"`
	OrderID     uuid.UUID   `json:"order_id" validate:"required"`
	OrderType   string      `json:"order_type" validate:"required"`
	OrderAmount int64       `json:"order_amount" validate:"required,min=0"`
	OrderPoints *int32      `json:"order_points"`
	OrderItems  []OrderItem `json:"order_items" validate:"required,dive,min=1"`
}

type VoucherApplicationResult struct {
	Success        bool        `json:"success"`
	VoucherID      uuid.UUID   `json:"voucher_id"`
	VoucherCode    string      `json:"voucher_code"`
	DiscountAmount int64       `json:"discount_amount"` // Số tiền đã giảm
	FinalAmount    int64       `json:"final_amount"`     // Tổng sau khi giảm
	// VoucherLog     *model.VoucherLog `json:"voucher_log,omitempty"` // Log entry đã tạo
	ErrorMessage string `json:"error_message,omitempty"`
}

type CreateApplicabilityRequest struct {
	ApplicableType string    `json:"applicable_type" validate:"required"` // USER, ROLE, TIER, GROUP, PRODUCT, CATEGORY, ALL, SERVICE, BRANCH
	ApplicableID   uuid.UUID `json:"applicable_id" validate:"required"` // UUID của entity áp dụng
}

type UpdateApplicabilityRequest struct {
	ApplicableType *string    `json:"applicable_type"`
	ApplicableID   *uuid.UUID `json:"applicable_id"`
}

type VoucherLogResponse struct {
	ID          uuid.UUID              `json:"id"`
	UserID      uuid.UUID              `json:"user_id"`
	VoucherID   uuid.UUID              `json:"voucher_id"`
	VoucherCode string                 `json:"voucher_code"`
	OrderID     uuid.UUID              `json:"order_id"`
	OrderType   string                 `json:"order_type"`
	Action      string                 `json:"action"` // used, refunded
	Amount      int64                  `json:"amount"` // Discount amount
	CreatedAt   time.Time              `json:"created_at"`
	Metadata    map[string]interface{} `json:"metadata,omitempty"`
}

type VoucherStatsResponse struct {
	VoucherID      uuid.UUID `json:"voucher_id"`
	VoucherCode    string    `json:"voucher_code"`
	VoucherName    string    `json:"voucher_name"`
	TotalUsed      int64     `json:"total_used"`
	TotalDiscount  int64     `json:"total_discount"` // Tổng tiền đã giảm
	UniqueUsers    int64     `json:"unique_users"`
	UsageLimit     *int32    `json:"usage_limit,omitempty"`
	RemainingUsage *int32    `json:"remaining_usage,omitempty"`
}

type TopVoucherResponse struct {
	VoucherID     uuid.UUID `json:"voucher_id"`
	VoucherCode   string    `json:"voucher_code"`
	VoucherName   string    `json:"voucher_name"`
	UsageCount    int64     `json:"usage_count"`
	TotalDiscount int64     `json:"total_discount"`
	UniqueUsers   int64     `json:"unique_users"`
}

type VoucherUsageTrendResponse struct {
	Date       time.Time `json:"date"`
	UsageCount int64     `json:"usage_count"`
	Discount   int64     `json:"discount"`
	UserCount  int64     `json:"user_count"`
}

type PublicVoucherResponse struct {
	ID          uuid.UUID `json:"id"`
	Code        string    `json:"code"`
	Name        string    `json:"name"`
	Description string    `json:"description"`

	DiscountUnit   string `json:"discount_unit"`
	DiscountMethod string `json:"discount_method"`

	DiscountDisplay string `json:"discount_display"`

	MinPurchaseDisplay string `json:"min_purchase_display,omitempty"` // "Đơn tối thiểu 200,000đ"

	UsageLimit     *int32     `json:"usage_limit,omitempty"`
	RemainingUsage *int32     `json:"remaining_usage,omitempty"`
	StartDate      *time.Time `json:"start_date,omitempty"`
	EndDate        *time.Time `json:"end_date,omitempty"`

	IsSaved bool `json:"is_saved"` // User đã save voucher này chưa (if authenticated)
}
