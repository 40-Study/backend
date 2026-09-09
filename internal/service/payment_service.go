package service

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"gorm.io/gorm"
	"study.com/v1/internal/dto"
	"study.com/v1/internal/model"
	"study.com/v1/internal/repository"
)

var (
	ErrPaymentNotFound          = errors.New("payment not found")
	ErrPaymentAlreadyDone       = errors.New("payment already processed")
	ErrPaymentAmountMismatch    = errors.New("payment amount mismatch")
	ErrPaymentExpired           = errors.New("payment expired")
	ErrInvalidPaymentStatus     = errors.New("invalid payment status")
	ErrProviderSignatureInvalid = errors.New("provider signature invalid")
	ErrTransactionNotFound      = errors.New("transaction not found")
	ErrTransactionPending       = errors.New("transaction pending")
)

type PaymentServiceInterface interface {
	CreatePaymentIntent(ctx context.Context, userID, orderID uuid.UUID, isAdmin bool, paymentMethod string) (*dto.PaymentIntentResponse, error)
	CheckAndProcessPayment(ctx context.Context, orderID, actorUserID uuid.UUID, isAdmin bool) (*dto.PaymentStatusResponse, error)
	GetPaymentStatus(ctx context.Context, orderID, actorUserID uuid.UUID, isAdmin bool) (*dto.PaymentStatusResponse, error)
}

type PaymentService struct {
	orderRepo          repository.OrderRepositoryInterface
	orderItemRepo      repository.OrderItemRepositoryInterface
	paymentEventRepo   repository.PaymentEventRepositoryInterface
	orderHistoryRepo   repository.OrderStatusHistoryRepositoryInterface
	enrollmentRepo     interface{}
	couponRepo         repository.CouponRepositoryInterface
	transactionService TransactionServiceInterface
}

func NewPaymentService(
	orderRepo repository.OrderRepositoryInterface,
	orderItemRepo repository.OrderItemRepositoryInterface,
	paymentEventRepo repository.PaymentEventRepositoryInterface,
	orderHistoryRepo repository.OrderStatusHistoryRepositoryInterface,
	enrollmentRepo interface{},
	couponRepo repository.CouponRepositoryInterface,
	transactionService TransactionServiceInterface,
) *PaymentService {
	return &PaymentService{
		orderRepo:          orderRepo,
		orderItemRepo:      orderItemRepo,
		paymentEventRepo:   paymentEventRepo,
		orderHistoryRepo:   orderHistoryRepo,
		enrollmentRepo:     enrollmentRepo,
		couponRepo:         couponRepo,
		transactionService: transactionService,
	}
}

func (s *PaymentService) CreatePaymentIntent(ctx context.Context, userID, orderID uuid.UUID, isAdmin bool, paymentMethod string) (*dto.PaymentIntentResponse, error) {
	// Get order
	order, err := s.orderRepo.GetByID(orderID)
	if err != nil {
		return nil, ErrOrderNotFound
	}

	// H-06 (audit 260909 vòng 2): thêm nhánh admin override — trước đây đã có check chủ đơn
	// hàng nhưng dùng errors.New("unauthorized") không phân biệt được để trả 403 ở handler.
	if order.UserID != userID && !isAdmin {
		return nil, ErrOrderForbidden
	}

	// Verify order is in correct state
	if order.Status != "pending" {
		return nil, ErrInvalidStateTransition
	}

	// Generate payment code
	paymentCode := s.generatePaymentCode()

	// Update order to processing status
	oldStatus := order.Status
	if err := s.orderRepo.UpdateStatus(orderID, "processing"); err != nil {
		return nil, err
	}

	// Create status history
	history := &model.OrderStatusHistory{
		ID:         uuid.New(),
		CreatedAt:  time.Now(),
		OrderID:    orderID,
		FromStatus: oldStatus,
		ToStatus:   "processing",
		Reason:     "Payment initiated",
	}
	s.orderHistoryRepo.Create(history)

	// Build response
	resp := &dto.PaymentIntentResponse{
		OrderID:     orderID,
		PaymentCode: paymentCode,
		Amount:      order.TotalAmount,
		Currency:    order.Currency,
		ExpiredAt:   time.Now().Add(24 * time.Hour), // 24 hours expiry
	}

	// Generate QR content for QR transfer
	if paymentMethod == "qr_transfer" {
		resp.QRContent = s.generateQRContent(order, paymentCode)
	} else if paymentMethod == "bank_transfer" {
		bankName, accountNumber, accountName := getBankTransferInfoFromEnv()
		resp.BankTransferInfo = &dto.BankTransferInfo{
			BankName:      bankName,
			AccountNumber: accountNumber,
			AccountName:   accountName,
			Content:       fmt.Sprintf("40STUDY %s", paymentCode),
		}
	}

	return resp, nil
}

// CheckAndProcessPayment - Check transaction via gRPC and process if found
func (s *PaymentService) CheckAndProcessPayment(ctx context.Context, orderID, actorUserID uuid.UUID, isAdmin bool) (*dto.PaymentStatusResponse, error) {
	// Get order
	order, err := s.orderRepo.GetByID(orderID)
	if err != nil {
		return nil, ErrOrderNotFound
	}
	// H-06: trước đây route này không nhận userID gì cả — bất kỳ user đăng nhập nào biết
	// orderID cũng trigger check thanh toán (và xem kết quả) đơn hàng của người khác.
	if order.UserID != actorUserID && !isAdmin {
		return nil, ErrOrderForbidden
	}

	// If already completed, return success
	if order.Status == "completed" {
		return &dto.PaymentStatusResponse{
			OrderID: orderID,
			Status:  order.Status,
			PaidAt:  order.PaidAt,
			Amount:  order.TotalAmount,
		}, nil
	}

	// If not processing, can't check
	if order.Status != "processing" {
		return &dto.PaymentStatusResponse{
			OrderID: orderID,
			Status:  order.Status,
			Amount:  order.TotalAmount,
		}, nil
	}

	// Get payment code from order (stored in PaymentTransactionID for now)
	paymentCode := ""
	if order.PaymentTransactionID != nil {
		paymentCode = *order.PaymentTransactionID
	}

	if paymentCode == "" {
		return nil, errors.New("payment code not found")
	}

	// Calculate time range (last 24 hours)
	toTime := time.Now()
	fromTime := toTime.Add(-24 * time.Hour)

	// Call transaction service via gRPC
	result, err := s.transactionService.CheckTransaction(ctx, paymentCode, fromTime, toTime)
	if err != nil {
		return nil, fmt.Errorf("failed to check transaction: %w", err)
	}

	// If transaction not found
	if !result.Found {
		return &dto.PaymentStatusResponse{
			OrderID: orderID,
			Status:  "pending",
			Amount:  order.TotalAmount,
		}, nil
	}

	// Verify amount matches
	amount, _ := decimal.NewFromString(result.Amount)
	if amount.Compare(order.TotalAmount) != 0 {
		return nil, ErrPaymentAmountMismatch
	}

	// Transaction found, complete the order
	oldStatus := order.Status

	err = s.orderRepo.WithTransaction(func(txRepo *repository.OrderRepository) error {
		// M-06 (audit 260909 vòng 2): chống replay — giao dịch ngân hàng này (result.TransactionID)
		// đã dùng cho đơn/lần mua xu khác chưa? Unique constraint DB-level (bank_transaction_usages)
		// là chốt chặn thật; vi phạm -> insert lỗi -> transaction rollback -> đơn KHÔNG completed.
		usage := &model.BankTransactionUsage{
			BankTransactionID: result.TransactionID,
			ReferenceType:     "order",
			ReferenceID:       order.ID,
		}
		if err := txRepo.RecordBankTransactionUsage(usage); err != nil {
			if errors.Is(err, gorm.ErrDuplicatedKey) {
				return ErrPaymentAlreadyDone
			}
			return err
		}

		// Update payment info
		if err := txRepo.UpdatePaymentInfo(order.ID, "bank_transfer", "mbbank", result.TransactionID, time.Now()); err != nil {
			return err
		}

		// Create history
		history := &model.OrderStatusHistory{
			ID:         uuid.New(),
			CreatedAt:  time.Now(),
			OrderID:    order.ID,
			FromStatus: oldStatus,
			ToStatus:   "completed",
			Reason:     "Payment received via transaction check",
		}
		if err := s.orderHistoryRepo.Create(history); err != nil {
			return err
		}

		return nil
	})

	if err != nil {
		return nil, err
	}

	// Create enrollments for each course
	items, err := s.orderItemRepo.GetByOrderID(orderID)
	if err != nil {
		return nil, err
	}

	// H-08 (audit 260909 vòng 2): trước đây các lệnh ghi enrollment/coupon usage dưới đây
	// gọi hàm nhưng KHÔNG gán lỗi trả về vào biến nào cả (không có cả "_ ="), nên lỗi INSERT
	// bị nuốt hoàn toàn — đơn hàng chuyển "completed" (đã trừ tiền/xác nhận thanh toán) nhưng
	// học viên có thể không được ghi danh, không log, không cách nào phát hiện. Sửa để lỗi
	// được trả về (visible) thay vì biến mất. LƯU Ý: đây chỉ đóng phần "lỗi bị nuốt", KHÔNG
	// đóng H-07 (toàn bộ khối này vẫn chưa cùng transaction với UpdatePaymentInfo/history phía
	// trên — đó là thay đổi kiến trúc Unit-of-Work lớn hơn, xem "chưa làm" trong báo cáo).
	if s.enrollmentRepo != nil {
		enrollmentRepo, ok := s.enrollmentRepo.(interface {
			GetByUserAndCourse(ctx context.Context, userID, courseID uuid.UUID) (*model.Enrollment, error)
			Create(ctx context.Context, enrollment *model.Enrollment) error
		})
		if !ok {
			return nil, errors.New("enrollmentRepo does not implement required interface")
		}

		for _, item := range items {
			// Check if already enrolled
			existingEnrollment, checkErr := enrollmentRepo.GetByUserAndCourse(ctx, order.UserID, item.CourseID)
			if checkErr == nil && existingEnrollment != nil {
				continue // Already enrolled
			}

			// Create enrollment
			enrollment := &model.Enrollment{
				UserID:     order.UserID,
				CourseID:   item.CourseID,
				EnrolledAt: time.Now(),
			}
			if err := enrollmentRepo.Create(ctx, enrollment); err != nil {
				return nil, fmt.Errorf("failed to create enrollment for course %s: %w", item.CourseID, err)
			}
		}

		// Update coupon usage if applicable
		if order.CouponID != nil {
			if err := s.couponRepo.IncrementUsageCount(*order.CouponID); err != nil {
				return nil, fmt.Errorf("failed to increment coupon usage: %w", err)
			}

			usage := &model.CouponUsage{
				ID:             uuid.New(),
				CreatedAt:      time.Now(),
				CouponID:       *order.CouponID,
				UserID:         order.UserID,
				OrderID:        orderID,
				DiscountAmount: order.DiscountAmount,
			}
			if err := s.couponRepo.CreateUsage(usage); err != nil {
				return nil, fmt.Errorf("failed to create coupon usage record: %w", err)
			}
		}
	}

	now := time.Now()
	return &dto.PaymentStatusResponse{
		OrderID: orderID,
		Status:  "completed",
		PaidAt:  &now,
		Amount:  order.TotalAmount,
	}, nil
}

// GetPaymentStatus - Get payment status for order
func (s *PaymentService) GetPaymentStatus(ctx context.Context, orderID, actorUserID uuid.UUID, isAdmin bool) (*dto.PaymentStatusResponse, error) {
	order, err := s.orderRepo.GetByID(orderID)
	if err != nil {
		return nil, ErrOrderNotFound
	}
	// H-06: cùng lý do CheckAndProcessPayment — route trước đây không kiểm tra chủ đơn hàng.
	if order.UserID != actorUserID && !isAdmin {
		return nil, ErrOrderForbidden
	}

	// If order is still processing, try to check transaction
	if order.Status == "processing" {
		return s.CheckAndProcessPayment(ctx, orderID, actorUserID, isAdmin)
	}

	return &dto.PaymentStatusResponse{
		OrderID: orderID,
		Status:  order.Status,
		PaidAt:  order.PaidAt,
		Amount:  order.TotalAmount,
	}, nil
}

func (s *PaymentService) generatePaymentCode() string {
	randomBytes := make([]byte, 8)
	rand.Read(randomBytes)
	return fmt.Sprintf("PAY%s%s", time.Now().Format("060102150405"), hex.EncodeToString(randomBytes)[:8])
}

func (s *PaymentService) generateQRContent(order *model.Order, paymentCode string) string {
	bankName, accountNumber, _ := getBankTransferInfoFromEnv()

	qrData := map[string]interface{}{
		"bank":    bankName,
		"account": accountNumber,
		"amount":  order.TotalAmount.String(),
		"content": fmt.Sprintf("40STUDY %s", paymentCode),
		"order":   order.OrderNumber,
	}

	data, _ := json.Marshal(qrData)
	return string(data)
}

func getBankTransferInfoFromEnv() (bankName, accountNumber, accountName string) {
	bankName = os.Getenv("MB_BANK_NAME")
	if bankName == "" {
		bankName = "MB"
	}

	accountNumber = os.Getenv("MB_ACCOUNT_NO")
	if accountNumber == "" {
		accountNumber = "1234567890"
	}

	accountName = os.Getenv("BANK_ACCOUNT_NAME")
	if accountName == "" {
		accountName = "40Study"
	}

	return bankName, accountNumber, accountName
}
