package service

// Review #76 — thiết kế đối chiếu thống nhất (vòng 4) + chính sách tiền chủ dự án chốt sau review
// final (29/09/2026). Mọi đơn ĐÃ CẤP MÃ chuyển khoản đi qua đúng một hàm reconcileIssuedOrder:
//
//   A. Đơn đã cấp mã (processing/expired/cancelled) luôn được đối chiếu trong cửa sổ tra cứu CỐ ĐỊNH
//      của nó. Chỉ đơn processing được hoàn tất (tiền khớp về trong hạn). QUYẾT ĐỊNH 1: KHÔNG BAO
//      GIỜ khôi phục đơn đã cancelled/expired — có giao dịch cho đơn đã đóng (trong hay sau hạn, đúng
//      hay sai số tiền) thì không ghi danh, không dùng voucher, không đổi trạng thái; chỉ gắn cờ CẦN
//      HOÀN TIỀN (history payment_after_expiry, đúng 1 lần) + log [PAYMENT-LATE-REFUND-NEEDED]. Lý
//      do: học viên có thể đã mua lại bằng đơn khác, khôi phục tạo 2 đơn completed cho 1 khoá.
//      Không đơn nào bị coi là "không có tiền" khi chưa qua ân hạn và chưa xác minh được.
//   B. Cửa sổ cố định, QUYẾT ĐỊNH 2 (không phụ thuộc múi giờ server gRPC): mọi mốc tính theo giờ VN.
//      from = đầu ngày VN của (lúc cấp mã - 1 ngày); to = cuối ngày VN (23:59:59) của
//      min(hạn mã + ân hạn + 1 ngày, hôm nay). Cửa sổ không dài thêm theo thời gian.
//   C. Hoàn tất đọc lại trạng thái DƯỚI KHOÁ DÒNG (SELECT ... FOR UPDATE): đơn vừa bị huỷ/chốt thì
//      không hoàn tất mà gắn cờ hoàn tiền; hoàn tất thắng thì huỷ đọc lại và báo đã thanh toán.
//   D. Không kẹt vĩnh viễn: đơn processing quá hạn mã + unverifiedExpiryAfter mà ngân hàng vẫn lỗi
//      → expired + history unverified_expiry + [PAYMENT-ALERT]. Tiền về sau đó: gắn cờ hoàn tiền (A).

import (
	"context"
	"errors"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"study.com/v1/internal/dto"
	"study.com/v1/internal/grpc"
	"study.com/v1/internal/model"
	"study.com/v1/internal/repository"
)

// unverifiedExpiryAfter (vòng 4 D): quá hạn mã chừng này mà ngân hàng vẫn lỗi/timeout thì chốt
// expired để học viên không bị khoá mua lại khoá học mãi; admin đối soát tay theo history.
const unverifiedExpiryAfter = 24 * time.Hour

// unverifiedExpiryHistoryStatus — to_status của history đánh dấu "chốt expired khi CHƯA xác minh
// được với ngân hàng". varchar(20).
const unverifiedExpiryHistoryStatus = "unverified_expiry"

// bankTimeZoneName — múi giờ của mọi mốc ngày gửi sang gRPC và của transactionDate MB trả về.
const bankTimeZoneName = "Asia/Ho_Chi_Minh"

var (
	bankTZOnce sync.Once
	bankTZ     *time.Location
)

// bankTimeZone — Asia/Ho_Chi_Minh, nạp 1 lần. Máy thiếu tzdata (Windows không cài Go tzdata,
// container tối giản) thì fallback FixedZone +07 (VN không có giờ mùa hè) và log 1 lần, không lỗi.
func bankTimeZone() *time.Location {
	bankTZOnce.Do(func() { bankTZ = resolveBankTimeZone(time.LoadLocation) })
	return bankTZ
}

func resolveBankTimeZone(load func(string) (*time.Location, error)) *time.Location {
	loc, err := load(bankTimeZoneName)
	if err != nil {
		log.Printf("[PAYMENT-TZ] không nạp được %s (%v), dùng FixedZone ICT +07", bankTimeZoneName, err)
		return time.FixedZone("ICT", 7*60*60)
	}
	return loc
}

func startOfBankDay(t time.Time) time.Time {
	loc := bankTimeZone()
	t = t.In(loc)
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, loc)
}

func endOfBankDay(t time.Time) time.Time {
	loc := bankTimeZone()
	t = t.In(loc)
	return time.Date(t.Year(), t.Month(), t.Day(), 23, 59, 59, 0, loc)
}

// paymentCodeDeadline — hạn mã của đơn đã cấp mã. Đơn cũ (trước khi có cột payment_code_expired_at)
// lấy created_at + 24h, đúng mốc mặc định lúc cấp.
func paymentCodeDeadline(order *model.Order) time.Time {
	if order.PaymentCodeExpiredAt != nil {
		return *order.PaymentCodeExpiredAt
	}
	return order.CreatedAt.Add(pendingOrderDefaultTTL)
}

// bankLookupWindow — cửa sổ tra cứu CỐ ĐỊNH của một đơn (B), theo NGÀY giờ VN. Lúc cấp mã >=
// created_at và >= hạn mã - 24h (hạn mã = min(lúc cấp + 24h, hạn giữ đơn)). to là 23:59:59 giờ VN nên
// ngày UTC của nó trùng ngày VN: server gRPC chạy UTC hay giờ VN đều đổi ra đúng ngày, và không bao
// giờ là ngày tương lai theo giờ VN.
func bankLookupWindow(order *model.Order, now time.Time) (from, to time.Time) {
	deadline := paymentCodeDeadline(order)
	issued := deadline.Add(-pendingOrderDefaultTTL)
	if order.CreatedAt.After(issued) {
		issued = order.CreatedAt
	}
	from = startOfBankDay(issued.Add(-bankLookupDateSlack))
	last := deadline.Add(paymentReconcileGracePeriod + bankLookupDateSlack)
	if last.After(now) {
		last = now
	}
	return from, endOfBankDay(last)
}

// lookupBankTransaction — lời gọi gRPC CheckTransaction DUY NHẤT của PaymentService: cửa sổ cố
// định của đơn (bankLookupWindow) và timeout bankLookupTimeout. Hết timeout xử lý như ngân hàng lỗi.
func (s *PaymentService) lookupBankTransaction(ctx context.Context, order *model.Order, now time.Time) (*grpc.CheckTransactionResult, error) {
	if s.transactionService == nil {
		return nil, errors.New("transaction service unavailable")
	}
	fromTime, toTime := bankLookupWindow(order, now)
	lookupCtx, cancel := context.WithTimeout(ctx, bankLookupTimeout)
	defer cancel()
	return s.transactionService.CheckTransaction(lookupCtx, *order.PaymentTransactionID, fromTime, toTime)
}

// reconcileIssuedOrder — bộ quyết định DUY NHẤT cho đơn đã cấp mã (processing/expired/cancelled).
// Caller bảo đảm hasPaymentCode(order).
func (s *PaymentService) reconcileIssuedOrder(ctx context.Context, order *model.Order) (*dto.PaymentStatusResponse, error) {
	now := time.Now()
	deadline := paymentCodeDeadline(order)
	codeExpired := now.After(deadline)
	processing := order.Status == "processing"
	current := func(extra func(*dto.PaymentStatusResponse)) *dto.PaymentStatusResponse {
		r := &dto.PaymentStatusResponse{OrderID: order.ID, Status: order.Status, PaidAt: order.PaidAt, Amount: order.TotalAmount}
		if extra != nil {
			extra(r)
		}
		return r
	}

	// Đơn đã đóng đã gắn cờ cần hoàn tiền: đã cảnh báo đúng 1 lần, không gọi ngân hàng lại.
	if !processing && s.hasLatePaymentRecord(order.ID) {
		return current(func(r *dto.PaymentStatusResponse) { r.LatePaymentReceived = true }), nil
	}

	result, err := s.lookupBankTransaction(ctx, order, now)
	// Service Python báo lỗi bằng Found=false + Status="error" (không phải Go error); gRPC lỗi hoặc
	// timeout. Tất cả là "chưa xác minh được": không bao giờ coi là "không có tiền". bank_unavailable
	// cho web phân biệt "ngân hàng lỗi, thử lại sau" với "chưa có giao dịch".
	if err != nil || result == nil || result.Status == "error" {
		log.Printf("[PAYMENT-CHECK] order=%s status=%s chưa đối chiếu được với ngân hàng (err=%v)", order.ID, order.Status, bankErrorDetail(err, result))
		if processing && now.After(deadline.Add(unverifiedExpiryAfter)) {
			resp, expErr := s.expireUnverified(ctx, order, deadline, bankErrorDetail(err, result))
			if resp != nil {
				resp.BankUnavailable = true
			}
			return resp, expErr
		}
		return current(func(r *dto.PaymentStatusResponse) {
			r.Reconciling = processing
			r.BankUnavailable = true
		}), nil
	}

	if !result.Found {
		if !processing {
			return current(nil), nil // đơn đã đóng, chưa có tiền: giữ nguyên
		}
		if !codeExpired {
			return &dto.PaymentStatusResponse{OrderID: order.ID, Status: "pending", Amount: order.TotalAmount}, nil
		}
		if now.Before(deadline.Add(paymentReconcileGracePeriod)) {
			// Ân hạn 30 phút (quyết định vòng 3): ngân hàng có thể ghi có chậm.
			return current(func(r *dto.PaymentStatusResponse) { r.Reconciling = true }), nil
		}
		expired, err := s.expireOrderTx(ctx, order, "Payment code expired (verified: no transaction)")
		if err != nil {
			return nil, err
		}
		if !expired {
			return current(nil), nil
		}
		return &dto.PaymentStatusResponse{OrderID: order.ID, Status: "expired", Amount: order.TotalAmount}, nil
	}

	// Quyết định 1: đơn đã đóng KHÔNG BAO GIỜ được khôi phục. Mọi giao dịch khớp mã → cần hoàn tiền.
	if !processing {
		flagged := s.flagRefundNeeded(ctx, order, result)
		return current(func(r *dto.PaymentStatusResponse) { r.LatePaymentReceived = flagged }), nil
	}

	amount, _ := decimal.NewFromString(result.Amount)
	amountMatches := amount.Compare(order.TotalAmount) == 0
	paidAt, dateKnown := parseBankTransactionDate(result.TransactionDate)
	// Trong hạn: khớp tiền VÀ (mã còn hạn lúc đối chiếu, hoặc ngân hàng ghi nhận <= hạn mã).
	if amountMatches && (!codeExpired || (dateKnown && !paidAt.After(deadline))) {
		return s.completePaidOrder(ctx, order, result)
	}
	if !codeExpired {
		// Sai số tiền khi mã còn hạn: giữ nguyên hành vi cũ (lỗi, đơn không đổi); đến hạn thì nhánh
		// dưới chốt expired + cờ hoàn tiền.
		return nil, ErrPaymentAmountMismatch
	}
	return s.expireWithLatePayment(ctx, order, result, paidAt, dateKnown, amountMatches)
}

func bankErrorDetail(err error, result *grpc.CheckTransactionResult) string {
	switch {
	case err != nil:
		return err.Error()
	case result == nil:
		return "empty response"
	default:
		return "status=error: " + result.ErrorMessage
	}
}

// errOrderClosedBeforeCompletion — đơn đã bị huỷ/chốt expired giữa lúc đọc và lúc hoàn tất (đua).
var errOrderClosedBeforeCompletion = errors.New("order closed before payment completion")

// completePaidOrder — hoàn tất đơn processing khi đã có giao dịch khớp tiền trong hạn. Đọc lại trạng
// thái DƯỚI KHOÁ DÒNG (C): đơn đã completed → ErrPaymentAlreadyDone; đơn vừa bị huỷ/chốt expired →
// KHÔNG hoàn tất (quyết định 1), gắn cờ cần hoàn tiền.
//
// H2-06 vòng 3b: bank_transaction_usage, payment info, history, enrollment/total_students, voucher
// log chạy CHUNG một transaction; lỗi ở bất kỳ bước nào rollback tất cả (unique constraint
// bank_transaction_usages vẫn chống xử lý trùng sau rollback).
func (s *PaymentService) completePaidOrder(ctx context.Context, order *model.Order, result *grpc.CheckTransactionResult) (*dto.PaymentStatusResponse, error) {
	orderID := order.ID
	fromStatus := order.Status
	paidNow := time.Now()
	err := s.orderRepo.WithTransaction(func(txRepo *repository.OrderRepository) error {
		txDB := txRepo.TxDB()

		var locked model.Order
		if err := txDB.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ?", orderID).First(&locked).Error; err != nil {
			return err
		}
		switch locked.Status {
		case "completed":
			return ErrPaymentAlreadyDone
		case "expired", "cancelled":
			return errOrderClosedBeforeCompletion
		case "pending", "processing":
		default:
			return repository.ErrOrderConflict
		}
		fromStatus = locked.Status

		// M-06 (audit 260909 vòng 2): chống replay — một giao dịch ngân hàng chỉ dùng cho 1 đơn.
		usage := &model.BankTransactionUsage{BankTransactionID: result.TransactionID, ReferenceType: "order", ReferenceID: orderID}
		if err := txRepo.RecordBankTransactionUsage(usage); err != nil {
			if errors.Is(err, gorm.ErrDuplicatedKey) {
				return ErrPaymentAlreadyDone
			}
			return err
		}
		if err := txRepo.UpdatePaymentInfo(orderID, "bank_transfer", "mbbank", result.TransactionID, paidNow); err != nil {
			return err
		}

		// Chốt phí nền tảng (quyết định #2, 27/09/2026) theo % tại thời điểm hoàn tất.
		if s.platformSettingRepo != nil {
			feePercent, feeErr := s.platformSettingRepo.GetPlatformFeePercent(ctx)
			if feeErr != nil {
				return feeErr
			}
			if err := txRepo.SetPlatformFeeSnapshot(orderID, feePercent, calculatePlatformFeeAmount(locked.TotalAmount, feePercent)); err != nil {
				return err
			}
		}

		if err := repository.NewOrderStatusHistoryRepository(txDB).Create(&model.OrderStatusHistory{
			ID: uuid.New(), CreatedAt: time.Now(), OrderID: orderID, FromStatus: locked.Status, ToStatus: "completed",
			Reason: "Payment received via transaction check",
		}); err != nil {
			return err
		}

		items, err := repository.NewOrderItemRepository(txDB).GetByOrderID(orderID)
		if err != nil {
			return err
		}
		if s.enrollmentRepo != nil {
			if err := completeOrderFulfillment(ctx, txDB, repository.NewEnrollmentRepository(txDB), repository.NewCourseRepository(txDB), s.voucherService, items, &locked); err != nil {
				return err
			}
		}
		return nil
	})
	if errors.Is(err, errOrderClosedBeforeCompletion) {
		fresh, rerr := s.orderRepo.GetByID(orderID)
		if rerr != nil {
			return nil, rerr
		}
		flagged := s.flagRefundNeeded(ctx, fresh, result)
		return &dto.PaymentStatusResponse{OrderID: orderID, Status: fresh.Status, PaidAt: fresh.PaidAt, Amount: fresh.TotalAmount, LatePaymentReceived: flagged}, nil
	}
	if err != nil {
		if shouldAlertFulfillmentFailure(err) {
			s.recordFulfillmentFailureAlert(orderID, fromStatus, result.TransactionID, result.Amount, err)
		}
		return nil, err
	}
	return &dto.PaymentStatusResponse{OrderID: orderID, Status: "completed", PaidAt: &paidNow, Amount: order.TotalAmount}, nil
}

// expireUnverified (vòng 4 D) — đơn processing quá hạn mã + unverifiedExpiryAfter mà ngân hàng vẫn
// lỗi: chốt expired (hoàn voucher) + history unverified_expiry + [PAYMENT-ALERT] để admin đối soát
// sao kê tay. Học viên mua lại được; tiền về sau đó chỉ gắn cờ hoàn tiền (quyết định 1).
func (s *PaymentService) expireUnverified(ctx context.Context, order *model.Order, deadline time.Time, detail string) (*dto.PaymentStatusResponse, error) {
	note := fmt.Sprintf("Bank could not be verified %s after the payment code expired at %s (last error: %s). Expired without verification; reconcile manually against the bank statement.",
		unverifiedExpiryAfter, deadline.Format(time.RFC3339), detail)
	var applied bool
	err := s.orderRepo.WithTransaction(func(txRepo *repository.OrderRepository) error {
		var txErr error
		applied, txErr = releaseOrderAndTransition(ctx, txRepo, s.voucherService, order, []string{"processing"}, "expired", "Payment code expired (bank unverifiable)")
		if txErr != nil || !applied {
			return txErr
		}
		return repository.NewOrderStatusHistoryRepository(txRepo.TxDB()).Create(&model.OrderStatusHistory{
			ID: uuid.New(), CreatedAt: time.Now(), OrderID: order.ID, FromStatus: "expired", ToStatus: unverifiedExpiryHistoryStatus, Reason: note,
		})
	})
	if err != nil {
		return nil, err
	}
	if !applied {
		fresh, rerr := s.orderRepo.GetByID(order.ID)
		if rerr != nil {
			return nil, rerr
		}
		return &dto.PaymentStatusResponse{OrderID: order.ID, Status: fresh.Status, PaidAt: fresh.PaidAt, Amount: fresh.TotalAmount}, nil
	}
	log.Printf("[PAYMENT-ALERT] order=%s: chốt expired khi CHƯA xác minh được với ngân hàng (%s), cần đối soát sao kê thủ công", order.ID, detail)
	order.Status = "expired"
	return &dto.PaymentStatusResponse{OrderID: order.ID, Status: "expired", Amount: order.TotalAmount}, nil
}

// flagRefundNeeded — đơn đã đóng (expired/cancelled) có giao dịch cho mã của nó (quyết định 1: trong
// hay sau hạn, đúng hay sai số tiền): ghi history payment_after_expiry đúng 1 lần (khoá dòng order
// rồi mới kiểm history) + log [PAYMENT-LATE-REFUND-NEEDED]. Không đổi trạng thái, không ghi danh,
// không đụng voucher. Trả true khi đơn đã có cờ (vừa ghi hoặc từ trước).
func (s *PaymentService) flagRefundNeeded(ctx context.Context, order *model.Order, result *grpc.CheckTransactionResult) bool {
	note := fmt.Sprintf("Received bank transaction %s amount %s at %s for order already %s (payment code expired at %s). Order is NOT restored; refund manually.",
		result.TransactionID, result.Amount, result.TransactionDate, order.Status, paymentCodeDeadline(order).Format(time.RFC3339))
	var recorded bool
	txErr := s.orderRepo.WithTransaction(func(txRepo *repository.OrderRepository) error {
		txDB := txRepo.TxDB()
		var locked model.Order
		if err := txDB.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ?", order.ID).First(&locked).Error; err != nil {
			return err
		}
		var existing int64
		if err := txDB.Model(&model.OrderStatusHistory{}).Where("order_id = ? AND to_status = ?", order.ID, latePaymentHistoryStatus).Count(&existing).Error; err != nil {
			return err
		}
		if existing > 0 {
			return nil
		}
		recorded = true
		return repository.NewOrderStatusHistoryRepository(txDB).Create(&model.OrderStatusHistory{
			ID: uuid.New(), CreatedAt: time.Now(), OrderID: order.ID, FromStatus: locked.Status, ToStatus: latePaymentHistoryStatus, Reason: note,
		})
	})
	if txErr != nil {
		log.Printf("[PAYMENT-LATE-REFUND-NEEDED] order=%s amount=%s tx=%s: có tiền cho đơn đã đóng nhưng KHÔNG ghi được cờ hoàn tiền: %v", order.ID, result.Amount, result.TransactionID, txErr)
		return true
	}
	if recorded {
		log.Printf("[PAYMENT-LATE-REFUND-NEEDED] order=%s amount=%s tx=%s status=%s: nhận tiền cho đơn đã đóng, cần hoàn tiền thủ công", order.ID, result.Amount, result.TransactionID, order.Status)
	}
	return true
}

// lateRefundState — trạng thái hoàn tiền của đơn đã đóng nhận tiền về muộn, đọc từ history:
// needed = có cờ payment_after_expiry mà admin CHƯA ghi late_refund_done; refundedAt != nil khi đã
// ghi (mốc admin xác nhận đã chuyển khoản hoàn). Chỉ đơn đã đóng từng có mã mới có thể có cờ nên
// chỉ những đơn đó tốn 1 truy vấn.
func lateRefundState(db *gorm.DB, order *model.Order) (needed bool, refundedAt *time.Time) {
	if db == nil || (order.Status != "expired" && order.Status != "cancelled") || !hasPaymentCode(order) {
		return false, nil
	}
	var rows []struct {
		ToStatus string
		LastAt   time.Time
	}
	if err := db.Model(&model.OrderStatusHistory{}).
		Select("to_status, MAX(created_at) AS last_at").
		Where("order_id = ? AND to_status IN ?", order.ID, []string{latePaymentHistoryStatus, lateRefundDoneHistoryStatus}).
		Group("to_status").Scan(&rows).Error; err != nil {
		log.Printf("[PAYMENT-STATUS] order=%s không đọc được cờ hoàn tiền: %v", order.ID, err)
		return false, nil
	}
	flagged := false
	for _, r := range rows {
		switch r.ToStatus {
		case latePaymentHistoryStatus:
			flagged = true
		case lateRefundDoneHistoryStatus:
			at := r.LastAt
			refundedAt = &at
		}
	}
	if !flagged {
		return false, nil
	}
	return refundedAt == nil, refundedAt
}
