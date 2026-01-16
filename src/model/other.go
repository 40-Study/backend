package model

import (
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/shopspring/decimal"
	"gorm.io/gorm"
)

// Food Ordering Module

// ---------- FoodCategory ----------
type FoodCategory struct {
	ID          uuid.UUID   `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	Name        string      `gorm:"not null;uniqueIndex:uidx_food_categories_name" json:"name"`
	Description pgtype.Text `json:"description"`
	IsActive    bool        `gorm:"default:true;index" json:"is_active"`

	CreatedAt time.Time      `gorm:"autoCreateTime" json:"created_at"`
	UpdatedAt time.Time      `gorm:"autoUpdateTime" json:"updated_at"`
	DeletedAt gorm.DeletedAt `gorm:"index" json:"deleted_at"`

	Items []FoodItem `gorm:"foreignKey:FoodCategoryID;constraint:OnDelete:CASCADE" json:"items,omitempty"`
}

func (FoodCategory) TableName() string {
	return "food_categories"
}

// ---------- FoodItem ----------
type FoodItem struct {
	ID             uuid.UUID   `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	FoodCategoryID uuid.UUID   `gorm:"type:uuid;not null;uniqueIndex:uidx_food_items_category_name,priority:1" json:"food_category_id"`
	Name           string      `gorm:"not null;uniqueIndex:uidx_food_items_category_name,priority:2" json:"name"`
	Description    pgtype.Text `json:"description"`
	Price          int64       `gorm:"not null" json:"price"`
	Stock          *int32      `gorm:"default:null" json:"stock"` // NULL = unlimited, >= 0 = limited stock
	IsActive       bool        `gorm:"default:true;index" json:"is_active"`

	CreatedAt time.Time      `gorm:"autoCreateTime" json:"created_at"`
	UpdatedAt time.Time      `gorm:"autoUpdateTime" json:"updated_at"`
	DeletedAt gorm.DeletedAt `gorm:"index" json:"deleted_at"`

	Category FoodCategory    `gorm:"foreignKey:FoodCategoryID" json:"category,omitempty"`
	Images   []FoodItemImage `gorm:"foreignKey:FoodItemID;constraint:OnDelete:CASCADE" json:"images,omitempty"`
	Reviews  []FoodReview    `gorm:"foreignKey:FoodItemID;constraint:OnDelete:CASCADE" json:"reviews,omitempty"`
}

func (FoodItem) TableName() string {
	return "food_items"
}

// ---------- FoodItemImage (gallery ảnh của món) ----------
type FoodItemImage struct {
	ID         uuid.UUID   `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	FoodItemID uuid.UUID   `gorm:"type:uuid;not null;index:idx_food_item_images_item_order,priority:1" json:"food_item_id"`
	URL        pgtype.Text `gorm:"not null" json:"url"`
	SortOrder  int         `gorm:"not null;default:0;index:idx_food_item_images_item_order,priority:2" json:"sort_order"`
	IsCover    bool        `gorm:"default:false;index" json:"is_cover"`

	CreatedAt time.Time      `gorm:"autoCreateTime" json:"created_at"`
	DeletedAt gorm.DeletedAt `gorm:"index" json:"deleted_at"`
}

func (FoodItemImage) TableName() string {
	return "food_item_images"
}

// ---------- FoodReview ----------
type FoodReview struct {
	ID         uuid.UUID `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	UserID     uuid.UUID `gorm:"type:uuid;not null;index" json:"user_id"`
	FoodItemID uuid.UUID `gorm:"type:uuid;not null;index" json:"food_item_id"`

	Rating  int16       `gorm:"not null;check:rating >= 1 AND rating <= 5;index" json:"rating"`
	Comment pgtype.Text `json:"comment"`

	CreatedAt time.Time      `gorm:"autoCreateTime;index" json:"created_at"`
	UpdatedAt time.Time      `gorm:"autoUpdateTime" json:"updated_at"`
	DeletedAt gorm.DeletedAt `gorm:"index" json:"deleted_at"`

	Images []FoodReviewImage `gorm:"foreignKey:FoodReviewID;constraint:OnDelete:CASCADE" json:"images,omitempty"`
}

func (FoodReview) TableName() string {
	return "food_reviews"
}

// ---------- FoodReviewImage ----------
type FoodReviewImage struct {
	ID           uuid.UUID   `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	FoodReviewID uuid.UUID   `gorm:"type:uuid;not null;index:idx_food_review_images_review_order,priority:1" json:"food_review_id"`
	URL          pgtype.Text `gorm:"not null" json:"url"`
	SortOrder    int         `gorm:"not null;default:0;index:idx_food_review_images_review_order,priority:2" json:"sort_order"`

	CreatedAt time.Time      `gorm:"autoCreateTime" json:"created_at"`
	DeletedAt gorm.DeletedAt `gorm:"index" json:"deleted_at"`
}

func (FoodReviewImage) TableName() string {
	return "food_review_images"
}

// ---------- Order (Đơn hàng) ----------
type Order struct {
	ID          uuid.UUID `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	OrderNumber string    `gorm:"type:varchar(30);uniqueIndex;not null" json:"order_number"` // ORD-YYYYMMDD-XXXXX
	UserID      uuid.UUID `gorm:"type:uuid;not null;index:idx_orders_user_status,priority:1" json:"user_id"`
	OrderType   string    `gorm:"type:varchar(20);not null;index" json:"order_type"`

	// Pricing
	SubTotal       int64 `gorm:"not null" json:"sub_total"`                 // Tổng tiền trước giảm giá
	DiscountAmount int64 `gorm:"not null;default:0" json:"discount_amount"` // Tiền giảm từ voucher
	TotalAmount    int64 `gorm:"not null" json:"total_amount"`              // Tổng cuối = SubTotal - Discount

	// Voucher info (nếu dùng voucher)
	VoucherID     *uuid.UUID  `gorm:"type:uuid;index" json:"voucher_id"`
	VoucherCode   pgtype.Text `json:"voucher_code"`
	UserVoucherID *uuid.UUID  `gorm:"type:uuid;index" json:"user_voucher_id"` // Link đến user_vouchers

	// Payment
	PaymentMethod string           `gorm:"type:varchar(50);not null" json:"payment_method"`                         // cod, wallet, card, point
	PaymentStatus string           `gorm:"type:varchar(20);not null;default:'pending';index" json:"payment_status"` // pending, paid, failed, refunded
	PaidAt        pgtype.Timestamp `json:"paid_at"`
	TransactionID *uuid.UUID       `gorm:"type:uuid" json:"transaction_id"` // Link đến transactions table

	// Order status workflow
	Status string `gorm:"type:varchar(20);not null;default:'pending';index:idx_orders_user_status,priority:2" json:"status"`
	// pending → confirmed → completed / cancelled / refunded

	// Customer info (địa chỉ nhận hàng)
	// DEPRECATED: Dùng AddressID thay thế, giữ lại để backward compatible
	CustomerName    pgtype.Text `json:"customer_name"`
	CustomerPhone   pgtype.Text `json:"customer_phone"`
	CustomerAddress pgtype.Text `json:"customer_address"`

	// Địa chỉ giao hàng (structured address)
	AddressID *uuid.UUID `gorm:"type:uuid;index" json:"address_id"` // Link đến bảng addresses

	// Branch/Zone info (đơn thuộc chi nhánh/khu vực nào)
	BranchID *uuid.UUID `gorm:"type:uuid;index" json:"branch_id"`
	ZoneID   *uuid.UUID `gorm:"type:uuid" json:"zone_id"`

	// Cancellation
	CancelledByUserID  *uuid.UUID  `gorm:"type:uuid" json:"cancelled_by_user_id"`
	CancellationReason pgtype.Text `json:"cancellation_reason"`

	// Timestamps
	CompletedAt pgtype.Timestamp `json:"completed_at"`
	CancelledAt pgtype.Timestamp `json:"cancelled_at"`

	// Metadata
	Notes    pgtype.Text            `json:"notes"`
	Metadata map[string]interface{} `gorm:"type:jsonb;serializer:json" json:"metadata"`

	CreatedAt time.Time      `gorm:"autoCreateTime;index" json:"created_at"`
	UpdatedAt time.Time      `gorm:"autoUpdateTime" json:"updated_at"`
	DeletedAt gorm.DeletedAt `gorm:"index" json:"deleted_at"`

	// Relations
	Items   []OrderItem `gorm:"foreignKey:OrderID;constraint:OnDelete:CASCADE" json:"items,omitempty"`
	User    *User       `gorm:"foreignKey:UserID" json:"user,omitempty"`
	Address *Address    `gorm:"foreignKey:AddressID" json:"address,omitempty"` // Địa chỉ giao hàng
}

func (Order) TableName() string {
	return "orders"
}

// Order status constants
const (
	OrderStatusPending   = "pending"
	OrderStatusConfirmed = "confirmed"
	OrderStatusCompleted = "completed"
	OrderStatusCancelled = "cancelled"
	OrderStatusRefunded  = "refunded"
)

const (
	OrderTypeFood       = "food"
	OrderTypeGameRental = "game_rental"
	OrderTypeVIP        = "vip"
	OrderTypeReward     = "reward"
	OrderTypeAccount    = "account"
)

const (
	PaymentStatusPending  = "pending"
	PaymentStatusPaid     = "paid"
	PaymentStatusFailed   = "failed"
	PaymentStatusRefunded = "refunded"
)

// ---------- OrderItem (Chi tiết đơn hàng) ----------
type OrderItem struct {
	ID      uuid.UUID `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	OrderID uuid.UUID `gorm:"type:uuid;not null;index" json:"order_id"`

	// Item reference (polymorphic)
	ItemID   uuid.UUID `gorm:"type:uuid;not null;index" json:"item_id"`    // FoodItem.ID, Account.ID, etc.
	ItemType string    `gorm:"type:varchar(50);not null" json:"item_type"` // food, game_account, vip_package, reward

	ItemName        string      `gorm:"type:varchar(255);not null" json:"item_name"` // Snapshot tên lúc đặt
	ItemDescription pgtype.Text `json:"item_description"`                            // Snapshot mô tả

	Quantity  int32 `gorm:"not null;default:1" json:"quantity"`
	UnitPrice int64 `gorm:"not null" json:"unit_price"` // Giá đơn vị lúc đặt
	SubTotal  int64 `gorm:"not null" json:"sub_total"`  // = Quantity * UnitPrice

	// Custom options (cho food: size, toppings, etc.)
	Options map[string]interface{} `gorm:"type:jsonb;serializer:json" json:"options"`

	// Notes cho item này
	Notes pgtype.Text `json:"notes"`

	CreatedAt time.Time      `gorm:"autoCreateTime" json:"created_at"`
	DeletedAt gorm.DeletedAt `gorm:"index" json:"deleted_at"`
}

func (OrderItem) TableName() string {
	return "order_items"
}

// Item type constants
const (
	OrderItemTypeFood        = "food"
	OrderItemTypeGameAccount = "game_account"
	OrderItemTypeVIPPackage  = "vip_package"
	OrderItemTypeReward      = "reward"
)

// Account Thuê Game Module
type Account struct {
	ID        uuid.UUID      `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	Username  string         `gorm:"not null" json:"username"`
	Password  string         `gorm:"not null" json:"-"`
	Status    pgtype.Text    `gorm:"default:'available'" json:"status"`
	CreatedAt time.Time      `gorm:"autoCreateTime" json:"created_at"`
	UpdatedAt time.Time      `gorm:"autoUpdateTime" json:"updated_at"`
	DeletedAt gorm.DeletedAt `gorm:"index" json:"deleted_at"`
}

func (Account) TableName() string {
	return "accounts"
}

// Points/Loyalty Module

type PointReward struct {
	ID          uuid.UUID      `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	Name        string         `gorm:"not null" json:"name"`
	Description pgtype.Text    `json:"description"`
	PointsCost  int32          `gorm:"not null" json:"points_cost"`
	CreatedAt   time.Time      `gorm:"autoCreateTime" json:"created_at"`
	UpdatedAt   time.Time      `gorm:"autoUpdateTime" json:"updated_at"`
	DeletedAt   gorm.DeletedAt `gorm:"index" json:"deleted_at"`
}

func (PointReward) TableName() string {
	return "point_rewards"
}

// Commerce/Payment Module

// Transaction types
const (
	TransactionTypeTopup    = "topup"
	TransactionTypeWithdraw = "withdraw"
	TransactionTypePayment  = "payment"
	TransactionTypeRefund   = "refund"
	TransactionTypeBonus    = "bonus"
	TransactionTypePenalty  = "penalty"
)

// Transaction statuses
const (
	TransactionStatusPending   = "pending"
	TransactionStatusCompleted = "completed"
	TransactionStatusFailed    = "failed"
	TransactionStatusCancelled = "cancelled"
)

type Wallet struct {
	ID           uuid.UUID        `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	UserID       uuid.UUID        `gorm:"type:uuid;unique;not null;index" json:"user_id"`
	Balance      *decimal.Decimal `gorm:"type:numeric;default:0;check:balance >= 0" json:"balance"` // Không được âm
	Currency     string           `gorm:"default:'VND'" json:"currency"`
	IsLocked     bool             `gorm:"default:false;index" json:"is_locked"` // Khóa ví khi phát hiện gian lận
	LockedReason pgtype.Text      `json:"locked_reason,omitempty"`
	LockedAt     pgtype.Timestamp `json:"locked_at,omitempty"`
	CreatedAt    time.Time        `gorm:"autoCreateTime" json:"created_at"`
	UpdatedAt    time.Time        `gorm:"autoUpdateTime" json:"updated_at"`
	DeletedAt    gorm.DeletedAt   `gorm:"index" json:"deleted_at"`
}

func (Wallet) TableName() string {
	return "wallets"
}

type Transaction struct {
	ID       uuid.UUID `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	WalletID uuid.UUID `gorm:"type:uuid;not null;index:idx_transactions_wallet_id" json:"wallet_id"`

	// Transaction info
	Amount *decimal.Decimal `gorm:"type:numeric;not null;check:amount > 0" json:"amount"`
	Type   string           `gorm:"type:varchar(20);not null;index:idx_transactions_type" json:"type"`                       // topup, withdraw, payment, refund, bonus, penalty
	Status string           `gorm:"type:varchar(20);not null;default:'pending';index:idx_transactions_status" json:"status"` // pending, completed, failed, cancelled

	// Balance tracking (quan trọng cho audit)
	BalanceBefore *decimal.Decimal `gorm:"type:numeric;not null" json:"balance_before"`
	BalanceAfter  *decimal.Decimal `gorm:"type:numeric;not null" json:"balance_after"`

	// Related references
	OrderID       *uuid.UUID  `gorm:"type:uuid;index:idx_transactions_order_id" json:"order_id,omitempty"`
	ReferenceType pgtype.Text `gorm:"type:varchar(50)" json:"reference_type,omitempty"` // order, topup_request, withdraw_request
	ReferenceID   *uuid.UUID  `gorm:"type:uuid" json:"reference_id,omitempty"`

	// Payment gateway info (cho topup/withdraw)
	PaymentMethod          pgtype.Text            `gorm:"type:varchar(50)" json:"payment_method,omitempty"`                                          // momo, vnpay, banking, admin
	PaymentGatewayRef      pgtype.Text            `gorm:"type:varchar(255);index:idx_transactions_gateway_ref" json:"payment_gateway_ref,omitempty"` // Transaction ID từ gateway
	PaymentGatewayResponse map[string]interface{} `gorm:"type:jsonb;serializer:json" json:"payment_gateway_response,omitempty"`                      // Response từ gateway

	// Security & audit
	IPAddress   pgtype.Text `gorm:"type:varchar(45)" json:"ip_address,omitempty"`
	UserAgent   pgtype.Text `json:"user_agent,omitempty"`
	Description pgtype.Text `json:"description,omitempty"`
	Notes       pgtype.Text `json:"notes,omitempty"` // Admin notes

	// Additional metadata
	Metadata map[string]interface{} `gorm:"type:jsonb;serializer:json" json:"metadata,omitempty"`

	// Timestamps
	CompletedAt pgtype.Timestamp `json:"completed_at,omitempty"`
	FailedAt    pgtype.Timestamp `json:"failed_at,omitempty"`
	CancelledAt pgtype.Timestamp `json:"cancelled_at,omitempty"`
	CreatedAt   time.Time        `gorm:"autoCreateTime;index:idx_transactions_created_at" json:"created_at"`
	UpdatedAt   time.Time        `gorm:"autoUpdateTime" json:"updated_at"`
	DeletedAt   gorm.DeletedAt   `gorm:"index" json:"deleted_at"`
}

func (Transaction) TableName() string {
	return "transactions"
}

type Payment struct {
	ID uuid.UUID `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`

	// Owner
	UserID  uuid.UUID  `gorm:"type:uuid;not null;index:idx_payments_user_id" json:"user_id"`
	OrderID *uuid.UUID `gorm:"type:uuid;index:idx_payments_order_id" json:"order_id,omitempty"`

	Amount decimal.Decimal `gorm:"type:numeric(18,2);not null;check:amount > 0" json:"amount"`
	Method string          `gorm:"type:varchar(30);not null;index:idx_payments_method" json:"method"`

	Status string `gorm:"type:varchar(20);not null;default:'pending';index:idx_payments_status" json:"status"`
	GatewayRef      pgtype.Text            `gorm:"type:varchar(255);index:idx_payments_gateway_ref" json:"gateway_ref,omitempty"`
	GatewayResponse map[string]interface{} `gorm:"type:jsonb;serializer:json" json:"gateway_response,omitempty"`

	PaidAt     *time.Time      `json:"paid_at,omitempty"`
	CreatedAt  time.Time       `gorm:"autoCreateTime" json:"created_at"`
	UpdatedAt  time.Time       `gorm:"autoUpdateTime" json:"updated_at"`
	DeletedAt  gorm.DeletedAt  `gorm:"index" json:"deleted_at,omitempty"`
}

func (Payment) TableName() string {
	return "payments"
}


type TransactionLock struct {
	ID            uuid.UUID `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	WalletID      uuid.UUID `gorm:"type:uuid;not null;index:idx_transaction_locks_wallet_id" json:"wallet_id"`
	TransactionID uuid.UUID `gorm:"type:uuid;not null" json:"transaction_id"`
	LockKey       string    `gorm:"type:varchar(255);not null;uniqueIndex:idx_transaction_locks_lock_key" json:"lock_key"` // Idempotency key
	ExpiresAt     time.Time `gorm:"not null" json:"expires_at"`
	CreatedAt     time.Time `gorm:"autoCreateTime" json:"created_at"`
}

func (TransactionLock) TableName() string {
	return "transaction_locks"
}

// WalletAudit - Bảng tracking mọi thay đổi của wallet
type WalletAudit struct {
	ID            uuid.UUID  `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	WalletID      uuid.UUID  `gorm:"type:uuid;not null;index:idx_wallet_audit_logs_wallet_id" json:"wallet_id"`
	TransactionID *uuid.UUID `gorm:"type:uuid" json:"transaction_id,omitempty"`

	Action       string           `gorm:"type:varchar(50);not null" json:"action"` // balance_change, lock_wallet, unlock_wallet
	OldBalance   *decimal.Decimal `gorm:"type:numeric" json:"old_balance,omitempty"`
	NewBalance   *decimal.Decimal `gorm:"type:numeric" json:"new_balance,omitempty"`
	ChangeAmount *decimal.Decimal `gorm:"type:numeric" json:"change_amount,omitempty"`

	PerformedBy *uuid.UUID             `gorm:"type:uuid" json:"performed_by,omitempty"` // User hoặc Admin
	IPAddress   pgtype.Text            `gorm:"type:varchar(45)" json:"ip_address,omitempty"`
	Metadata    map[string]interface{} `gorm:"type:jsonb;serializer:json" json:"metadata,omitempty"`

	CreatedAt time.Time `gorm:"autoCreateTime;index:idx_wallet_audit_logs_created_at" json:"created_at"`
}

func (WalletAudit) TableName() string {
	return "wallet_audit_logs"
}

// System/Ops Module

type Setting struct {
	ID        uuid.UUID      `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	Key       string         `gorm:"unique;not null" json:"key"`
	Value     string         `gorm:"not null" json:"value"`
	Type      string         `gorm:"default:'string'" json:"type"`
	CreatedAt time.Time      `gorm:"autoCreateTime" json:"created_at"`
	UpdatedAt time.Time      `gorm:"autoUpdateTime" json:"updated_at"`
	DeletedAt gorm.DeletedAt `gorm:"index" json:"deleted_at"`
}

func (Setting) TableName() string {
	return "settings"
}

type SystemLog struct {
	ID        uuid.UUID              `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	Level     string                 `gorm:"not null" json:"level"`
	Message   string                 `gorm:"not null" json:"message"`
	Source    pgtype.Text            `json:"source"`
	Metadata  map[string]interface{} `gorm:"type:jsonb;serializer:json" json:"metadata"`
	CreatedAt time.Time              `gorm:"autoCreateTime" json:"created_at"`
}

func (SystemLog) TableName() string {
	return "system_logs"
}

// Content/Livestream/MiniGame Module

type ContentPage struct {
	ID          uuid.UUID      `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	Title       string         `gorm:"not null" json:"title"`
	Slug        string         `gorm:"unique;not null" json:"slug"`
	Body        pgtype.Text    `json:"body"`
	IsPublished pgtype.Bool    `gorm:"default:false" json:"is_published"`
	CreatedAt   time.Time      `gorm:"autoCreateTime" json:"created_at"`
	UpdatedAt   time.Time      `gorm:"autoUpdateTime" json:"updated_at"`
	DeletedAt   gorm.DeletedAt `gorm:"index" json:"deleted_at"`
}

func (ContentPage) TableName() string {
	return "content_pages"
}

type Livestream struct {
	ID        uuid.UUID        `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	Title     string           `gorm:"not null" json:"title"`
	StreamUrl string           `gorm:"not null" json:"stream_url"`
	StartTime time.Time        `gorm:"not null" json:"start_time"`
	EndTime   pgtype.Timestamp `json:"end_time"`
	Status    string           `gorm:"default:'scheduled'" json:"status"`
	CreatedAt time.Time        `gorm:"autoCreateTime" json:"created_at"`
	UpdatedAt time.Time        `gorm:"autoUpdateTime" json:"updated_at"`
	DeletedAt gorm.DeletedAt   `gorm:"index" json:"deleted_at"`
}

func (Livestream) TableName() string {
	return "livestreams"
}

type MiniGame struct {
	ID          uuid.UUID              `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	Name        string                 `gorm:"not null" json:"name"`
	Description pgtype.Text            `json:"description"`
	Rules       map[string]interface{} `gorm:"type:jsonb;serializer:json" json:"rules"`
	IsActive    pgtype.Bool            `gorm:"default:true" json:"is_active"`
	CreatedAt   time.Time              `gorm:"autoCreateTime" json:"created_at"`
	UpdatedAt   time.Time              `gorm:"autoUpdateTime" json:"updated_at"`
	DeletedAt   gorm.DeletedAt         `gorm:"index" json:"deleted_at"`
}

func (MiniGame) TableName() string {
	return "mini_games"
}
