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
	CreatedAt            time.Time       `gorm:"autoCreateTime" json:"created_at"`
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
	PaymentCodeCreatedAt *time.Time      `json:"payment_code_created_at,omitempty"`
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
	BaseModel
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
// OrderStatusHistory - Track status changes for orders
type InstructorPayout struct {
	BaseModel
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

// ============================================================
// VOUCHER MODELS
// ============================================================

// DiscountUnit - Unit for discount (MONEY or POINT)
type DiscountUnit string

const (
	DiscountUnitMoney  DiscountUnit = "MONEY"
	DiscountUnitPoint DiscountUnit = "POINT"
)

// DiscountMethod - Method for discount (FIXED or PERCENT)
type DiscountMethod string

const (
	DiscountMethodFixed   DiscountMethod = "FIXED"
	DiscountMethodPercent DiscountMethod = "PERCENT"
)

// VoucherApplicableType - Type of entity the voucher applies to
type VoucherApplicableType string

const (
	VoucherApplicableTypeUser     VoucherApplicableType = "USER"
	VoucherApplicableTypeRole     VoucherApplicableType = "ROLE"
	VoucherApplicableTypeTier     VoucherApplicableType = "TIER"
	VoucherApplicableTypeGroup    VoucherApplicableType = "GROUP"
	VoucherApplicableTypeProduct  VoucherApplicableType = "PRODUCT"
	VoucherApplicableTypeCategory VoucherApplicableType = "CATEGORY"
	VoucherApplicableTypeAll      VoucherApplicableType = "ALL"
	VoucherApplicableTypeService  VoucherApplicableType = "SERVICE"
	VoucherApplicableTypeBranch  VoucherApplicableType = "BRANCH"
)

// Voucher - Main voucher model
type Voucher struct {
	ID          uuid.UUID       `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	Code        string          `gorm:"type:varchar(50);uniqueIndex;not null" json:"code"`
	Name        string          `gorm:"type:varchar(255);not null" json:"name"`
	Description string          `gorm:"type:text" json:"description"`

	// Discount configuration
	DiscountUnit   DiscountUnit        `gorm:"type:varchar(20);not null" json:"discount_unit"`
	DiscountMethod DiscountMethod       `gorm:"type:varchar(20);not null" json:"discount_method"`

	// Discount values - depending on unit and method
	DiscountAmountMoney  *decimal.Decimal `gorm:"type:decimal(12,2)" json:"discount_amount_money,omitempty"`  // MONEY + FIXED
	DiscountAmountPoints int32            `gorm:"type:int" json:"discount_amount_points,omitempty"`           // POINT + FIXED
	DiscountPercent     *decimal.Decimal `gorm:"type:decimal(5,2)" json:"discount_percent,omitempty"`       // PERCENT

	// Max discount caps
	MaxDiscountMoney  *decimal.Decimal `gorm:"type:decimal(12,2)" json:"max_discount_money,omitempty"`  // PERCENT + MONEY
	MaxDiscountPoints int32            `gorm:"type:int" json:"max_discount_points,omitempty"`         // PERCENT + POINT

	// Minimum purchase requirements
	MinPurchaseMoney  *decimal.Decimal `gorm:"type:decimal(12,2)" json:"min_purchase_money,omitempty"`
	MinPurchasePoints int32            `gorm:"type:int" json:"min_purchase_points,omitempty"`

	// Payment method restrictions
	AcceptAllPaymentMethods bool     `gorm:"type:bool;default:true" json:"accept_all_payment_methods"`
	PaymentMethodsAccepted  []string `gorm:"type:text[]" json:"payment_methods_accepted"`

	// Usage limits
	UsedCount   int32 `gorm:"type:int;default:0" json:"used_count"`
	UsageLimit  int32 `gorm:"type:int" json:"usage_limit"`
	UsagePerUser int32 `gorm:"type:int;default:1" json:"usage_per_user"`

	// Stacking
	CanStack bool `gorm:"type:bool;default:false" json:"can_stack"`

	// Date range
	StartDate *time.Time `gorm:"type:timestamp" json:"start_date,omitempty"`
	EndDate   *time.Time `gorm:"type:timestamp" json:"end_date,omitempty"`

	// Status
	IsActive bool `gorm:"type:bool;default:true" json:"is_active"`

	// Soft delete
	DeletedAt gorm.DeletedAt `gorm:"index" json:"-"`

	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`

	// Relationships
	Applicabilities []VoucherApplicability `gorm:"foreignKey:VoucherID;constraint:OnDelete:CASCADE" json:"-"`
	Logs           []VoucherLog            `gorm:"foreignKey:VoucherID;constraint:OnDelete:CASCADE" json:"-"`
}

func (Voucher) TableName() string {
	return "vouchers"
}

// UserVoucher - User's saved/bookmarked voucher
type UserVoucher struct {
	ID         uuid.UUID  `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	UserID     uuid.UUID  `gorm:"type:uuid;not null;index" json:"user_id"`
	VoucherID  uuid.UUID  `gorm:"type:uuid;not null;index" json:"voucher_id"`
	Source     string     `gorm:"type:varchar(50)" json:"source"` // manual, admin_grant, event_reward
	SavedAt    time.Time  `gorm:"type:timestamp;not null" json:"saved_at"`
	Notes      string     `gorm:"type:text" json:"notes"`

	DeletedAt gorm.DeletedAt `gorm:"index" json:"-"`

	// Relationships
	User    User    `gorm:"foreignKey:UserID;constraint:OnDelete:CASCADE" json:"-"`
	Voucher Voucher `gorm:"foreignKey:VoucherID;constraint:OnDelete:CASCADE" json:"-"`
}

func (UserVoucher) TableName() string {
	return "user_vouchers"
}

// VoucherApplicability - Rules for where voucher can be applied
type VoucherApplicability struct {
	ID             uuid.UUID            `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	VoucherID      uuid.UUID            `gorm:"type:uuid;not null;index" json:"voucher_id"`
	ApplicableType VoucherApplicableType `gorm:"type:varchar(20);not null" json:"applicable_type"`
	ApplicableID   uuid.UUID            `gorm:"type:uuid;not null" json:"applicable_id"`

	CreatedAt time.Time `json:"created_at"`

	// Relationships
	Voucher Voucher `gorm:"foreignKey:VoucherID;constraint:OnDelete:CASCADE" json:"-"`
}

func (VoucherApplicability) TableName() string {
	return "voucher_applicabilities"
}

// VoucherLog - Audit trail for voucher usage
type VoucherLog struct {
	ID         uuid.UUID `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	UserID     uuid.UUID `gorm:"type:uuid;not null;index" json:"user_id"`
	VoucherID  uuid.UUID `gorm:"type:uuid;not null;index" json:"voucher_id"`
	VoucherCode string   `gorm:"type:varchar(50);not null" json:"voucher_code"`
	OrderID    uuid.UUID `gorm:"type:uuid;not null;index" json:"order_id"`
	OrderType  string   `gorm:"type:varchar(50)" json:"order_type"`
	Action     string   `gorm:"type:varchar(20);not null" json:"action"` // used, refunded
	Amount     int64    `gorm:"type:bigint;not null" json:"amount"`      // Discount amount

	CreatedAt time.Time              `json:"created_at"`
	Metadata  map[string]interface{} `gorm:"type:jsonb" json:"metadata,omitempty"`

	// Relationships
	User    User    `gorm:"foreignKey:UserID;constraint:OnDelete:CASCADE" json:"-"`
	Voucher Voucher `gorm:"foreignKey:VoucherID;constraint:OnDelete:CASCADE" json:"-"`
}

func (VoucherLog) TableName() string {
	return "voucher_logs"
}
