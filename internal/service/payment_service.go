package service

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/url"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/spf13/viper"
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
	CreatePaymentIntent(ctx context.Context, userID, orderID uuid.UUID, paymentMethod string) (*dto.PaymentIntentResponse, error)
	CheckAndProcessPayment(ctx context.Context, orderID uuid.UUID) (*dto.PaymentStatusResponse, error)
	GetPaymentStatus(ctx context.Context, orderID uuid.UUID) (*dto.PaymentStatusResponse, error)
	GetPaymentCaptureInfo(order *model.Order) *dto.PaymentCaptureInfo
}

type PaymentService struct {
	orderRepo           repository.OrderRepositoryInterface
	orderItemRepo       repository.OrderItemRepositoryInterface
	paymentEventRepo    repository.PaymentEventRepositoryInterface
	orderHistoryRepo    repository.OrderStatusHistoryRepositoryInterface
	enrollmentRepo      interface{}
	couponRepo          repository.CouponRepositoryInterface
	transactionService  TransactionServiceInterface
	notificationService NotificationServiceInterface
}

func NewPaymentService(
	orderRepo repository.OrderRepositoryInterface,
	orderItemRepo repository.OrderItemRepositoryInterface,
	paymentEventRepo repository.PaymentEventRepositoryInterface,
	orderHistoryRepo repository.OrderStatusHistoryRepositoryInterface,
	enrollmentRepo interface{},
	couponRepo repository.CouponRepositoryInterface,
	transactionService TransactionServiceInterface,
	notificationService NotificationServiceInterface,
) *PaymentService {
	return &PaymentService{
		orderRepo:           orderRepo,
		orderItemRepo:       orderItemRepo,
		paymentEventRepo:    paymentEventRepo,
		orderHistoryRepo:    orderHistoryRepo,
		enrollmentRepo:      enrollmentRepo,
		couponRepo:          couponRepo,
		transactionService:  transactionService,
		notificationService: notificationService,
	}
}

const PaymentCodeTTL = 30 * time.Minute

func (s *PaymentService) CreatePaymentIntent(ctx context.Context, userID, orderID uuid.UUID, paymentMethod string) (*dto.PaymentIntentResponse, error) {
	order, err := s.orderRepo.GetByID(orderID)
	if err != nil {
		return nil, ErrOrderNotFound
	}

	if order.UserID != userID {
		return nil, errors.New("unauthorized")
	}

	if order.Status != "pending" && order.Status != "processing" {
		return nil, ErrInvalidStateTransition
	}

	now := time.Now()
	var paymentCode string
	var paymentCodeCreatedAt time.Time
	needsNewPaymentCode := true

	// Check if existing payment code is still valid
	if order.PaymentTransactionID != nil && *order.PaymentTransactionID != "" &&
		order.PaymentCodeCreatedAt != nil {
		expiredAt := order.PaymentCodeCreatedAt.Add(PaymentCodeTTL)
		if now.Before(expiredAt) {
			paymentCode = *order.PaymentTransactionID
			paymentCodeCreatedAt = *order.PaymentCodeCreatedAt
			needsNewPaymentCode = false
		}
	}

	// Generate new payment code only if needed
	if needsNewPaymentCode {
		paymentCode = s.generatePaymentCode()
		paymentCodeCreatedAt = now

		// Update order to processing if pending
		if order.Status == "pending" {
			oldStatus := order.Status
			if err := s.orderRepo.UpdateStatus(orderID, "processing"); err != nil {
				return nil, err
			}
			history := &model.OrderStatusHistory{
				ID:         uuid.New(),
				CreatedAt:  now,
				OrderID:    orderID,
				FromStatus: oldStatus,
				ToStatus:   "processing",
				Reason:     "Payment initiated",
			}
			s.orderHistoryRepo.Create(history)
		}

		// Save new payment code
		s.orderRepo.UpdatePaymentCode(orderID, paymentCode, paymentCodeCreatedAt)

		// Send notification only for new payment
		if s.notificationService != nil {
			refType := "order"
			s.notificationService.SendNotification(dto.CreateNotificationDTO{
				Title:            "Đơn hàng đang chờ thanh toán",
				Content:          fmt.Sprintf("Đơn hàng #%s đang chờ thanh toán. Vui lòng hoàn tất thanh toán trong 30 phút.", order.OrderNumber),
				NotificationType: "payment_pending",
				ReferenceType:    &refType,
				ReferenceID:      &orderID,
				UserIDs:          []uuid.UUID{userID},
			})
		}
	}

	bankName, accountNumber, accountName := getBankTransferInfoFromEnv()
	transferContent := fmt.Sprintf("40STUDY %s", paymentCode)
	expiredAt := paymentCodeCreatedAt.Add(PaymentCodeTTL)

	return &dto.PaymentIntentResponse{
		OrderID:     orderID,
		PaymentCode: paymentCode,
		Amount:      order.TotalAmount,
		Currency:    order.Currency,
		ExpiredAt:   expiredAt,
		QRContent:   generateVietQRUrl(bankName, accountNumber, accountName, order.TotalAmount, transferContent),
		BankTransferInfo: &dto.BankTransferInfo{
			BankName:      bankName,
			AccountNumber: accountNumber,
			AccountName:   accountName,
			Content:       transferContent,
		},
	}, nil
}

// GetPaymentCaptureInfo returns payment capture info for an order (for embedding in OrderResponse)
func (s *PaymentService) GetPaymentCaptureInfo(order *model.Order) *dto.PaymentCaptureInfo {
	if order.Status != "pending" && order.Status != "processing" {
		return nil
	}

	// No payment code yet
	if order.PaymentTransactionID == nil || *order.PaymentTransactionID == "" {
		return nil
	}

	now := time.Now()
	paymentCode := *order.PaymentTransactionID

	// Check expiry
	var expiredAt time.Time
	if order.PaymentCodeCreatedAt != nil {
		expiredAt = order.PaymentCodeCreatedAt.Add(PaymentCodeTTL)
	} else {
		// Legacy: no created_at, assume expired
		return &dto.PaymentCaptureInfo{
			PaymentCode: paymentCode,
			IsExpired:   true,
		}
	}

	isExpired := now.After(expiredAt)
	if isExpired {
		return &dto.PaymentCaptureInfo{
			PaymentCode:      paymentCode,
			PaymentExpiredAt: &expiredAt,
			IsExpired:        true,
		}
	}

	// Still valid - include QR
	bankName, accountNumber, accountName := getBankTransferInfoFromEnv()
	transferContent := fmt.Sprintf("40STUDY %s", paymentCode)

	return &dto.PaymentCaptureInfo{
		PaymentCode:      paymentCode,
		QRContent:        generateVietQRUrl(bankName, accountNumber, accountName, order.TotalAmount, transferContent),
		PaymentExpiredAt: &expiredAt,
		IsExpired:        false,
		BankTransferInfo: &dto.BankTransferInfo{
			BankName:      bankName,
			AccountNumber: accountNumber,
			AccountName:   accountName,
			Content:       transferContent,
		},
	}
}

// CheckAndProcessPayment - Check transaction via gRPC and process if found
func (s *PaymentService) CheckAndProcessPayment(ctx context.Context, orderID uuid.UUID) (*dto.PaymentStatusResponse, error) {
	// Get order
	order, err := s.orderRepo.GetByID(orderID)
	if err != nil {
		return nil, ErrOrderNotFound
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

	// Calculate time range (+/- 5 minutes)
	now := time.Now()
	fromTime := now.Add(-5 * time.Minute)
	toTime := now.Add(5 * time.Minute)

	log.Printf("🔍 Checking payment for OrderID: %s, PaymentCode: %s", orderID, paymentCode)

	// Debug: Print all transactions in range
	s.transactionService.GetAllTransactions(ctx, fromTime, toTime)

	// Call transaction service via HTTP
	result, err := s.transactionService.CheckTransaction(ctx, paymentCode, fromTime, toTime)
	if err != nil {
		log.Printf("❌ Failed to check transaction: %v", err)
		return nil, fmt.Errorf("failed to check transaction: %w", err)
	}

	// If transaction not found
	if !result.Found {
		log.Printf("⏳ Transaction not found yet for PaymentCode: %s", paymentCode)
		return &dto.PaymentStatusResponse{
			OrderID: orderID,
			Status:  "pending",
			Amount:  order.TotalAmount,
		}, nil
	}

	// Verify amount matches
	amount, _ := decimal.NewFromString(result.Amount)
	if amount.Compare(order.TotalAmount) != 0 {
		log.Printf("❌ Payment amount mismatch: expected %s, got %s", order.TotalAmount.String(), result.Amount)
		return nil, ErrPaymentAmountMismatch
	}

	log.Printf("✅ PAYMENT FOUND! OrderID: %s, Amount: %s, TransactionID: %s", orderID, result.Amount, result.TransactionID)

	// Transaction found, complete the order
	oldStatus := order.Status

	err = s.orderRepo.WithTransaction(func(txRepo *repository.OrderRepository) error {
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

	// enrollmentRepo type assertion - simplified for now
	if s.enrollmentRepo != nil {
		for _, item := range items {
			// Check if already enrolled
			existingEnrollment, checkErr := s.enrollmentRepo.(interface {
				GetByUserAndCourse(ctx context.Context, userID, courseID uuid.UUID) (*model.Enrollment, error)
			}).GetByUserAndCourse(ctx, order.UserID, item.CourseID)

			if checkErr == nil && existingEnrollment != nil {
				continue // Already enrolled
			}

			// Create enrollment
			enrollment := &model.Enrollment{
				UserID:     order.UserID,
				CourseID:   item.CourseID,
				EnrolledAt: time.Now(),
			}
			err := s.enrollmentRepo.(interface {
				Create(ctx context.Context, enrollment *model.Enrollment) error
			}).Create(ctx, enrollment)
			if err != nil {
				log.Printf("❌ Failed to create enrollment for course %s: %v", item.CourseID, err)
			} else {
				log.Printf("✅ Enrolled user %s in course %s", order.UserID, item.CourseID)
			}
		}

		// Update coupon usage if applicable
		if order.CouponID != nil {
			s.couponRepo.IncrementUsageCount(*order.CouponID)

			usage := &model.CouponUsage{
				ID:             uuid.New(),
				CreatedAt:      time.Now(),
				CouponID:       *order.CouponID,
				UserID:         order.UserID,
				OrderID:        orderID,
				DiscountAmount: order.DiscountAmount,
			}
			s.couponRepo.CreateUsage(usage)
		}
	}

	completedAt := time.Now()
	log.Printf("🎉 ORDER COMPLETED! OrderID: %s, Amount: %s", orderID, order.TotalAmount.String())
	return &dto.PaymentStatusResponse{
		OrderID: orderID,
		Status:  "completed",
		PaidAt:  &completedAt,
		Amount:  order.TotalAmount,
	}, nil
}

// GetPaymentStatus - Get payment status for order
func (s *PaymentService) GetPaymentStatus(ctx context.Context, orderID uuid.UUID) (*dto.PaymentStatusResponse, error) {
	order, err := s.orderRepo.GetByID(orderID)
	if err != nil {
		return nil, ErrOrderNotFound
	}

	// If order is still processing, try to check transaction
	if order.Status == "processing" {
		return s.CheckAndProcessPayment(ctx, orderID)
	}

	return &dto.PaymentStatusResponse{
		OrderID: orderID,
		Status:  order.Status,
		PaidAt:  order.PaidAt,
		Amount:  order.TotalAmount,
	}, nil
}

func (s *PaymentService) generatePaymentCode() string {
	// Generate unique 8-char code using alphanumeric (0-9, A-Z)
	const charset = "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZ"
	code := make([]byte, 8)
	randomBytes := make([]byte, 8)
	rand.Read(randomBytes)
	for i := 0; i < 8; i++ {
		code[i] = charset[randomBytes[i]%36]
	}
	return string(code)
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
	bankName = viper.GetString("MB_BANK_NAME")
	if bankName == "" {
		bankName = "MB"
	}

	accountNumber = viper.GetString("MB_ACCOUNT_NO")
	if accountNumber == "" {
		accountNumber = "0343150904"
	}

	accountName = viper.GetString("BANK_ACCOUNT_NAME")
	if accountName == "" {
		accountName = "FORTEX"
	}

	return bankName, accountNumber, accountName
}

func generateVietQRUrl(bankCode, accountNumber, accountName string, amount decimal.Decimal, content string) string {
	// VietQR format: https://img.vietqr.io/image/{BankCode}-{AccountNumber}-{template}.png
	baseUrl := "https://img.vietqr.io/image"
	template := "compact2"

	return fmt.Sprintf("%s/%s-%s-%s.png?amount=%s&addInfo=%s&accountName=%s",
		baseUrl,
		bankCode,
		accountNumber,
		template,
		amount.String(),
		url.QueryEscape(content),
		url.QueryEscape(accountName),
	)
}
