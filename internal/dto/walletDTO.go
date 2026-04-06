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

// ─── Teacher wallet DTOs ────────────────────────────────────────────────────

// TeacherWalletResponse is the teacher's earnings summary
type TeacherWalletResponse struct {
	UserID         uuid.UUID       `json:"user_id"`
	TotalEarnings  decimal.Decimal `json:"total_earnings"`
	TotalPaidOut   decimal.Decimal `json:"total_paid_out"`
	AvailBalance   decimal.Decimal `json:"available_balance"`
	Currency       string          `json:"currency"`
	OrderCount     int64           `json:"order_count"`
	BankName       *string         `json:"bank_name,omitempty"`
	BankAccountNum *string         `json:"bank_account_number,omitempty"`
	BankAccountNam *string         `json:"bank_account_name,omitempty"`
}

// TeacherTransaction is a single earning/refund row for the teacher
type TeacherTransaction struct {
	OrderID       uuid.UUID       `json:"order_id"`
	OrderNumber   string          `json:"order_number"`
	CourseName    string          `json:"course_name"`
	CourseID      uuid.UUID       `json:"course_id"`
	BuyerName     string          `json:"buyer_name"`
	Amount        decimal.Decimal `json:"amount"`
	Currency      string          `json:"currency"`
	Type          TransactionType `json:"type"`
	Status        string          `json:"status"`
	PaymentMethod *string         `json:"payment_method,omitempty"`
	CreatedAt     time.Time       `json:"created_at"`
	PaidAt        *time.Time      `json:"paid_at,omitempty"`
}

// TeacherTransactionListResponse is paginated teacher transaction history
type TeacherTransactionListResponse struct {
	Transactions []TeacherTransaction `json:"transactions"`
	TotalCount   int64                `json:"total_count"`
	Page         int                  `json:"page"`
	Limit        int                  `json:"limit"`
	TotalPages   int                  `json:"total_pages"`
}

// UpdateBankInfoRequest is the request body for updating bank info
type UpdateBankInfoRequest struct {
	BankName          string `json:"bank_name" validate:"required"`
	BankAccountNumber string `json:"bank_account_number" validate:"required"`
	BankAccountName   string `json:"bank_account_name" validate:"required"`
}
