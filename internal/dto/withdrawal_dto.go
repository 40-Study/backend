package dto

import (
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

// ─── Phase 4: rút tiền giảng viên ───────────────────────────────────────────

// CreateWithdrawalRequest — POST /api/wallet/teacher/withdrawals. Amount nhận cả "500000" lẫn 500000.
type CreateWithdrawalRequest struct {
	Amount decimal.Decimal `json:"amount"`
}

// RejectWithdrawalRequest — POST /api/admin/withdrawals/:id/reject.
type RejectWithdrawalRequest struct {
	Reason string `json:"reason"`
}

// MarkWithdrawalCompletedRequest — POST /api/admin/withdrawals/:id/mark-completed.
type MarkWithdrawalCompletedRequest struct {
	TransactionID string `json:"transaction_id"`
}

// WithdrawalItem — 1 yêu cầu rút (giáo viên xem của chính mình).
type WithdrawalItem struct {
	ID                uuid.UUID       `json:"id"`
	Amount            decimal.Decimal `json:"amount"`
	Currency          string          `json:"currency"`
	Status            string          `json:"status"`
	RejectionReason   *string         `json:"rejection_reason"`
	TransactionID     *string         `json:"transaction_id"`
	BankName          *string         `json:"bank_name"`
	BankAccountNumber *string         `json:"bank_account_number"`
	BankAccountName   *string         `json:"bank_account_name"`
	CreatedAt         time.Time       `json:"created_at"`
	ProcessedAt       *time.Time      `json:"processed_at"`
}

// AdminWithdrawalItem — thêm thông tin giảng viên + số dư hiện tại (âm => cảnh báo trên UI).
type AdminWithdrawalItem struct {
	WithdrawalItem
	TeacherID               uuid.UUID       `json:"teacher_id"`
	TeacherName             string          `json:"teacher_name"`
	TeacherEmail            string          `json:"teacher_email"`
	TeacherAvailableBalance decimal.Decimal `json:"teacher_available_balance"`
}

// WithdrawalListResponse — phân trang chuẩn {items,total_count,page,limit,total_pages}.
type WithdrawalListResponse struct {
	Items      []WithdrawalItem `json:"items"`
	TotalCount int64            `json:"total_count"`
	Page       int              `json:"page"`
	Limit      int              `json:"limit"`
	TotalPages int              `json:"total_pages"`
}

type AdminWithdrawalListResponse struct {
	Items      []AdminWithdrawalItem `json:"items"`
	TotalCount int64                 `json:"total_count"`
	Page       int                   `json:"page"`
	Limit      int                   `json:"limit"`
	TotalPages int                   `json:"total_pages"`
}

// WithdrawalStatusResponse — kết quả approve/reject/mark-completed.
type WithdrawalStatusResponse struct {
	ID     uuid.UUID `json:"id"`
	Status string    `json:"status"`
}

// NegativeBalanceTeacher — giảng viên đang có số dư âm (quyết định #8: admin thấy cảnh báo).
type NegativeBalanceTeacher struct {
	TeacherID        uuid.UUID       `json:"teacher_id"`
	TeacherName      string          `json:"teacher_name"`
	TeacherEmail     string          `json:"teacher_email"`
	AvailableBalance decimal.Decimal `json:"available_balance"`
}

type NegativeBalanceListResponse struct {
	Items []NegativeBalanceTeacher `json:"items"`
}
