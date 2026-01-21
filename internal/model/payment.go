package model

import (
	"time"

	"github.com/google/uuid"
	"github.com/lib/pq"
	"github.com/shopspring/decimal"
	"gorm.io/gorm"
)

type Order struct {
	ID                   uuid.UUID       `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	UserID               uuid.UUID       `gorm:"type:uuid;not null;index" json:"user_id"`
	OrderNumber          string          `gorm:"type:varchar(50);uniqueIndex;not null" json:"order_number"`
	Subtotal             decimal.Decimal `gorm:"type:decimal(12,2);not null" json:"subtotal"`
	DiscountAmount       decimal.Decimal `gorm:"type:decimal(12,2);default:0" json:"discount_amount"`
	TaxAmount            decimal.Decimal `gorm:"type:decimal(12,2);default:0" json:"tax_amount"`
	TotalAmount          decimal.Decimal `gorm:"type:decimal(12,2);not null" json:"total_amount"`
	Currency             string          `gorm:"type:varchar(3);default:'VND'" json:"currency"`
	Status               string          `gorm:"type:varchar(20);default:'pending';check:status IN ('pending', 'processing', 'completed', 'failed', 'refunded', 'cancelled');index" json:"status"`
	PaymentMethod        *string         `gorm:"type:varchar(30)" json:"payment_method,omitempty"`
	PaymentGateway       *string         `gorm:"type:varchar(30)" json:"payment_gateway,omitempty"`
	PaymentTransactionID *string         `gorm:"type:varchar(255)" json:"payment_transaction_id,omitempty"`
	PaidAt               *time.Time      `json:"paid_at,omitempty"`
	CouponID             *uuid.UUID      `gorm:"type:uuid" json:"coupon_id,omitempty"`
	Notes                *string         `gorm:"type:text" json:"notes,omitempty"`

	// Relationships
	User        User         `gorm:"foreignKey:UserID" json:"-"`
	Coupon      *Coupon      `gorm:"foreignKey:CouponID" json:"-"`
	Items       []OrderItem  `gorm:"foreignKey:OrderID;constraint:OnDelete:CASCADE" json:"-"`
	CouponUsage *CouponUsage `gorm:"foreignKey:OrderID" json:"-"`
}

func (Order) TableName() string {
	return "orders"
}

type OrderItem struct {
	ID             uuid.UUID       `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	CreatedAt      time.Time       `json:"created_at"`
	OrderID        uuid.UUID       `gorm:"type:uuid;not null;index" json:"order_id"`
	CourseID       uuid.UUID       `gorm:"type:uuid;not null" json:"course_id"`
	Price          decimal.Decimal `gorm:"type:decimal(12,2);not null" json:"price"`
	DiscountAmount decimal.Decimal `gorm:"type:decimal(12,2);default:0" json:"discount_amount"`
	FinalPrice     decimal.Decimal `gorm:"type:decimal(12,2);not null" json:"final_price"`

	// Relationships
	Order  Order  `gorm:"foreignKey:OrderID;constraint:OnDelete:CASCADE" json:"-"`
	Course Course `gorm:"foreignKey:CourseID" json:"-"`
}

func (OrderItem) TableName() string {
	return "order_items"
}

type Coupon struct {
	gorm.Model
	ID                  uuid.UUID        `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	Code                string           `gorm:"type:varchar(50);uniqueIndex;not null" json:"code"`
	Description         *string          `gorm:"type:text" json:"description,omitempty"`
	DiscountType        string           `gorm:"type:varchar(20);not null;check:discount_type IN ('percentage', 'fixed_amount')" json:"discount_type"`
	DiscountValue       decimal.Decimal  `gorm:"type:decimal(12,2);not null" json:"discount_value"`
	MinPurchaseAmount   *decimal.Decimal `gorm:"type:decimal(12,2)" json:"min_purchase_amount,omitempty"`
	MaxDiscountAmount   *decimal.Decimal `gorm:"type:decimal(12,2)" json:"max_discount_amount,omitempty"`
	UsageLimit          *int             `json:"usage_limit,omitempty"`
	UsageCount          int              `gorm:"default:0" json:"usage_count"`
	PerUserLimit        int              `gorm:"default:1" json:"per_user_limit"`
	ApplicableCourseIDs pq.StringArray   `gorm:"type:uuid[]" json:"applicable_course_ids"`
	StartsAt            *time.Time       `json:"starts_at,omitempty"`
	ExpiresAt           *time.Time       `json:"expires_at,omitempty"`
	IsActive            bool             `gorm:"default:true" json:"is_active"`

	// Relationships
	Usages []CouponUsage `gorm:"foreignKey:CouponID;constraint:OnDelete:CASCADE" json:"-"`
}

func (Coupon) TableName() string {
	return "coupons"
}

type CouponUsage struct {
	ID             uuid.UUID       `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	CreatedAt      time.Time       `json:"created_at"`
	CouponID       uuid.UUID       `gorm:"type:uuid;not null;index" json:"coupon_id"`
	UserID         uuid.UUID       `gorm:"type:uuid;not null;index" json:"user_id"`
	OrderID        uuid.UUID       `gorm:"type:uuid;not null;index" json:"order_id"`
	DiscountAmount decimal.Decimal `gorm:"type:decimal(12,2);not null" json:"discount_amount"`

	// Relationships
	Coupon Coupon `gorm:"foreignKey:CouponID;constraint:OnDelete:CASCADE" json:"-"`
	User   User   `gorm:"foreignKey:UserID;constraint:OnDelete:CASCADE" json:"-"`
	Order  Order  `gorm:"foreignKey:OrderID;constraint:OnDelete:CASCADE" json:"-"`
}

func (CouponUsage) TableName() string {
	return "coupon_usages"
}

type InstructorPayout struct {
	gorm.Model
	ID                uuid.UUID       `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	InstructorID      uuid.UUID       `gorm:"type:uuid;not null;index" json:"instructor_id"`
	Amount            decimal.Decimal `gorm:"type:decimal(12,2);not null" json:"amount"`
	Currency          string          `gorm:"type:varchar(3);default:'VND'" json:"currency"`
	Status            string          `gorm:"type:varchar(20);default:'pending';check:status IN ('pending', 'processing', 'completed', 'failed')" json:"status"`
	PaymentMethod     *string         `gorm:"type:varchar(50)" json:"payment_method,omitempty"`
	BankName          *string         `gorm:"type:varchar(100)" json:"bank_name,omitempty"`
	BankAccountNumber *string         `gorm:"type:varchar(50)" json:"bank_account_number,omitempty"`
	BankAccountName   *string         `gorm:"type:varchar(255)" json:"bank_account_name,omitempty"`
	TransactionID     *string         `gorm:"type:varchar(255)" json:"transaction_id,omitempty"`
	PeriodStart       *time.Time      `gorm:"type:date" json:"period_start,omitempty"`
	PeriodEnd         *time.Time      `gorm:"type:date" json:"period_end,omitempty"`
	ProcessedAt       *time.Time      `json:"processed_at,omitempty"`
	Notes             *string         `gorm:"type:text" json:"notes,omitempty"`

	// Relationships
	Instructor User `gorm:"foreignKey:InstructorID" json:"-"`
}

func (InstructorPayout) TableName() string {
	return "instructor_payouts"
}
