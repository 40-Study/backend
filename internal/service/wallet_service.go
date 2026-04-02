package service

import (
	"context"
	"math"

	"github.com/google/uuid"
	"study.com/v1/internal/dto"
	"study.com/v1/internal/repository"
)

// WalletServiceInterface defines the wallet service contract
type WalletServiceInterface interface {
	GetWallet(ctx context.Context, userID uuid.UUID) (*dto.WalletResponse, error)
	GetTransactions(ctx context.Context, userID uuid.UUID, txType string, page, limit int) (*dto.WalletTransactionListResponse, error)
}

// WalletService implements WalletServiceInterface
type WalletService struct {
	walletRepo *repository.WalletRepository
}

func NewWalletService(walletRepo *repository.WalletRepository) *WalletService {
	return &WalletService{walletRepo: walletRepo}
}

// GetWallet returns the user's wallet summary (total spent + order count)
func (s *WalletService) GetWallet(_ context.Context, userID uuid.UUID) (*dto.WalletResponse, error) {
	totalSpent, orderCount, err := s.walletRepo.GetTotalSpent(userID)
	if err != nil {
		return nil, err
	}

	return &dto.WalletResponse{
		UserID:     userID,
		TotalSpent: totalSpent,
		Currency:   "VND",
		OrderCount: orderCount,
	}, nil
}

// GetTransactions returns paginated transaction history for the user
func (s *WalletService) GetTransactions(_ context.Context, userID uuid.UUID, txType string, page, limit int) (*dto.WalletTransactionListResponse, error) {
	orders, total, err := s.walletRepo.GetTransactions(userID, txType, page, limit)
	if err != nil {
		return nil, err
	}

	transactions := make([]dto.WalletTransaction, 0, len(orders))
	for _, o := range orders {
		txKind := dto.TransactionTypeExpense
		if o.Status == "refunded" {
			txKind = dto.TransactionTypeIncome
		}

		description := buildDescription(o.Status)

		transactions = append(transactions, dto.WalletTransaction{
			ID:            o.ID,
			OrderNumber:   o.OrderNumber,
			Amount:        o.TotalAmount,
			Currency:      o.Currency,
			Type:          txKind,
			Status:        o.Status,
			PaymentMethod: o.PaymentMethod,
			Description:   description,
			CreatedAt:     o.CreatedAt,
			PaidAt:        o.PaidAt,
		})
	}

	totalPages := int(math.Ceil(float64(total) / float64(limit)))

	return &dto.WalletTransactionListResponse{
		Transactions: transactions,
		TotalCount:   total,
		Page:         page,
		Limit:        limit,
		TotalPages:   totalPages,
	}, nil
}

func buildDescription(status string) string {
	switch status {
	case "completed":
		return "Course purchase"
	case "refunded":
		return "Order refund"
	case "cancelled":
		return "Order cancelled"
	default:
		return "Order " + status
	}
}
