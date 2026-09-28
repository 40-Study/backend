package service

// Review #76 vòng 4 — thiết kế đối chiếu thống nhất (chủ dự án chốt 29/09/2026). Bốn vòng trước mỗi
// nhánh (poll, intent, huỷ, đơn đã đóng) tự quyết định riêng nên vòng nào cũng lộ một lỗ. Giờ mọi
// đơn ĐÃ CẤP MÃ chuyển khoản đi qua đúng một hàm reconcileIssuedOrder, bất kể trạng thái:
//
//   A. Bất biến: đơn đã cấp mã (processing/expired/cancelled) luôn được đối chiếu trong cửa sổ tra
//      cứu CỐ ĐỊNH của nó. Tiền khớp về trong hạn → completed (đơn đã đóng được khôi phục). Tiền về
//      sau hạn hoặc sai số tiền → history payment_after_expiry + [PAYMENT-ALERT] đúng 1 lần. Không
//      đơn nào bị coi là "không có tiền" khi chưa qua ân hạn và chưa xác minh được với ngân hàng.
//   B. Cửa sổ cố định: from = ngày cấp mã - 1 ngày; to = min(hạn mã + ân hạn + 1 ngày, bây giờ).
//      Không bao giờ gửi ngày tương lai, cửa sổ không dài thêm theo thời gian.
//   C. Hoàn tất đọc lại trạng thái DƯỚI KHOÁ DÒNG (SELECT ... FOR UPDATE) nên đua với huỷ an toàn:
//      huỷ thắng thì hoàn tất khôi phục đơn; hoàn tất thắng thì huỷ đọc lại và báo đã thanh toán.
//   D. Không kẹt vĩnh viễn: đơn processing quá hạn mã + unverifiedExpiryAfter mà ngân hàng vẫn lỗi
//      → expired + history unverified_expiry + [PAYMENT-ALERT]. Tiền về sau đó vẫn theo A.

import (
	"context"
	"errors"
	"fmt"
	"log"
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

// paymentCodeDeadline — hạn mã của đơn đã cấp mã. Đơn cũ (trước khi có cột payment_code_expired_at)
// lấy created_at + 24h, đúng mốc mặc định lúc cấp.
func paymentCodeDeadline(order *model.Order) time.Time {
	if order.PaymentCodeExpiredAt != nil {
		return *order.PaymentCodeExpiredAt
	}
	return order.CreatedAt.Add(pendingOrderDefaultTTL)
}

// bankLookupWindow (vòng 4 B) — cửa sổ tra cứu CỐ ĐỊNH của một đơn, chỉ phụ thuộc dữ liệu đã lưu
// của đơn và "bây giờ" (chỉ để chặn ngày tương lai). Lúc cấp mã >= created_at và >= hạn mã - 24h
// (hạn mã = min(lúc cấp + 24h, hạn giữ đơn)). Nới 1 ngày (bankLookupDateSlack) mỗi phía vì service
// Python đổi timestamp sang NGÀY theo múi giờ máy nó chạy (UTC hay giờ VN).
func bankLookupWindow(order *model.Order, now time.Time) (from, to time.Time) {
	deadline := paymentCodeDeadline(order)
	issued := deadline.Add(-pendingOrderDefaultTTL)
	if order.CreatedAt.After(issued) {
		issued = order.CreatedAt
	}
	from = issued.Add(-bankLookupDateSlack)
	to = deadline.Add(paymentReconcileGracePeriod + bankLookupDateSlack)
	if to.After(now) {
		to = now
	}
	return from, to
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

	// Đơn đã đóng từng ghi nhận tiền về muộn: đã cảnh báo đúng 1 lần, không gọi ngân hàng lại.
	if !processing && s.hasLatePaymentRecord(order.ID) {
		return current(func(r *dto.PaymentStatusResponse) { r.LatePaymentReceived = true }), nil
	}

	result, err := s.lookupBankTransaction(ctx, order, now)
	// Service Python báo lỗi bằng Found=false + Status="error" (không phải Go error); gRPC lỗi hoặc
	// timeout. Tất cả là "chưa xác minh được": không bao giờ coi là "không có tiền".
	if err != nil || result == nil || result.Status == "error" {
		log.Printf("[PAYMENT-CHECK] order=%s status=%s chưa đối chiếu được với ngân hàng (err=%v)", order.ID, order.Status, bankErrorDetail(err, result))
		if processing && now.After(deadline.Add(unverifiedExpiryAfter)) {
			return s.expireUnverified(ctx, order, deadline, bankErrorDetail(err, result))
		}
		return current(func(r *dto.PaymentStatusResponse) { r.Reconciling = processing }), nil
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

	amount, _ := decimal.NewFromString(result.Amount)
	amountMatches := amount.Compare(order.TotalAmount) == 0
	paidAt, dateKnown := parseBankTransactionDate(result.TransactionDate)
	// Trong hạn: khớp tiền VÀ (mã còn hạn lúc đối chiếu, hoặc ngân hàng ghi nhận <= hạn mã).
	paidInTime := amountMatches && (!codeExpired || (dateKnown && !paidAt.After(deadline)))
	if paidInTime {
		return s.completePaidOrder(ctx, order, result)
	}
	if processing {
		if !codeExpired {
			return nil, ErrPaymentAmountMismatch
		}
		return s.expireWithLatePayment(ctx, order, result, paidAt, dateKnown, amountMatches)
	}
	recorded := s.recordLatePaymentOnClosedOrder(ctx, order, result)
	return current(func(r *dto.PaymentStatusResponse) { r.LatePaymentReceived = recorded }), nil
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

// completePaidOrder — hoàn tất đơn khi đã có giao dịch khớp tiền trong hạn. Đọc lại trạng thái DƯỚI
// KHOÁ DÒNG (vòng 4 C): đơn đã completed → ErrPaymentAlreadyDone; đơn đã bị huỷ/chốt expired trong
// lúc đó → KHÔI PHỤC thành completed (học viên đã trả đúng hạn), lấy lại lượt voucher đã hoàn.
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
		case "pending", "processing", "expired", "cancelled":
		default:
			return repository.ErrOrderConflict
		}
		fromStatus = locked.Status
		restored := locked.Status == "expired" || locked.Status == "cancelled"

		// M-06 (audit 260909 vòng 2): chống replay — một giao dịch ngân hàng chỉ dùng cho 1 đơn.
		usage := &model.BankTransactionUsage{BankTransactionID: result.TransactionID, ReferenceType: "order", ReferenceID: orderID}
		if err := txRepo.RecordBankTransactionUsage(usage); err != nil {
			if errors.Is(err, gorm.ErrDuplicatedKey) {
				return ErrPaymentAlreadyDone
			}
			return err
		}

		if restored {
			res := txDB.Model(&model.Order{}).
				Where("id = ? AND status = ?", orderID, locked.Status).
				Updates(map[string]interface{}{
					"payment_method":         "bank_transfer",
					"payment_gateway":        "mbbank",
					"payment_transaction_id": result.TransactionID,
					"paid_at":                paidNow,
					"status":                 "completed",
				})
			if res.Error != nil {
				return res.Error
			}
			if res.RowsAffected == 0 {
				return repository.ErrOrderConflict
			}
			// Lúc huỷ/chốt expired, releaseOrderAndTransition đã trả lại 1 lượt voucher. Học viên đã
			// trả đúng giá có giảm giá trong hạn nên lượt đó thuộc về đơn này: tăng lại KHÔNG điều kiện
			// (tiền đã nhận, không được từ chối vì hết lượt).
			if locked.VoucherID != nil {
				if err := txDB.Model(&model.Voucher{}).Where("id = ?", *locked.VoucherID).
					Update("used_count", gorm.Expr("used_count + 1")).Error; err != nil {
					return err
				}
			}
		} else if err := txRepo.UpdatePaymentInfo(orderID, "bank_transfer", "mbbank", result.TransactionID, paidNow); err != nil {
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

		reason := "Payment received via transaction check"
		if restored {
			reason = fmt.Sprintf("Payment %s (amount %s) received within payment code validity; order restored from %s", result.TransactionID, result.Amount, locked.Status)
		}
		if err := repository.NewOrderStatusHistoryRepository(txDB).Create(&model.OrderStatusHistory{
			ID: uuid.New(), CreatedAt: time.Now(), OrderID: orderID, FromStatus: locked.Status, ToStatus: "completed", Reason: reason,
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
	if err != nil {
		if shouldAlertFulfillmentFailure(err) {
			s.recordFulfillmentFailureAlert(orderID, fromStatus, result.TransactionID, result.Amount, err)
		}
		return nil, err
	}
	if fromStatus == "expired" || fromStatus == "cancelled" {
		log.Printf("[PAYMENT-RESTORE] order=%s tx=%s: tiền về trong hạn cho đơn đã %s, đã khôi phục completed", orderID, result.TransactionID, fromStatus)
	}
	return &dto.PaymentStatusResponse{OrderID: orderID, Status: "completed", PaidAt: &paidNow, Amount: order.TotalAmount}, nil
}

// expireUnverified (vòng 4 D) — đơn processing quá hạn mã + unverifiedExpiryAfter mà ngân hàng vẫn
// lỗi: chốt expired (hoàn voucher) + history unverified_expiry + [PAYMENT-ALERT] để admin đối soát
// sao kê tay. Học viên mua lại được; tiền về sau đó (trong cửa sổ cố định) vẫn được xử lý theo A.
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

// recordLatePaymentOnClosedOrder — đơn đã đóng (expired/cancelled) có giao dịch cho mã của nó nhưng
// không đủ điều kiện hoàn tất (về sau hạn, sai số tiền, không đọc được ngày): ghi history
// payment_after_expiry + [PAYMENT-ALERT] đúng 1 lần (khoá dòng order rồi mới kiểm history). Không đổi
// trạng thái đơn.
func (s *PaymentService) recordLatePaymentOnClosedOrder(ctx context.Context, order *model.Order, result *grpc.CheckTransactionResult) bool {
	note := fmt.Sprintf("Received bank transaction %s amount %s at %s for order already %s (payment code expired at %s). Refund manually.",
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
		log.Printf("[PAYMENT-ALERT] order=%s tx=%s: có tiền cho đơn đã đóng nhưng KHÔNG ghi được history: %v", order.ID, result.TransactionID, txErr)
		return true
	}
	if recorded {
		log.Printf("[PAYMENT-ALERT] order=%s tx=%s amount=%s: nhận tiền cho đơn đã %s, cần hoàn tiền thủ công", order.ID, result.TransactionID, result.Amount, order.Status)
	}
	return true
}
