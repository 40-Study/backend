package model

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/lib/pq"
	"github.com/shopspring/decimal"
	"gorm.io/gorm"
)

// 6. Banner (Bảng banner quảng cáo)
type Banner struct {
	ID             uuid.UUID              `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	Title          string                 `gorm:"not null" json:"title"`
	Description    pgtype.Text            `json:"description"`
	ImageUrl       string                 `gorm:"not null" json:"image_url"`
	LinkUrl        pgtype.Text            `json:"link_url"`
	BannerType     pgtype.Text            `json:"banner_type"`
	Position       pgtype.Text            `json:"position"`
	TargetAudience map[string]interface{} `gorm:"type:jsonb;serializer:json" json:"target_audience"`
	DisplayOrder   pgtype.Int4            `json:"display_order"`
	StartDate      pgtype.Timestamp       `json:"start_date"`
	EndDate        pgtype.Timestamp       `json:"end_date"`
	IsActive       pgtype.Bool            `gorm:"default:true" json:"is_active"`
	ClickCount     pgtype.Int4            `gorm:"default:0" json:"click_count"`
	CreatedAt      time.Time              `gorm:"autoCreateTime" json:"created_at"`
	UpdatedAt      time.Time              `gorm:"autoUpdateTime" json:"updated_at"`
	DeletedAt      gorm.DeletedAt         `gorm:"index" json:"deleted_at"`
}

func (Banner) TableName() string {
	return "banners"
}

// 50. Voucher (Bảng voucher)
type DiscountUnit string

const (
	DiscountUnitMoney DiscountUnit = "MONEY"
	DiscountUnitPoint DiscountUnit = "POINT"
)

type DiscountMethod string

const (
	DiscountMethodFixed   DiscountMethod = "FIXED"
	DiscountMethodPercent DiscountMethod = "PERCENT"
)

type Voucher struct {
	ID          uuid.UUID   `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	Code        string      `gorm:"unique;not null" json:"code"`
	Name        string      `gorm:"not null" json:"name"`
	Description pgtype.Text `json:"description"`

	DiscountUnit   DiscountUnit   `gorm:"type:varchar(10);not null" json:"discount_unit"`   // MONEY hoặc POINT
	DiscountMethod DiscountMethod `gorm:"type:varchar(10);not null" json:"discount_method"` // FIXED hoặc PERCENT

	// Giá trị giảm theo từng loại
	DiscountAmountMoney  *decimal.Decimal `gorm:"type:numeric" json:"discount_amount_money"` // Dùng khi FIXED + MONEY
	DiscountAmountPoints pgtype.Int4      `json:"discount_amount_points"`                    // Dùng khi FIXED + POINT
	DiscountPercent      *decimal.Decimal `gorm:"type:numeric" json:"discount_percent"`      // Dùng khi PERCENT

	MaxDiscountMoney  *decimal.Decimal `gorm:"type:numeric" json:"max_discount_money"`
	MaxDiscountPoints pgtype.Int4      `json:"max_discount_points"`

	MinPurchaseMoney        *decimal.Decimal `gorm:"type:numeric" json:"min_purchase_money"`
	MinPurchasePoints       pgtype.Int4      `json:"min_purchase_points"`
	AcceptAllPaymentMethods bool             `json:"accept_all_payment_methods"`
	PaymentMethodsAccepted  pq.StringArray   `gorm:"type:text[]" json:"payment_methods_accepted"`

	UsageLimit   pgtype.Int4 `json:"usage_limit"`
	UsagePerUser pgtype.Int4 `json:"usage_per_user"`
	UsedCount    pgtype.Int4 `gorm:"default:0" json:"used_count"`
	CanStack     pgtype.Bool `gorm:"not null;default:false" json:"can_stack"`

	StartDate pgtype.Timestamp `json:"start_date"`
	EndDate   pgtype.Timestamp `json:"end_date"`
	IsActive  pgtype.Bool      `gorm:"not null;default:true" json:"is_active"`

	CreatedAt time.Time      `gorm:"autoCreateTime" json:"created_at"`
	UpdatedAt time.Time      `gorm:"autoUpdateTime" json:"updated_at"`
	DeletedAt gorm.DeletedAt `gorm:"index" json:"deleted_at"`
}

func (Voucher) TableName() string { return "vouchers" }

// MarshalJSON custom serialization for Voucher
func (v Voucher) MarshalJSON() ([]byte, error) {
	type Alias Voucher
	return json.Marshal(&struct {
		Description          *string          `json:"description"`
		DiscountAmountMoney  *decimal.Decimal `json:"discount_amount_money"`
		DiscountAmountPoints *int32           `json:"discount_amount_points"`
		DiscountPercent      *decimal.Decimal `json:"discount_percent"`
		MaxDiscountMoney     *decimal.Decimal `json:"max_discount_money"`
		MaxDiscountPoints    *int32           `json:"max_discount_points"`
		MinPurchaseMoney     *decimal.Decimal `json:"min_purchase_money"`
		MinPurchasePoints    *int32           `json:"min_purchase_points"`
		UsageLimit           *int32           `json:"usage_limit"`
		UsagePerUser         *int32           `json:"usage_per_user"`
		UsedCount            *int32           `json:"used_count"`
		CanStack             bool             `json:"can_stack"`
		StartDate            *time.Time       `json:"start_date"`
		EndDate              *time.Time       `json:"end_date"`
		IsActive             bool             `json:"is_active"`
		*Alias
	}{
		Description:          pgTextToStringPtr(v.Description),
		DiscountAmountMoney:  v.DiscountAmountMoney,
		DiscountAmountPoints: pgInt4ToInt32Ptr(v.DiscountAmountPoints),
		DiscountPercent:      v.DiscountPercent,
		MaxDiscountMoney:     v.MaxDiscountMoney,
		MaxDiscountPoints:    pgInt4ToInt32Ptr(v.MaxDiscountPoints),
		MinPurchaseMoney:     v.MinPurchaseMoney,
		UsageLimit:           pgInt4ToInt32Ptr(v.UsageLimit),
		UsagePerUser:         pgInt4ToInt32Ptr(v.UsagePerUser),
		UsedCount:            pgInt4ToInt32Ptr(v.UsedCount),
		CanStack:             v.CanStack.Bool,
		StartDate:            pgTimestampToTimePtr(v.StartDate),
		EndDate:              pgTimestampToTimePtr(v.EndDate),
		IsActive:             v.IsActive.Bool,
		Alias:                (*Alias)(&v),
	})
}

// Helper functions for pgtype conversion
func pgTextToStringPtr(t pgtype.Text) *string {
	if t.Valid {
		return &t.String
	}
	return nil
}

func pgInt4ToInt32Ptr(i pgtype.Int4) *int32 {
	if i.Valid {
		return &i.Int32
	}
	return nil
}

func pgTimestampToTimePtr(t pgtype.Timestamp) *time.Time {
	if t.Valid {
		return &t.Time
	}
	return nil
}

type VoucherApplicableType string

const (
	VATUser  VoucherApplicableType = "USER"
	VATRole  VoucherApplicableType = "ROLE"
	VATTier  VoucherApplicableType = "USER_TIER"
	VATGroup VoucherApplicableType = "GROUP"

	VATAll      VoucherApplicableType = "ALL"
	VATProduct  VoucherApplicableType = "PRODUCT"
	VATCategory VoucherApplicableType = "CATEGORY"
	VATService  VoucherApplicableType = "SERVICE"
	VATBranch   VoucherApplicableType = "BRANCH"
)

// 62. VoucherApplicability (Bảng áp dụng voucher)
type VoucherApplicability struct {
	ID             uuid.UUID             `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	VoucherID      uuid.UUID             `gorm:"type:uuid;not null;index" json:"voucher_id"`
	ApplicableType VoucherApplicableType `gorm:"type:varchar(20);not null" json:"applicable_type"`
	ApplicableID   uuid.UUID             `gorm:"type:uuid;not null" json:"applicable_id"`
	CreatedAt      time.Time             `gorm:"autoCreateTime" json:"created_at"`
}

func (VoucherApplicability) TableName() string {
	return "voucher_applicabilities"
}

// VoucherLogStatus constants
type VoucherLogStatus string

const (
	VoucherLogStatusUsed      VoucherLogStatus = "used"      // Đã sử dụng thành công
	VoucherLogStatusCancelled VoucherLogStatus = "cancelled" // Đã hủy (do hủy đơn)
)

// 51. VoucherLog (Bảng lịch sử voucher)
type VoucherLog struct {
	ID             uuid.UUID        `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	VoucherID      uuid.UUID        `gorm:"type:uuid;not null;index" json:"voucher_id"`
	UserID         uuid.UUID        `gorm:"type:uuid;not null;index" json:"user_id"`
	OrderID        uuid.UUID        `gorm:"type:uuid;not null;index" json:"order_id"`
	OrderType      pgtype.Text      `json:"order_type"`
	DiscountAmount *decimal.Decimal `gorm:"type:numeric" json:"discount_amount"`
	OriginalAmount *decimal.Decimal `gorm:"type:numeric" json:"original_amount"`
	FinalAmount    *decimal.Decimal `gorm:"type:numeric" json:"final_amount"`
	Status         string           `gorm:"type:varchar(20);not null;default:'used';index" json:"status"` // used, cancelled
	UsedAt         time.Time        `gorm:"autoCreateTime" json:"used_at"`
	CancelledAt    *time.Time       `json:"cancelled_at,omitempty"` // Thời điểm hủy (nếu có)
}

func (VoucherLog) TableName() string {
	return "voucher_logs"
}
