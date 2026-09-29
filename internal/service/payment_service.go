package service

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"gorm.io/gorm"
	"study.com/v1/internal/dto"
	"study.com/v1/internal/grpc"
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
	// ErrPaymentVerificationPending (review #76 vòng 3): đơn có mã chuyển khoản chưa đối chiếu xong
	// với ngân hàng (đang trong ân hạn, hoặc ngân hàng lỗi/timeout). Không được đổi trạng thái đơn
	// (huỷ, mở phiên mới) khi chưa biết tiền đã về hay chưa. Handler trả 409 ERR_PAYMENT_VERIFYING.
	ErrPaymentVerificationPending = errors.New("Đơn hàng đang được đối chiếu thanh toán với ngân hàng. Vui lòng không chuyển khoản lại và thử lại sau ít phút.")
)

// paymentReconcileGracePeriod (review #76 vòng 3, quyết định chủ dự án 28/09): sau khi mã chuyển
// khoản hết hạn, ngân hàng có thể ghi có chậm. Trong khoảng này "not_found" CHƯA phải bằng chứng
// không có tiền: đơn giữ processing (reconciling), không huỷ được, không chốt expired. Nguồn DUY
// NHẤT của con số này.
const paymentReconcileGracePeriod = 30 * time.Minute

// bankLookupTimeout (review #76 vòng 3): hạn cho mỗi lần gọi gRPC CheckTransaction. Hết hạn xử lý
// như ngân hàng lỗi (chưa xác minh được), không treo request/goroutine theo dịch vụ ngân hàng.
const bankLookupTimeout = 10 * time.Second

// bankLookupDateSlack (review #76 vòng 3): service Python đổi timestamp sang NGÀY theo múi giờ của
// máy nó chạy (chưa rõ UTC hay giờ VN). Nới cửa sổ tra cứu thêm 1 ngày mỗi phía để giao dịch lúc
// 0h–7h giờ VN không rơi ra ngoài khi service chạy UTC.
const bankLookupDateSlack = 24 * time.Hour

type PaymentServiceInterface interface {
	CreatePaymentIntent(ctx context.Context, userID, orderID uuid.UUID, isAdmin bool, paymentMethod string) (*dto.PaymentIntentResponse, error)
	CheckAndProcessPayment(ctx context.Context, orderID, actorUserID uuid.UUID, isAdmin bool) (*dto.PaymentStatusResponse, error)
	GetPaymentStatus(ctx context.Context, orderID, actorUserID uuid.UUID, isAdmin bool) (*dto.PaymentStatusResponse, error)
}

type PaymentService struct {
	orderRepo repository.OrderRepositoryInterface
	// paymentEventRepo (M3-09, review vòng 3b/4; XÓA ở Minor, review vòng 4b/5): TRƯỚC ĐÂY giữ
	// lại vì chưa có quyết định rõ ràng ("có thể dành cho tính năng webhook/payment-event sắp
	// tới"). Grep lại lần nữa (2026-09-09, vòng 5 bổ sung) vẫn xác nhận 0 lần đọc — team-lead
	// quyết định XÓA hẳn thay vì tiếp tục giữ field chết. Nếu tính năng webhook/payment-event
	// thật sự cần đến sau này, thêm lại field + tham số constructor ở đúng thời điểm đó (YAGNI —
	// không giữ field rỗng "phòng khi cần").
	//
	// orderHistoryRepo (M3-08, bổ sung vòng 4): field này TRƯỚC ĐÂY chỉ dùng ở CreatePaymentIntent
	// (đã đổi sang orderHistoryRepoTx trong transaction — xem M3-08) nên gần như dead — NHƯNG có
	// công dụng THẬT MỚI ở đây: ghi order_status_history "fulfillment_failed" NGOÀI transaction
	// đã rollback khi completeOrderFulfillment lỗi (đánh đổi rollback, chỉ đạo team-lead vòng 4)
	// — chủ đích PHẢI dùng connection gốc (không phải tx vừa rollback) nên field bare này vẫn
	// cần thiết, KHÔNG xóa dù M3-09 gợi ý dọn field chết.
	orderHistoryRepo repository.OrderStatusHistoryRepositoryInterface
	// enrollmentRepo (item 14, dọn dẹp phụ khi tách completeOrderFulfillment dùng chung):
	// TRƯỚC ĐÂY khai kiểu interface{} rồi type-assert bằng interface ẩn danh mỗi lần dùng
	// (xem git blame CheckAndProcessPayment cũ) — không cần thiết vì repos.Enrollment luôn
	// implement đúng repository.EnrollmentRepositoryInterface (xem app/services.go). Đổi
	// sang kiểu cụ thể để completeOrderFulfillment (dùng chung với OrderService.CreateOrder,
	// đơn 0đ) không phải type-assert lại.
	enrollmentRepo     repository.EnrollmentRepositoryInterface
	voucherService     VoucherServiceInterface
	transactionService TransactionServiceInterface
	// platformSettingRepo (tính năng đơn hàng+hoàn tiền+doanh thu, quyết định #2): đọc % phí nền
	// tảng hiện hành để CHỐT vào đơn ngay lúc chuyển "completed" — xem chốt fee trong
	// CheckAndProcessPayment. nil-safe (một số test dựng PaymentService không cần fee) — coi như
	// phí 0% nếu không tiêm.
	platformSettingRepo repository.PlatformSettingRepositoryInterface
}

// M3-09 (review vòng 3b, bổ sung vòng 4; Minor vòng 4b/5 xóa nốt paymentEventRepo): TRƯỚC ĐÂY
// NewPaymentService còn nhận courseRepo/orderItemRepo/couponRepo — cả 3 đã 0 lần được đọc qua
// field bare (grep xác nhận): courseRepo vì completeOrderFulfillment luôn dùng courseRepoTx dựng
// mới từ txDB (H2-06, vòng 3), không phải field s.courseRepo; orderItemRepo/couponRepo tương tự
// đã chuyển hẳn sang orderItemRepoTx (tx-bound) và flow voucher (couponRepo chưa từng dùng ở
// PaymentService, chỉ khai theo interface cũ). paymentEventRepo xóa SAU (vòng 5 bổ sung, quyết
// định team-lead) — cùng lý do 0 lần đọc, chỉ khác là ban đầu giữ lại chờ quyết định riêng.
func NewPaymentService(
	orderRepo repository.OrderRepositoryInterface,
	orderHistoryRepo repository.OrderStatusHistoryRepositoryInterface,
	enrollmentRepo repository.EnrollmentRepositoryInterface,
	voucherService VoucherServiceInterface,
	transactionService TransactionServiceInterface,
	platformSettingRepo repository.PlatformSettingRepositoryInterface,
) *PaymentService {
	return &PaymentService{
		orderRepo:           orderRepo,
		orderHistoryRepo:    orderHistoryRepo,
		enrollmentRepo:      enrollmentRepo,
		voucherService:      voucherService,
		transactionService:  transactionService,
		platformSettingRepo: platformSettingRepo,
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
	// tx (H2-06, review vòng 3b): *gorm.DB của transaction caller đang mở (CreateOrder nhánh 0đ,
	// CheckAndProcessPayment nhánh trả phí) — truyền xuống RecordUsageLogTx để voucher log tham
	// gia CÙNG transaction với enrollment/total_students. nil nếu caller không có transaction
	// đang mở (hiện không còn call site nào như vậy, nhưng giữ nil-safe cho tương lai).
	tx *gorm.DB,
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
		if err := voucherService.RecordUsageLogTx(ctx, tx, *order.VoucherID, order.UserID, order.ID, order.DiscountAmount); err != nil {
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

	// B-03 (review vòng 5): đơn đã "processing" NHƯNG payment code hiện tại CÒN HẠN — trả lại
	// đúng code CŨ thay vì sinh mã MỚI. TRƯỚC ĐÂY guard "order.Status != pending" chặn HẲN
	// nhánh này (trả ErrInvalidStateTransition) — user bấm "thanh toán" 2 lần liên tiếp (double-
	// click, hoặc mở 2 tab) hoặc app crash rồi mở lại trang thanh toán sẽ bị lỗi dù payment
	// intent vẫn còn dùng được, phải quay lại giỏ hàng tạo đơn mới hoàn toàn không cần thiết.
	// Idempotent theo đúng ý nghĩa: cùng orderID, cùng trạng thái "đang chờ thanh toán còn hạn"
	// -> trả về CÙNG payment intent, không tạo thêm bản ghi/mã nào mới.
	if order.Status == "processing" && order.PaymentTransactionID != nil && *order.PaymentTransactionID != "" &&
		order.PaymentCodeExpiredAt != nil && time.Now().Before(*order.PaymentCodeExpiredAt) {
		return s.buildPaymentIntentResponse(order, *order.PaymentTransactionID, *order.PaymentCodeExpiredAt, paymentMethod), nil
	}

	// Review #76 MAJOR 2: TRƯỚC ĐÂY chỉ kiểm status, nên đơn pending bị bỏ rơi quá hạn giữ vẫn mở
	// được phiên thanh toán bất cứ lúc nào và còn được cấp thêm 24h (lazy-sweep chỉ chạy khi user
	// tạo đơn MỚI). Giờ server thực thi đúng mốc đang hiển thị (orderHoldExpiresAt): quá hạn thì
	// chuyển "expired" (hoàn used_count voucher nếu có) và trả ErrOrderExpired, không cấp mã mới.
	// Đơn đã "expired" từ trước cũng trả ErrOrderExpired để web báo cùng một câu.
	if order.Status == "expired" {
		return nil, ErrOrderExpired
	}
	// Review vòng 3 MINOR 1: đơn đã thanh toán báo đúng là đã thanh toán (web chuyển màn thành công),
	// không phải "hết hạn" kèm nút tạo đơn mới.
	if order.Status == "completed" {
		return nil, ErrPaymentAlreadyDone
	}
	// Re-review #76 vòng 2: đơn processing có mã đã hết hạn thì KHÔNG tự chốt expired ở đây (có thể
	// tiền đã về trong hạn). Đi qua đúng bước đối chiếu gRPC lần cuối của CheckAndProcessPayment rồi
	// ánh xạ ĐÚNG kết quả (review vòng 3 MINOR 1): đã thanh toán → ErrPaymentAlreadyDone; chưa đối
	// chiếu xong (ân hạn / ngân hàng lỗi) → ErrPaymentVerificationPending; chỉ expired → ErrOrderExpired.
	if order.Status == "processing" && order.PaymentCodeExpiredAt != nil && time.Now().After(*order.PaymentCodeExpiredAt) {
		status, err := s.CheckAndProcessPayment(ctx, orderID, userID, isAdmin)
		if errors.Is(err, ErrPaymentAlreadyDone) || (err == nil && status.Status == "completed") {
			return nil, ErrPaymentAlreadyDone
		}
		if err != nil {
			return nil, err
		}
		if status.Status == "expired" {
			return nil, ErrOrderExpired
		}
		return nil, ErrPaymentVerificationPending
	}
	if expired, err := s.expireIfHoldElapsed(ctx, order); err != nil {
		return nil, err
	} else if expired {
		return nil, ErrOrderExpired
	}

	// Verify order is in correct state — "pending" là nhánh DUY NHẤT còn lại được phép tạo intent
	// MỚI (processing với code CÒN HẠN đã trả ở nhánh trên; processing với code HẾT HẠN hoặc
	// KHÔNG có code, cancelled/expired/completed/failed đều rơi vào đây -> lỗi, đúng ý "đã
	// cancelled/expired -> lỗi" của B-03).
	if order.Status != "pending" {
		return nil, ErrInvalidStateTransition
	}

	// Generate payment code
	paymentCode := s.generatePaymentCode()
	// Quyết định chủ dự án 28/09: hạn mã = min(now+24h, hạn giữ đơn). Trước đây luôn now+24h nên
	// đơn mở thanh toán sát hạn được giữ tới ~48h kể từ lúc tạo. Web đếm ngược theo expired_at
	// trả về ở đây nên tự hiển thị đúng mốc này.
	expiresAt := time.Now().Add(pendingOrderDefaultTTL)
	if holdExpiresAt := orderHoldExpiresAt(order); holdExpiresAt != nil && holdExpiresAt.Before(expiresAt) {
		expiresAt = *holdExpiresAt
	}

	// Update order to processing status
	oldStatus := order.Status
	// item 25 (review web vòng 1): lưu payment code + expiry vào order NGAY trong bước này —
	// xem comment tại OrderRepository.UpdatePaymentCode để biết lý do (trước đây chỉ set status,
	// mã thanh toán chỉ tồn tại trong response, không tra lại được).
	//
	// M3-08 (review vòng 3b, bổ sung vòng 4): TRƯỚC ĐÂY UpdatePaymentCode và history.Create là
	// 2 lệnh ghi RỜI RẠC — history.Create() còn KHÔNG kiểm lỗi trả về (không cả "_ ="), nên nếu
	// ghi history lỗi giữa chừng, order đã chuyển "processing" (mã thanh toán đã lưu) nhưng
	// order_status_history thiếu bản ghi audit trail, không ai biết đơn "processing" từ đâu. Gộp
	// vào MỘT transaction: lỗi ở bước nào rollback CẢ HAI, không để order "processing" mồ côi
	// history.
	err = s.orderRepo.WithTransaction(func(txRepo *repository.OrderRepository) error {
		orderHistoryRepoTx := repository.NewOrderStatusHistoryRepository(txRepo.TxDB())
		return updatePaymentCodeAndHistoryTx(txRepo, orderHistoryRepoTx, orderID, paymentCode, expiresAt, oldStatus)
	})
	if err != nil {
		return nil, err
	}

	return s.buildPaymentIntentResponse(order, paymentCode, expiresAt, paymentMethod), nil
}

// buildPaymentIntentResponse (B-03, review vòng 5) — tách phần dựng response ra khỏi
// CreatePaymentIntent để dùng CHUNG cho cả 2 nhánh: tạo intent MỚI (paymentCode vừa sinh) và trả
// lại intent CŨ còn hạn (paymentCode đọc từ order.PaymentTransactionID) — không copy lại logic
// QR/bank-transfer 2 lần.
func (s *PaymentService) buildPaymentIntentResponse(order *model.Order, paymentCode string, expiresAt time.Time, paymentMethod string) *dto.PaymentIntentResponse {
	resp := &dto.PaymentIntentResponse{
		OrderID:     order.ID,
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

	return resp
}

// paymentCodeUpdater — interface HẸP, chỉ đúng 1 method updatePaymentCodeAndHistoryTx cần từ
// txRepo. *repository.OrderRepository implement interface này tự nhiên (Go structural typing),
// không cần đổi gì ở call site thật (CreatePaymentIntent vẫn truyền thẳng *repository.OrderRepository
// từ WithTransaction). Lý do tách interface riêng (B-03, review vòng 5 — sửa SAU khi B-02 đã
// viết xong): UpdatePaymentCode đổi thành UPDATE CÓ ĐIỀU KIỆN (buildUpdatePaymentCodeQuery) —
// trên DryRun DB, RowsAffected LUÔN = 0 (không có kết nối thật để Exec), nên gọi
// txRepo.UpdatePaymentCode(...) THẬT trên DryRun giờ LUÔN trả ErrOrderConflict, làm hỏng test
// B-02 (không bao giờ chạm tới bước ghi history được nữa để test riêng nhánh đó). Tách interface
// hẹp để fake được BƯỚC NÀY độc lập — hành vi CONDITIONAL UPDATE thật của UpdatePaymentCode đã
// có test riêng (TestUpdatePaymentCode_ConditionalGuard/_ReturnsErrOrderConflictOnZeroRows,
// order_repository_test.go), không cần lặp lại ở đây.
type paymentCodeUpdater interface {
	UpdatePaymentCode(orderID uuid.UUID, paymentCode string, expiredAt time.Time) error
}

// updatePaymentCodeAndHistoryTx (B-02, review vòng 5) — tách THÂN CLOSURE của
// CreatePaymentIntent's WithTransaction ra hàm riêng, NHẬN interface
// repository.OrderStatusHistoryRepositoryInterface cho orderHistoryRepo (thay vì tự dựng
// repository.NewOrderStatusHistoryRepository(txRepo.TxDB()) BÊN TRONG closure như trước) để test
// bằng fake — mô phỏng "ghi history lỗi -> hàm phải trả lỗi, KHÔNG nuốt" mà không cần DB thật.
func updatePaymentCodeAndHistoryTx(txRepo paymentCodeUpdater, orderHistoryRepo repository.OrderStatusHistoryRepositoryInterface, orderID uuid.UUID, paymentCode string, expiresAt time.Time, oldStatus string) error {
	if err := txRepo.UpdatePaymentCode(orderID, paymentCode, expiresAt); err != nil {
		return err
	}
	history := &model.OrderStatusHistory{
		ID:         uuid.New(),
		CreatedAt:  time.Now(),
		OrderID:    orderID,
		FromStatus: oldStatus,
		ToStatus:   "processing",
		Reason:     "Payment initiated",
	}
	return orderHistoryRepo.Create(history)
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

	// Review #76 vòng 4 (thiết kế thống nhất, chủ dự án chốt): mọi đơn ĐÃ CẤP MÃ chuyển khoản, kể cả
	// đã expired/cancelled, đều đi qua đúng một bộ quyết định reconcileIssuedOrder (payment_reconcile.go)
	// với cửa sổ tra cứu cố định của chính đơn đó. Xem bảng trạng thái trong body PR #76.
	switch order.Status {
	case "completed":
		return &dto.PaymentStatusResponse{OrderID: orderID, Status: order.Status, PaidAt: order.PaidAt, Amount: order.TotalAmount}, nil
	case "pending":
		// Đơn "pending" chưa mở phiên thanh toán nên chưa có mã, không thể có tiền về: quá hạn giữ
		// (orderHoldExpiresAt, review #76 MAJOR 2) thì chốt "expired" ngay, không cần gRPC.
		if expired, err := s.expireIfHoldElapsed(ctx, order); err != nil {
			return nil, err
		} else if expired {
			return &dto.PaymentStatusResponse{OrderID: orderID, Status: "expired", Amount: order.TotalAmount}, nil
		}
	case "processing":
		if !hasPaymentCode(order) {
			return nil, errors.New("payment code not found")
		}
		return s.reconcileIssuedOrder(ctx, order)
	case "expired", "cancelled":
		if hasPaymentCode(order) {
			return s.reconcileIssuedOrder(ctx, order)
		}
	}
	return &dto.PaymentStatusResponse{OrderID: orderID, Status: order.Status, Amount: order.TotalAmount}, nil
}

// recordFulfillmentFailureAlert (vòng 4b, chỉ đạo team-lead — tách ra từ CheckAndProcessPayment
// để test được không cần DB thật): gọi khi nhánh TRẢ PHÍ của CheckAndProcessPayment rollback vì
// completeOrderFulfillment lỗi, SAU KHI đã loại trừ ErrPaymentAlreadyDone (benign, không alert).
// Tại thời điểm này giao dịch ngân hàng ĐÃ được amount-match nhưng KHÔNG còn dấu vết nào trong DB
// (transaction rollback hết) — log "[PAYMENT-ALERT]" cố định prefix để ops grep + ghi 1 dòng
// order_status_history "fulfillment_failed" NGOÀI transaction đã rollback (best-effort, dùng
// connection gốc s.orderHistoryRepo). Lỗi ở chính bước ghi fallback này chỉ log thêm, KHÔNG che
// lỗi gốc (fulfillErr vẫn được caller trả về nguyên vẹn).
// shouldAlertFulfillmentFailure (B-01, review vòng 4b/5) — tách riêng phần QUYẾT ĐỊNH "có nên
// cảnh báo ops hay không" thành 1 hàm THUẦN (pure — chỉ nhận error, trả bool, không side-effect)
// khỏi nhánh `if err != nil` của CheckAndProcessPayment — trước đây quyết định này ẩn trong 1
// điều kiện if lồng trực tiếp trong hàm lớn, không tách được ra để test độc lập. Benign DUY NHẤT:
// ErrPaymentAlreadyDone (unique constraint bank_transaction_usages, M-06 — một request KHÁC đã
// xử lý ĐÚNG giao dịch ngân hàng này rồi, không phải "tiền mất dấu vết"). MỌI lỗi khác (thường
// gặp nhất: completeOrderFulfillment lỗi giữa chừng — enrollment/total_students/voucher log)
// đều PHẢI cảnh báo, vì giao dịch ngân hàng đã amount-match THẬT nhưng transaction rollback hết.
func shouldAlertFulfillmentFailure(err error) bool {
	if err == nil {
		return false
	}
	return !errors.Is(err, ErrPaymentAlreadyDone)
}

func (s *PaymentService) recordFulfillmentFailureAlert(orderID uuid.UUID, oldStatus, bankTxID, amount string, fulfillErr error) {
	log.Printf("[PAYMENT-ALERT] order=%s tx=%s amount=%s err=%v", orderID, bankTxID, amount, fulfillErr)
	fallbackHistory := &model.OrderStatusHistory{
		ID:         uuid.New(),
		CreatedAt:  time.Now(),
		OrderID:    orderID,
		FromStatus: oldStatus,
		ToStatus:   "fulfillment_failed",
		Reason: fmt.Sprintf(
			"Payment transaction %s received (amount %s matched) but order fulfillment failed and the whole transaction rolled back — needs manual reconciliation: %v",
			bankTxID, amount, fulfillErr),
	}
	if histErr := s.orderHistoryRepo.Create(fallbackHistory); histErr != nil {
		log.Printf("[PAYMENT-ALERT] order=%s tx=%s failed to write fallback fulfillment_failed history: %v", orderID, bankTxID, histErr)
	}
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

	// Đơn đang "processing": tranh thủ đối chiếu giao dịch ngân hàng. Smoke test 11/09: khi dịch
	// vụ gRPC ngân hàng không chạy, route này TRẢ LỖI thay vì trạng thái — web poll 5s/lần sẽ hiện
	// lỗi liên tục dù đơn vẫn bình thường. Lỗi đối chiếu chỉ ghi log và trả trạng thái hiện tại;
	// riêng lỗi quyền/không tìm thấy vẫn trả về như cũ.
	// Review #76 MAJOR 2: "pending" cũng đi qua CheckAndProcessPayment để đơn quá hạn giữ được chốt
	// "expired" ngay khi web poll, cùng mốc với CreatePaymentIntent (không gọi gRPC cho pending).
	// Re-review #76 vòng 2/3: đơn đã đóng (expired/cancelled) cũng đi qua để trả cờ
	// late_payment_received (history, hoặc tra ngân hàng lại cho đơn từng có mã).
	if order.Status == "processing" || order.Status == "pending" || order.Status == "expired" || order.Status == "cancelled" {
		resp, checkErr := s.CheckAndProcessPayment(ctx, orderID, actorUserID, isAdmin)
		if checkErr == nil {
			return resp, nil
		}
		if errors.Is(checkErr, ErrOrderForbidden) || errors.Is(checkErr, ErrOrderNotFound) {
			return nil, checkErr
		}
		log.Printf("[PAYMENT-STATUS] order=%s đối chiếu giao dịch lỗi, trả trạng thái hiện tại: %v", orderID, checkErr)
		if refreshed, rerr := s.orderRepo.GetByID(orderID); rerr == nil {
			order = refreshed
		}
	}

	return &dto.PaymentStatusResponse{
		OrderID: orderID,
		Status:  order.Status,
		PaidAt:  order.PaidAt,
		Amount:  order.TotalAmount,
	}, nil
}

// expireIfHoldElapsed (review #76 MAJOR 2) — nguồn DUY NHẤT cho quyết định "đơn còn mở đã quá hạn
// giữ chưa", dùng chung cho CreatePaymentIntent, CheckAndProcessPayment (và GetPaymentStatus qua
// nó). Mốc là orderHoldExpiresAt, tức đúng giá trị expires_at web đang hiển thị: pending chưa mở
// phiên thì created_at + 24h, đã có mã thì payment_code_expired_at. Quá hạn thì chuyển "expired"
// qua releaseOrderAndTransition (UPDATE có điều kiện + hoàn used_count voucher). Trả true cả khi
// request khác vừa chuyển trạng thái trước (applied=false): mốc đã qua nên đơn không còn dùng được,
// caller vẫn phải từ chối.
//
// Re-review #76 vòng 2: CHỈ áp cho đơn "pending" (chưa có mã chuyển khoản nên không thể có tiền
// về). Đơn "processing" có mã phải qua đối chiếu gRPC lần cuối trong CheckAndProcessPayment, không
// bao giờ chốt expired ở đây.
func (s *PaymentService) expireIfHoldElapsed(ctx context.Context, order *model.Order) (bool, error) {
	if order.Status != "pending" {
		return false, nil
	}
	expiresAt := orderHoldExpiresAt(order)
	if expiresAt == nil || !time.Now().After(*expiresAt) {
		return false, nil
	}
	return s.expireOrderTx(ctx, order, "Order hold expired before payment was initiated")
}

// expireOrderTx chuyển order sang "expired" qua releaseOrderAndTransition (UPDATE có điều kiện +
// hoàn used_count voucher đúng 1 lần). Re-review #76 vòng 2 (MINOR): khi applied=false (request
// khác vừa đổi trạng thái, vd vừa hoàn tất thanh toán) thì ĐỌC LẠI và cập nhật order.Status theo
// trạng thái thật, trả expired=true chỉ khi đơn thật sự đang "expired" — trước đây vẫn báo
// "expired" cho đơn vừa được thanh toán xong.
func (s *PaymentService) expireOrderTx(ctx context.Context, order *model.Order, reason string) (bool, error) {
	var applied bool
	err := s.orderRepo.WithTransaction(func(txRepo *repository.OrderRepository) error {
		var txErr error
		applied, txErr = releaseOrderAndTransition(ctx, txRepo, s.voucherService, order, []string{order.Status}, "expired", reason)
		return txErr
	})
	if err != nil {
		return false, err
	}
	if applied {
		order.Status = "expired"
		return true, nil
	}
	fresh, err := s.orderRepo.GetByID(order.ID)
	if err != nil {
		return false, err
	}
	order.Status = fresh.Status
	order.PaidAt = fresh.PaidAt
	return fresh.Status == "expired", nil
}

// latePaymentHistoryStatus — to_status của dòng history đánh dấu "đã nhận tiền nhưng không hoàn
// tất được đơn" (về sau hạn mã / sai số tiền / không đọc được ngày giao dịch). Admin lọc theo giá
// trị này để hoàn tiền; varchar(20) nên giữ đúng độ dài hiện tại.
const latePaymentHistoryStatus = "payment_after_expiry"

// expireWithLatePayment (re-review #76 vòng 2): có giao dịch ngân hàng cho mã này nhưng không đủ
// điều kiện hoàn tất (về SAU hạn mã, sai số tiền, hoặc không đọc được ngày). Vẫn chốt "expired"
// (quyết định: đơn quá hạn không thanh toán được), nhưng KHÔNG im lặng: log [PAYMENT-ALERT] + ghi
// history payment_after_expiry CÙNG transaction với lần chuyển trạng thái để admin hoàn tiền.
func (s *PaymentService) expireWithLatePayment(ctx context.Context, order *model.Order, result *grpc.CheckTransactionResult, paidAt time.Time, dateKnown, amountMatches bool) (*dto.PaymentStatusResponse, error) {
	when := result.TransactionDate
	if dateKnown {
		when = paidAt.Format(time.RFC3339)
	}
	codeExpiry := ""
	if order.PaymentCodeExpiredAt != nil {
		codeExpiry = order.PaymentCodeExpiredAt.Format(time.RFC3339)
	}
	note := fmt.Sprintf("Received bank transaction %s amount %s at %s but payment code expired at %s (amount matches order total %s: %t, date parsed: %t). Refund manually.",
		result.TransactionID, result.Amount, when, codeExpiry, order.TotalAmount.String(), amountMatches, dateKnown)

	fromStatus := order.Status
	var applied bool
	err := s.orderRepo.WithTransaction(func(txRepo *repository.OrderRepository) error {
		var txErr error
		applied, txErr = releaseOrderAndTransition(ctx, txRepo, s.voucherService, order, []string{fromStatus}, "expired", "Payment code expired")
		if txErr != nil || !applied {
			return txErr
		}
		return repository.NewOrderStatusHistoryRepository(txRepo.TxDB()).Create(&model.OrderStatusHistory{
			ID:         uuid.New(),
			CreatedAt:  time.Now(),
			OrderID:    order.ID,
			FromStatus: "expired",
			ToStatus:   latePaymentHistoryStatus,
			Reason:     note,
		})
	})
	if err != nil {
		return nil, err
	}
	if !applied {
		// Request khác đã đổi trạng thái trước (vd lần kiểm song song đã ghi cảnh báo này).
		fresh, rerr := s.orderRepo.GetByID(order.ID)
		if rerr != nil {
			return nil, rerr
		}
		return &dto.PaymentStatusResponse{
			OrderID:             order.ID,
			Status:              fresh.Status,
			PaidAt:              fresh.PaidAt,
			Amount:              fresh.TotalAmount,
			LatePaymentReceived: fresh.Status == "expired" && s.hasLatePaymentRecord(order.ID),
		}, nil
	}
	log.Printf("[PAYMENT-LATE-REFUND-NEEDED] order=%s amount=%s tx=%s paid_at=%s code_expired_at=%s: nhận tiền nhưng không hoàn tất được đơn, cần hoàn tiền thủ công",
		order.ID, result.Amount, result.TransactionID, when, codeExpiry)
	order.Status = "expired"
	return &dto.PaymentStatusResponse{OrderID: order.ID, Status: "expired", Amount: order.TotalAmount, LatePaymentReceived: true}, nil
}

// hasLatePaymentRecord — đơn từng được ghi history payment_after_expiry chưa. nil-safe: một số
// test dựng PaymentService không có orderHistoryRepo.
func (s *PaymentService) hasLatePaymentRecord(orderID uuid.UUID) bool {
	if s.orderHistoryRepo == nil {
		return false
	}
	rows, err := s.orderHistoryRepo.GetByOrderID(orderID)
	if err != nil {
		log.Printf("[PAYMENT-STATUS] order=%s không đọc được history: %v", orderID, err)
		return false
	}
	for _, h := range rows {
		if h.ToStatus == latePaymentHistoryStatus {
			return true
		}
	}
	return false
}

// ReconcileBeforeCancel (review #76 vòng 3 MAJOR 3, quyết định chủ dự án): huỷ đơn processing có mã
// chuyển khoản phải đối chiếu ngân hàng trước. nil = được phép huỷ (đơn không có mã, hoặc ngân hàng
// xác nhận không có tiền khi mã còn hạn, hoặc đơn vừa chuyển sang trạng thái khác — caller đọc lại).
// ErrPaymentAlreadyDone = tiền khớp, đơn đã hoàn tất. ErrPaymentVerificationPending = ngân hàng
// lỗi/timeout, đang trong ân hạn, hoặc có giao dịch nhưng sai số tiền (cần hỗ trợ xử lý).
func (s *PaymentService) ReconcileBeforeCancel(ctx context.Context, orderID uuid.UUID) error {
	order, err := s.orderRepo.GetByID(orderID)
	if err != nil {
		return ErrOrderNotFound
	}
	if order.Status != "processing" || order.PaymentTransactionID == nil || *order.PaymentTransactionID == "" {
		return nil
	}
	status, err := s.CheckAndProcessPayment(ctx, orderID, order.UserID, true)
	if errors.Is(err, ErrPaymentAlreadyDone) {
		return ErrPaymentAlreadyDone
	}
	if err != nil {
		log.Printf("[PAYMENT-CHECK] order=%s không huỷ vì đối chiếu lỗi/có giao dịch bất thường: %v", orderID, err)
		return ErrPaymentVerificationPending
	}
	if status.Status == "completed" {
		return ErrPaymentAlreadyDone
	}
	if status.Reconciling {
		return ErrPaymentVerificationPending
	}
	return nil
}

// parseBankTransactionDate đọc transactionDate của service Python (mbbank: "dd/MM/yyyy HH:mm:ss").
// ok=false khi rỗng/không đọc được: caller KHÔNG được coi là "trong hạn".
func parseBankTransactionDate(s string) (time.Time, bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return time.Time{}, false
	}
	if t, err := time.Parse(time.RFC3339, s); err == nil {
		return t, true
	}
	for _, layout := range []string{"02/01/2006 15:04:05", "02/01/2006 15:04", "2006-01-02 15:04:05", "2006-01-02T15:04:05"} {
		// MB ghi transactionDate theo giờ Việt Nam, không kèm múi giờ (bankTimeZone, payment_reconcile.go).
		if t, err := time.ParseInLocation(layout, s, bankTimeZone()); err == nil {
			return t, true
		}
	}
	return time.Time{}, false
}

// calculatePlatformFeeAmount (quyết định #2, 27/09/2026) — hàm THUẦN (không side-effect), tách
// riêng để unit test không cần DB: số tiền phí nền tảng = totalAmount * percent / 100, làm tròn 2
// chữ số thập phân (đơn vị đồng — cùng độ chính xác decimal(12,2) của total_amount, xem
// model.Order.PlatformFeeAmount). percent=0 (mặc định trước khi cấu hình) luôn cho fee=0.
func calculatePlatformFeeAmount(totalAmount, feePercent decimal.Decimal) decimal.Decimal {
	return totalAmount.Mul(feePercent).Div(decimal.NewFromInt(100)).Round(2)
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
