package model

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/datatypes"
)

// ============================================================================
// COIN TRANSACTION TYPES
// ============================================================================

type CoinTransactionType string

const (
	// Earn types
	CoinTxEarnLessonComplete CoinTransactionType = "EARN_LESSON_COMPLETE"
	CoinTxEarnQuizPass       CoinTransactionType = "EARN_QUIZ_PASS"
	CoinTxEarnStreakBonus    CoinTransactionType = "EARN_STREAK_BONUS"
	CoinTxEarnAchievement    CoinTransactionType = "EARN_ACHIEVEMENT"
	CoinTxEarnDailyLogin     CoinTransactionType = "EARN_DAILY_LOGIN"
	CoinTxEarnReferral       CoinTransactionType = "EARN_REFERRAL"
	CoinTxEarnPurchase       CoinTransactionType = "EARN_PURCHASE"
	// Spend types
	CoinTxSpendCourseUnlock CoinTransactionType = "SPEND_COURSE_UNLOCK"
	CoinTxSpendHint         CoinTransactionType = "SPEND_HINT"
	CoinTxSpendStreakFreeze CoinTransactionType = "SPEND_STREAK_FREEZE"
	CoinTxSpendGift         CoinTransactionType = "SPEND_GIFT"
	// Other types
	CoinTxReceiveGift CoinTransactionType = "RECEIVE_GIFT"
	CoinTxRefund      CoinTransactionType = "REFUND"
	CoinTxAdminAdjust CoinTransactionType = "ADMIN_ADJUST"
)

type CoinPurchaseStatus string

const (
	CoinPurchasePending   CoinPurchaseStatus = "PENDING"
	CoinPurchaseCompleted CoinPurchaseStatus = "COMPLETED"
	CoinPurchaseFailed    CoinPurchaseStatus = "FAILED"
	CoinPurchaseRefunded  CoinPurchaseStatus = "REFUNDED"
)

// ============================================================================
// USER COIN WALLET
// ============================================================================

// UserCoinWallet stores user's coin balance
type UserCoinWallet struct {
	ID          uuid.UUID `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
	UserID      uuid.UUID `gorm:"type:uuid;uniqueIndex;not null" json:"user_id"`
	Balance     int64     `gorm:"not null;default:0" json:"balance"`
	TotalEarned int64     `gorm:"not null;default:0" json:"total_earned"`
	TotalSpent  int64     `gorm:"not null;default:0" json:"total_spent"`

	// Relationships
	User         User              `gorm:"foreignKey:UserID;constraint:OnDelete:CASCADE" json:"-"`
	Transactions []CoinTransaction `gorm:"foreignKey:WalletID" json:"-"`
}

func (UserCoinWallet) TableName() string {
	return "user_coin_wallets"
}

// ============================================================================
// COIN TRANSACTION
// ============================================================================

// CoinTransaction records coin earn/spend history
type CoinTransaction struct {
	ID            uuid.UUID           `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	CreatedAt     time.Time           `json:"created_at"`
	WalletID      uuid.UUID           `gorm:"type:uuid;not null;index" json:"wallet_id"`
	UserID        uuid.UUID           `gorm:"type:uuid;not null;index" json:"user_id"`
	Type          CoinTransactionType `gorm:"type:varchar(30);not null" json:"type"`
	Amount        int64               `gorm:"not null" json:"amount"`
	BalanceAfter  int64               `gorm:"not null" json:"balance_after"`
	ReferenceType *string             `gorm:"type:varchar(50)" json:"reference_type,omitempty"`
	ReferenceID   *uuid.UUID          `gorm:"type:uuid" json:"reference_id,omitempty"`
	Description   *string             `gorm:"type:text" json:"description,omitempty"`
	Metadata      datatypes.JSON      `gorm:"type:jsonb;default:'{}'" json:"metadata"`

	// Relationships
	User   User           `gorm:"foreignKey:UserID;constraint:OnDelete:CASCADE" json:"-"`
	Wallet UserCoinWallet `gorm:"foreignKey:WalletID;constraint:OnDelete:CASCADE" json:"-"`
}

func (CoinTransaction) TableName() string {
	return "coin_transactions"
}

// ============================================================================
// COIN PACKAGE
// ============================================================================

// CoinPackage defines purchasable coin packages
type CoinPackage struct {
	BaseModel
	Name            string  `gorm:"type:varchar(100);not null" json:"name"`
	Description     *string `gorm:"type:text" json:"description,omitempty"`
	CoinAmount      int64   `gorm:"not null" json:"coin_amount"`
	BonusAmount     int64   `gorm:"default:0" json:"bonus_amount"`
	Price           float64 `gorm:"type:decimal(12,2);not null" json:"price"`
	Currency        string  `gorm:"type:varchar(3);default:'VND'" json:"currency"`
	DiscountPercent int     `gorm:"default:0" json:"discount_percent"`
	IsFeatured      bool    `gorm:"default:false" json:"is_featured"`
	IsActive        bool    `gorm:"default:true" json:"is_active"`
	SortOrder       int     `gorm:"default:0" json:"sort_order"`
}

func (CoinPackage) TableName() string {
	return "coin_packages"
}

// ============================================================================
// COIN PURCHASE
// ============================================================================

// CoinPurchase records coin purchase history
type CoinPurchase struct {
	ID               uuid.UUID          `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	CreatedAt        time.Time          `json:"created_at"`
	UpdatedAt        time.Time          `json:"updated_at"`
	UserID           uuid.UUID          `gorm:"type:uuid;not null;index" json:"user_id"`
	PackageID        *uuid.UUID         `gorm:"type:uuid" json:"package_id,omitempty"`
	CoinAmount       int64              `gorm:"not null" json:"coin_amount"`
	BonusAmount      int64              `gorm:"default:0" json:"bonus_amount"`
	Price            float64            `gorm:"type:decimal(12,2);not null" json:"price"`
	Currency         string             `gorm:"type:varchar(3);default:'VND'" json:"currency"`
	Status           CoinPurchaseStatus `gorm:"type:varchar(20);default:'PENDING'" json:"status"`
	PaymentMethod    *string            `gorm:"type:varchar(50)" json:"payment_method,omitempty"`
	PaymentReference *string            `gorm:"type:varchar(255)" json:"payment_reference,omitempty"`
	TransactionID    *uuid.UUID         `gorm:"type:uuid" json:"transaction_id,omitempty"`
	CompletedAt      *time.Time         `json:"completed_at,omitempty"`

	// Relationships
	User        User             `gorm:"foreignKey:UserID;constraint:OnDelete:CASCADE" json:"-"`
	Package     *CoinPackage     `gorm:"foreignKey:PackageID" json:"package,omitempty"`
	Transaction *CoinTransaction `gorm:"foreignKey:TransactionID" json:"transaction,omitempty"`
}

func (CoinPurchase) TableName() string {
	return "coin_purchases"
}
