package dto

import (
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

// WalletResponse represents the user's wallet summary
type WalletResponse struct {
	UserID        uuid.UUID       `json:"user_id"`
	TotalSpent    decimal.Decimal `json:"total_spent"`
	Currency      string          `json:"currency"`
	OrderCount    int64           `json:"order_count"`
}

// TransactionType classifies a transaction as income or expense
type TransactionType string

const (
	TransactionTypeIncome  TransactionType = "income"
	TransactionTypeExpense TransactionType = "expense"
)

// WalletTransaction is a single transaction item in history
type WalletTransaction struct {
	ID            uuid.UUID       `json:"id"`
	OrderNumber   string          `json:"order_number"`
	Amount        decimal.Decimal `json:"amount"`
	Currency      string          `json:"currency"`
	Type          TransactionType `json:"type"`
	Status        string          `json:"status"`
	PaymentMethod *string         `json:"payment_method,omitempty"`
	Description   string          `json:"description"`
	CreatedAt     time.Time       `json:"created_at"`
	PaidAt        *time.Time      `json:"paid_at,omitempty"`
}

// WalletTransactionListResponse is paginated transaction history
type WalletTransactionListResponse struct {
	Transactions []WalletTransaction `json:"transactions"`
	TotalCount   int64               `json:"total_count"`
	Page         int                 `json:"page"`
	Limit        int                 `json:"limit"`
	TotalPages   int                 `json:"total_pages"`
}
