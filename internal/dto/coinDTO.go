package dto

import (
	"time"

	"github.com/google/uuid"
)

// ============================================================================
// COIN WALLET DTOs
// ============================================================================

type CoinWalletResponse struct {
	ID          uuid.UUID `json:"id"`
	UserID      uuid.UUID `json:"user_id"`
	Balance     int64     `json:"balance"`
	TotalEarned int64     `json:"total_earned"`
	TotalSpent  int64     `json:"total_spent"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// ============================================================================
// COIN TRANSACTION DTOs
// ============================================================================

type CoinTransactionResponse struct {
	ID            uuid.UUID  `json:"id"`
	Type          string     `json:"type"`
	Amount        int64      `json:"amount"`
	BalanceAfter  int64      `json:"balance_after"`
	ReferenceType *string    `json:"reference_type,omitempty"`
	ReferenceID   *uuid.UUID `json:"reference_id,omitempty"`
	Description   *string    `json:"description,omitempty"`
	CreatedAt     time.Time  `json:"created_at"`
}

type CoinTransactionListResponse struct {
	Transactions []CoinTransactionResponse `json:"transactions"`
	TotalCount   int64                     `json:"total_count"`
	Page         int                       `json:"page"`
	Limit        int                       `json:"limit"`
}

// ============================================================================
// COIN PACKAGE DTOs
// ============================================================================

type CoinPackageResponse struct {
	ID              uuid.UUID `json:"id"`
	Name            string    `json:"name"`
	Description     *string   `json:"description,omitempty"`
	CoinAmount      int64     `json:"coin_amount"`
	BonusAmount     int64     `json:"bonus_amount"`
	TotalCoins      int64     `json:"total_coins"`
	Price           float64   `json:"price"`
	Currency        string    `json:"currency"`
	DiscountPercent int       `json:"discount_percent"`
	IsFeatured      bool      `json:"is_featured"`
	SortOrder       int       `json:"sort_order"`
}

type CreateCoinPackageRequest struct {
	Name            string  `json:"name" validate:"required,min=2,max=100"`
	Description     *string `json:"description,omitempty"`
	CoinAmount      int64   `json:"coin_amount" validate:"required,min=1"`
	BonusAmount     int64   `json:"bonus_amount" validate:"min=0"`
	Price           float64 `json:"price" validate:"required,gt=0"`
	Currency        string  `json:"currency" validate:"omitempty,len=3"`
	DiscountPercent int     `json:"discount_percent" validate:"min=0,max=100"`
	IsFeatured      bool    `json:"is_featured"`
	SortOrder       int     `json:"sort_order"`
}

type UpdateCoinPackageRequest struct {
	Name            *string  `json:"name,omitempty" validate:"omitempty,min=2,max=100"`
	Description     *string  `json:"description,omitempty"`
	CoinAmount      *int64   `json:"coin_amount,omitempty" validate:"omitempty,min=1"`
	BonusAmount     *int64   `json:"bonus_amount,omitempty" validate:"omitempty,min=0"`
	Price           *float64 `json:"price,omitempty" validate:"omitempty,gt=0"`
	DiscountPercent *int     `json:"discount_percent,omitempty" validate:"omitempty,min=0,max=100"`
	IsFeatured      *bool    `json:"is_featured,omitempty"`
	IsActive        *bool    `json:"is_active,omitempty"`
	SortOrder       *int     `json:"sort_order,omitempty"`
}

// ============================================================================
// COIN PURCHASE DTOs
// ============================================================================

type CreateCoinPurchaseRequest struct {
	PackageID     uuid.UUID `json:"package_id" validate:"required"`
	PaymentMethod string    `json:"payment_method" validate:"required"`
}

type CoinPurchaseResponse struct {
	ID               uuid.UUID  `json:"id"`
	UserID           uuid.UUID  `json:"user_id"`
	PackageID        *uuid.UUID `json:"package_id,omitempty"`
	CoinAmount       int64      `json:"coin_amount"`
	BonusAmount      int64      `json:"bonus_amount"`
	Price            float64    `json:"price"`
	Currency         string     `json:"currency"`
	Status           string     `json:"status"`
	PaymentMethod    *string    `json:"payment_method,omitempty"`
	PaymentReference *string    `json:"payment_reference,omitempty"`
	CompletedAt      *time.Time `json:"completed_at,omitempty"`
	CreatedAt        time.Time  `json:"created_at"`
}

type CoinPurchaseListResponse struct {
	Purchases  []CoinPurchaseResponse `json:"purchases"`
	TotalCount int64                  `json:"total_count"`
	Page       int                    `json:"page"`
	Limit      int                    `json:"limit"`
}

// ============================================================================
// COIN GIFT DTOs
// ============================================================================

type SendCoinGiftRequest struct {
	ReceiverID uuid.UUID `json:"receiver_id" validate:"required"`
	Amount     int64     `json:"amount" validate:"required,min=1"`
	Message    *string   `json:"message,omitempty"`
}

type CoinGiftResponse struct {
	SenderID     uuid.UUID `json:"sender_id"`
	ReceiverID   uuid.UUID `json:"receiver_id"`
	Amount       int64     `json:"amount"`
	Message      *string   `json:"message,omitempty"`
	SenderBalance   int64  `json:"sender_balance"`
	ReceiverBalance int64  `json:"receiver_balance"`
}

// ============================================================================
// ADMIN DTOs
// ============================================================================

type AdminAdjustCoinRequest struct {
	UserID      uuid.UUID `json:"user_id" validate:"required"`
	Amount      int64     `json:"amount" validate:"required"`
	Description string    `json:"description" validate:"required,min=1"`
}
