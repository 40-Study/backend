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
	// enrollmentRepo (item 14, dọn dẹp phụ khi tách completeOrderFulfillment dùng chung):
	// TRƯỚC ĐÂY khai kiểu interface{} rồi type-assert bằng interface ẩn danh mỗi lần dùng
	// (xem git blame CheckAndProcessPayment cũ) — không cần thiết vì repos.Enrollment luôn
	// implement đúng repository.EnrollmentRepositoryInterface (xem app/services.go). Đổi
	// sang kiểu cụ thể để completeOrderFulfillment (dùng chung với OrderService.CreateOrder,
	// đơn 0đ) không phải type-assert lại.
	enrollmentRepo     repository.EnrollmentRepositoryInterface
	// courseRepo (H2-04, review vòng 3): completeOrderFulfillment cần tăng total_students khi
	// tạo/khôi phục enrollment — trước đây hàm này hoàn toàn không đụng tới total_students.
	courseRepo         repository.CourseRepositoryInterface
	couponRepo         repository.CouponRepositoryInterface
	voucherService     VoucherServiceInterface
	transactionService TransactionServiceInterface
}

func NewPaymentService(
	orderRepo repository.OrderRepositoryInterface,
	orderItemRepo repository.OrderItemRepositoryInterface,
	paymentEventRepo repository.PaymentEventRepositoryInterface,
	orderHistoryRepo repository.OrderStatusHistoryRepositoryInterface,
	enrollmentRepo repository.EnrollmentRepositoryInterface,
	courseRepo repository.CourseRepositoryInterface,
	couponRepo repository.CouponRepositoryInterface,
	voucherService VoucherServiceInterface,
	transactionService TransactionServiceInterface,
) *PaymentService {
	return &PaymentService{
		orderRepo:          orderRepo,
		orderItemRepo:      orderItemRepo,
		paymentEventRepo:   paymentEventRepo,
		orderHistoryRepo:   orderHistoryRepo,
		enrollmentRepo:     enrollmentRepo,
		courseRepo:         courseRepo,
		couponRepo:         couponRepo,
		voucherService:     voucherService,
		transactionService: transactionService,
	}
}

// completeOrderFulfillment (item 14, review vòng 1 — "gọi đúng logic tạo enrollment đang dùng
// ở CheckAndProcessPayment, tách thành hàm dùng chung, không copy"): tạo enrollment cho từng
// course trong đơn (bỏ qua nếu đã enroll), và ghi nhận usage voucher (nếu có). Dùng chung cho
// CheckAndProcessPayment (đơn trả phí, sau khi khớp giao dịch ngân hàng) VÀ nhánh đơn 0đ tự
// hoàn tất trong OrderService.CreateOrder.
//
// H2-04 (review vòng 3): TRƯỚC ĐÂY dùng GetByUserAndCourse (có scope, loại soft-delete) + Create
// vô điều kiện — mua lại một khóa đã Unenroll trước đó (bản ghi cũ vẫn còn, chỉ bị soft-delete)
// sẽ vi phạm unique index idx_user_course khi INSERT mới. Đồng thời hàm này KHÔNG hề gọi
// IncrementTotalStudents, khiến courses.total_students sai lệch với số enrollment thật cho MỌI
// đơn hàng đi qua CreateOrder/CheckAndProcessPayment. Sửa bằng cách TÁI DÙNG đúng logic
// EnrollmentService.Enroll (GetByUserAndCourseUnscoped + RestoreAndReactivate cho trường hợp
// re-enroll, Create cho trường hợp enroll lần đầu, IncrementTotalStudents đúng 1 lần cho cả hai
// nhánh) — không copy logic mới, chỉ viết lại inline vì đây là 1 hàm tự do (không phải method
// của EnrollmentService) nên không gọi thẳng EnrollmentService.Enroll được (Enroll còn có bước
// kiểm course.Price.IsZero() không áp dụng ở đây — item này được gọi CHÍNH XÁC vì order đã trả
// tiền/đơn 0đ, không cần kiểm lại giá).
//
// H2-05 (review vòng 3): KHÔNG còn gọi voucherService.IncrementUsedCount ở đây nữa — used_count
// giờ được "reserve" (tăng) ngay lúc TẠO đơn (OrderService.CreateOrder, trong cùng transaction —
// xem VoucherServiceInterface.ReserveVoucherUsage) để chặn oversell khi nhiều đơn "pending" tồn
// tại song song. Hàm này chỉ còn ghi VoucherLog (audit trail) khi fulfillment thành công.
func completeOrderFulfillment(
	ctx context.Context,
	enrollmentRepo repository.EnrollmentRepositoryInterface,
	courseRepo repository.CourseRepositoryInterface,
	voucherService VoucherServiceInterface,
	items []model.OrderItem,
	order *model.Order,
) error {
	for _, item := range items {
		existing, err := enrollmentRepo.GetByUserAndCourseUnscoped(ctx, order.UserID, item.CourseID)
		if err != nil {
			return fmt.Errorf("failed to check existing enrollment for course %s: %w", item.CourseID, err)
		}

		if existing != nil && !existing.DeletedAt.Valid {
			continue // Đã enroll (active) — bỏ qua, không tăng total_students lần 2.
		}

		if existing != nil && existing.DeletedAt.Valid {
			// Mua lại khóa đã Unenroll trước đó: khôi phục bản ghi cũ thay vì INSERT mới, tránh
			// vi phạm idx_user_course — khớp EnrollmentService.Enroll.
			now := time.Now()
			updates := map[string]interface{}{
				"enrolled_at":         now,
				"completed_at":        nil,
				"last_accessed_at":    nil,
				"progress_percentage": decimal.Zero,
			}
			if err := enrollmentRepo.RestoreAndReactivate(ctx, existing.ID, updates); err != nil {
				return fmt.Errorf("failed to restore enrollment for course %s: %w", item.CourseID, err)
			}
		} else {
			enrollment := &model.Enrollment{
				UserID:     order.UserID,
				CourseID:   item.CourseID,
				EnrolledAt: time.Now(),
			}
			if err := enrollmentRepo.Create(ctx, enrollment); err != nil {
				return fmt.Errorf("failed to create enrollment for course %s: %w", item.CourseID, err)
			}
		}

		if courseRepo != nil {
			if err := courseRepo.IncrementTotalStudents(ctx, item.CourseID, 1); err != nil {
				return fmt.Errorf("failed to increment total_students for course %s: %w", item.CourseID, err)
			}
		}
	}

	// item 24: voucher usage log (bảng vouchers) thay cho coupon usage (bảng coupons đã bỏ —
	// xem comment VoucherID/CouponID tại model.Order). used_count đã reserve lúc tạo đơn
	// (H2-05) — ở đây chỉ ghi log.
	if order.VoucherID != nil && voucherService != nil {
		if err := voucherService.RecordUsageLog(ctx, *order.VoucherID, order.UserID, order.ID, order.DiscountAmount); err != nil {
			return fmt.Errorf("failed to record voucher usage log: %w", err)
		}
	}

	return nil
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
	expiresAt := time.Now().Add(24 * time.Hour)

	// Update order to processing status
	oldStatus := order.Status
	// item 25 (review web vòng 1): lưu payment code + expiry vào order NGAY trong bước này —
	// xem comment tại OrderRepository.UpdatePaymentCode để biết lý do (trước đây chỉ set status,
	// mã thanh toán chỉ tồn tại trong response, không tra lại được).
	if err := s.orderRepo.UpdatePaymentCode(orderID, paymentCode, expiresAt); err != nil {
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
		ExpiredAt:   expiresAt,
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

	// M2-02 (review vòng 3): payment_code_expired_at TRƯỚC ĐÂY được LƯU (item 25, vòng 1) nhưng
	// KHÔNG BAO GIỜ được đọc lại để từ chối — người dùng vẫn có thể bấm "Tôi đã chuyển khoản"
	// (CheckPayment, gọi thẳng hàm này) hoặc poll GetPaymentStatus sau khi mã đã hết hạn và giao
	// dịch ngân hàng khớp muộn vẫn được xử lý bình thường. Chặn ngay khi phát hiện quá hạn:
	// chuyển đơn sang "expired" (hoàn lại used_count đã reserve nếu có voucher — H2-05) thay vì
	// tiếp tục gọi gRPC check giao dịch. "expired" đã là trạng thái web mong đợi (xem
	// web/src/services/order.service.ts OrderStatus + use-orders.ts PAYMENT_TERMINAL_STATUSES).
	if order.PaymentCodeExpiredAt != nil && time.Now().After(*order.PaymentCodeExpiredAt) {
		oldStatus := order.Status
		expireErr := s.orderRepo.WithTransaction(func(txRepo *repository.OrderRepository) error {
			if err := txRepo.UpdateStatus(order.ID, "expired"); err != nil {
				return err
			}
			history := &model.OrderStatusHistory{
				ID:         uuid.New(),
				CreatedAt:  time.Now(),
				OrderID:    order.ID,
				FromStatus: oldStatus,
				ToStatus:   "expired",
				Reason:     "Payment code expired",
			}
			orderHistoryRepoTx := repository.NewOrderStatusHistoryRepository(txRepo.TxDB())
			if err := orderHistoryRepoTx.Create(history); err != nil {
				return err
			}
			if order.VoucherID != nil && s.voucherService != nil {
				if err := s.voucherService.ReleaseVoucherUsage(ctx, txRepo.TxDB(), *order.VoucherID); err != nil {
					return err
				}
			}
			return nil
		})
		if expireErr != nil {
			return nil, expireErr
		}
		return &dto.PaymentStatusResponse{
			OrderID: orderID,
			Status:  "expired",
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
	//
	// item 14: logic tạo enrollment + ghi usage voucher tách thành completeOrderFulfillment
	// (dùng chung với nhánh đơn 0đ tự hoàn tất trong OrderService.CreateOrder).
	if s.enrollmentRepo != nil {
		if err := completeOrderFulfillment(ctx, s.enrollmentRepo, s.courseRepo, s.voucherService, items, order); err != nil {
			return nil, err
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
