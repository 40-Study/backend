package service

import (
	"context"
	"fmt"
	"math"

	"github.com/google/uuid"
	"study.com/v1/internal/dto"
	"study.com/v1/internal/repository"
)

// WalletServiceInterface defines the wallet service contract
type WalletServiceInterface interface {
	// Student
	GetWallet(ctx context.Context, userID uuid.UUID) (*dto.WalletResponse, error)
	GetTransactions(ctx context.Context, userID uuid.UUID, txType string, page, limit int) (*dto.WalletTransactionListResponse, error)
	// Teacher
	GetTeacherWallet(ctx context.Context, teacherID uuid.UUID) (*dto.TeacherWalletResponse, error)
	GetTeacherTransactions(ctx context.Context, teacherID uuid.UUID, txType string, page, limit int) (*dto.TeacherTransactionListResponse, error)
	UpdateBankInfo(ctx context.Context, teacherID uuid.UUID, req dto.UpdateBankInfoRequest) error
}

// WalletService implements WalletServiceInterface
type WalletService struct {
	walletRepo         *repository.WalletRepository
	teacherProfileRepo repository.TeacherProfileRepositoryInterface
}

func NewWalletService(walletRepo *repository.WalletRepository, teacherProfileRepo repository.TeacherProfileRepositoryInterface) *WalletService {
	return &WalletService{walletRepo: walletRepo, teacherProfileRepo: teacherProfileRepo}
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

// ─── Teacher wallet methods ─────────────────────────────────────────────────

// GetTeacherWallet returns the teacher's earnings summary + bank info
func (s *WalletService) GetTeacherWallet(ctx context.Context, teacherID uuid.UUID) (*dto.TeacherWalletResponse, error) {
	earnings, err := s.walletRepo.GetTeacherEarnings(teacherID)
	if err != nil {
		return nil, err
	}

	paidOut, err := s.walletRepo.GetTeacherPayoutTotal(teacherID)
	if err != nil {
		return nil, err
	}

	// Get bank info from teacher profile
	profile, err := s.teacherProfileRepo.GetByUserID(ctx, teacherID)
	if err != nil {
		return nil, err
	}

	resp := &dto.TeacherWalletResponse{
		UserID:        teacherID,
		TotalEarnings: earnings.TotalEarnings,
		TotalPaidOut:  paidOut,
		AvailBalance:  earnings.TotalEarnings.Sub(paidOut),
		Currency:      "VND",
		OrderCount:    earnings.OrderCount,
	}

	if profile != nil {
		resp.BankName = profile.BankName
		resp.BankAccountNum = profile.BankAccountNumber
		resp.BankAccountNam = profile.BankAccountName
	}

	return resp, nil
}

// GetTeacherTransactions returns paginated earning transactions for the teacher
func (s *WalletService) GetTeacherTransactions(_ context.Context, teacherID uuid.UUID, txType string, page, limit int) (*dto.TeacherTransactionListResponse, error) {
	rows, total, err := s.walletRepo.GetTeacherTransactions(teacherID, txType, page, limit)
	if err != nil {
		return nil, err
	}

	txs := make([]dto.TeacherTransaction, 0, len(rows))
	for _, r := range rows {
		txKind := dto.TransactionTypeIncome
		if r.OrderStatus == "refunded" {
			txKind = dto.TransactionTypeExpense
		}

		txs = append(txs, dto.TeacherTransaction{
			OrderID:       r.OrderID,
			OrderNumber:   r.OrderNumber,
			CourseName:    r.CourseName,
			CourseID:      r.CourseID,
			BuyerName:     r.BuyerName,
			Amount:        r.FinalPrice,
			Currency:      r.Currency,
			Type:          txKind,
			Status:        r.OrderStatus,
			PaymentMethod: r.PaymentMethod,
			CreatedAt:     r.CreatedAt,
			PaidAt:        r.PaidAt,
		})
	}

	totalPages := int(math.Ceil(float64(total) / float64(limit)))
	return &dto.TeacherTransactionListResponse{
		Transactions: txs,
		TotalCount:   total,
		Page:         page,
		Limit:        limit,
		TotalPages:   totalPages,
	}, nil
}

// UpdateBankInfo updates the teacher's bank info in their profile
func (s *WalletService) UpdateBankInfo(ctx context.Context, teacherID uuid.UUID, req dto.UpdateBankInfoRequest) error {
	profile, err := s.teacherProfileRepo.GetByUserID(ctx, teacherID)
	if err != nil {
		return err
	}
	if profile == nil {
		return fmt.Errorf("teacher profile not found")
	}

	profile.BankName = &req.BankName
	profile.BankAccountNumber = &req.BankAccountNumber
	profile.BankAccountName = &req.BankAccountName

	return s.teacherProfileRepo.Update(ctx, profile)
}
