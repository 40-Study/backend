package service

// Review #76 vòng 4 — probe của reviewer (Q1, Q2) chuyển thành test Postgres thật. Thiết kế thống
// nhất do chủ dự án chốt: mọi đơn đã cấp mã (kể cả đã expired/cancelled) vẫn được đối chiếu trong
// cửa sổ cố định; huỷ đơn có mã vẫn chặn tạo đơn trùng 30 phút; ngân hàng lỗi quá 24h thì chốt
// expired kèm history unverified_expiry.

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"study.com/v1/internal/grpc"
	"study.com/v1/internal/model"
)

// Q2 (MAJOR B2): huỷ khi mã CÒN HẠN mà ngân hàng chưa ghi có. Đơn huỷ vẫn chặn tạo đơn trùng
// (409 ERR_PAYMENT_VERIFYING). Tiền (chuyển trong hạn) về sau khi huỷ: QUYẾT ĐỊNH 1 — KHÔNG khôi
// phục, không ghi danh, không dùng voucher; chỉ gắn cờ cần hoàn tiền.
func TestCancelOrder_CodeStillValid_BlocksDuplicateAndFlagsRefundOnCredit(t *testing.T) {
	f := newOrderFixture(t)
	student := f.user()
	orderID, _, course := f.processingWithCodeExpiring(student, "QA-r4 huỷ khi mã còn hạn", time.Hour)
	voucherID := f.attachVoucher(orderID, 1)
	bank := &fakeBankLookup{seq: []*grpc.CheckTransactionResult{bankNotFound, bankPaid(time.Now())}}
	pay := f.paymentServiceWith(bank, nil)
	f.svc.SetPaymentReconciler(pay)
	ctx := context.Background()

	if err := f.svc.CancelOrder(ctx, student, orderID, false, "QA"); err != nil {
		t.Fatalf("CancelOrder: %v (phương án b: vẫn cho huỷ sau khi đối chiếu)", err)
	}
	if got := f.loadOrder(orderID).Status; got != "cancelled" {
		t.Fatalf("status sau huỷ = %q, muốn cancelled", got)
	}
	// Web cần biết đơn huỷ này từng có mã để hiện nút "Kiểm tra thanh toán".
	if resp, err := f.svc.GetOrderByID(ctx, orderID, student, false); err != nil || !resp.PaymentCodeIssued || resp.RefundNeeded {
		t.Fatalf("GetOrder đơn huỷ có mã, chưa có tiền: resp=%+v err=%v, muốn payment_code_issued, chưa refund_needed", resp, err)
	}
	if _, err := f.svc.CreateOrder(ctx, student, buyNow(course)); !errors.Is(err, ErrPaymentVerificationPending) {
		t.Fatalf("CreateOrder cùng khoá ngay sau khi huỷ đơn có mã: err = %v, muốn ErrPaymentVerificationPending (409 ERR_PAYMENT_VERIFYING)", err)
	}
	usedAfterCancel := f.voucherUsed(voucherID)

	st, err := pay.GetPaymentStatus(ctx, orderID, student, false)
	if err != nil || st.Status != "cancelled" || !st.LatePaymentReceived {
		t.Fatalf("tiền về cho đơn đã huỷ: resp=%+v err=%v, muốn giữ cancelled + late_payment_received", st, err)
	}
	f.assertRefundFlaggedOnly(t, orderID, "cancelled", student, course, 0, voucherID, usedAfterCancel)
}
// Q1 (MAJOR B3): ngân hàng lỗi mãi. Quá hạn mã + 24h thì chốt expired kèm history unverified_expiry
// để admin đối soát tay, và khoá học được mở lại cho học viên mua.
func TestUnverifiableBank_ExpiresAfter24hAndUnblocksCourse(t *testing.T) {
	f := newOrderFixture(t)
	student := f.user()
	orderID, _, course := f.processingWithCodeExpiring(student, "QA-r4 ngân hàng lỗi mãi", -25*time.Hour)
	pay := f.paymentServiceWith(&fakeBankLookup{result: bankError}, nil)
	f.svc.SetPaymentReconciler(pay)
	ctx := context.Background()

	st, err := pay.GetPaymentStatus(ctx, orderID, student, false)
	if err != nil {
		t.Fatalf("GetPaymentStatus: %v", err)
	}
	if st.Status != "expired" || f.loadOrder(orderID).Status != "expired" {
		t.Fatalf("quá hạn mã 25h, ngân hàng lỗi: resp=%+v db=%q, muốn expired (không kẹt vĩnh viễn)", st, f.loadOrder(orderID).Status)
	}
	if n := f.historyCount(orderID, "unverified_expiry"); n != 1 {
		t.Fatalf("history unverified_expiry = %d, muốn 1", n)
	}
	if _, err := f.svc.CreateOrder(ctx, student, buyNow(course)); err != nil {
		t.Fatalf("CreateOrder sau khi đơn kẹt đã chốt: %v, muốn tạo được", err)
	}
}

// Đối chứng D: CHƯA quá 24h sau hạn mã mà ngân hàng lỗi → vẫn processing (reconciling) và vẫn chặn
// tạo đơn trùng.
func TestUnverifiableBank_Before24hStaysProcessingAndBlocks(t *testing.T) {
	f := newOrderFixture(t)
	student := f.user()
	orderID, _, course := f.processingWithCodeExpiring(student, "QA-r4 ngân hàng lỗi 23h", -23*time.Hour)
	pay := f.paymentServiceWith(&fakeBankLookup{result: bankError}, nil)
	ctx := context.Background()

	st, err := pay.GetPaymentStatus(ctx, orderID, student, false)
	if err != nil || st.Status != "processing" || !st.Reconciling {
		t.Fatalf("resp=%+v err=%v, muốn processing + reconciling", st, err)
	}
	if n := f.historyCount(orderID, unverifiedExpiryHistoryStatus); n != 0 {
		t.Fatalf("history unverified_expiry = %d, muốn 0 trước mốc 24h", n)
	}
	if _, err := f.svc.CreateOrder(ctx, student, buyNow(course)); !errors.Is(err, ErrPaymentVerificationPending) {
		t.Fatalf("CreateOrder: err = %v, muốn ErrPaymentVerificationPending", err)
	}
}

// A + D + quyết định 1: đơn đã chốt expired vì ngân hàng lỗi quá 24h; sau đó ngân hàng trả giao dịch
// khớp tiền về TRƯỚC hạn mã → KHÔNG khôi phục (học viên có thể đã mua lại), chỉ gắn cờ hoàn tiền.
func TestUnverifiedExpiredOrder_InTimeCreditFlagsRefund(t *testing.T) {
	f := newOrderFixture(t)
	student := f.user()
	orderID, codeExpiry, course := f.processingWithCodeExpiring(student, "QA-r4 tiền về sau unverified", -25*time.Hour)
	voucherID := f.attachVoucher(orderID, 1)
	bank := &fakeBankLookup{seq: []*grpc.CheckTransactionResult{bankError, bankPaid(codeExpiry.Add(-time.Minute))}}
	pay := f.paymentServiceWith(bank, nil)
	ctx := context.Background()

	if st, err := pay.GetPaymentStatus(ctx, orderID, student, false); err != nil || st.Status != "expired" || !st.BankUnavailable {
		t.Fatalf("poll 1 = %+v err=%v, muốn expired (unverified) + bank_unavailable", st, err)
	}
	usedAfterExpire := f.voucherUsed(voucherID)
	st, err := pay.GetPaymentStatus(ctx, orderID, student, false)
	if err != nil || st.Status != "expired" || !st.LatePaymentReceived {
		t.Fatalf("poll 2 = %+v err=%v, muốn giữ expired + late_payment_received", st, err)
	}
	f.assertRefundFlaggedOnly(t, orderID, "expired", student, course, 0, voucherID, usedAfterExpire)
}

// Quyết định 1 (MAJOR 1 review final, probe P2): A đã huỷ quá 30 phút, học viên mua lại và trả đơn
// B cùng khoá; sau đó tiền trong hạn của A về. Không được thành 2 đơn completed: A giữ cancelled,
// gắn cờ hoàn tiền, ghi danh vẫn đúng 1, voucher của A không bị dùng lại.
func TestClosedOrder_CreditAfterRebuyFlagsRefundWithoutSecondCompletion(t *testing.T) {
	f := newOrderFixture(t)
	student := f.user()
	orderA, codeExpiryA, course := f.processingWithCodeExpiring(student, "QA-final mua lại", -2*time.Hour)
	voucherID := f.attachVoucher(orderA, 0)
	f.exec("UPDATE orders SET status = 'cancelled' WHERE id = ?", orderA)
	if err := f.db.Create(&model.OrderStatusHistory{ID: uuid.New(), CreatedAt: time.Now().Add(-31 * time.Minute), OrderID: orderA,
		FromStatus: "processing", ToStatus: "cancelled", Reason: "QA"}).Error; err != nil {
		t.Fatalf("history huỷ: %v", err)
	}
	ctx := context.Background()

	b, err := f.svc.CreateOrder(ctx, student, buyNow(course))
	if err != nil {
		t.Fatalf("mua lại (đơn B) sau khi hết chặn 30 phút: %v", err)
	}
	f.exec("UPDATE orders SET status = 'processing', payment_transaction_id = 'PAYQA-REBUY-B', payment_code_expired_at = ? WHERE id = ?", time.Now().Add(time.Hour), b.ID)
	if st, err := f.paymentServiceWith(&fakeBankLookup{result: bankPaid(time.Now())}, nil).GetPaymentStatus(ctx, b.ID, student, false); err != nil || st.Status != "completed" {
		t.Fatalf("trả đơn B: resp=%+v err=%v, muốn completed", st, err)
	}

	st, err := f.paymentServiceWith(&fakeBankLookup{result: bankPaid(codeExpiryA.Add(-time.Minute))}, nil).GetPaymentStatus(ctx, orderA, student, false)
	if err != nil || st.Status != "cancelled" || !st.LatePaymentReceived {
		t.Fatalf("tiền trong hạn của A về sau khi đã mua lại: resp=%+v err=%v, muốn giữ cancelled + cờ hoàn tiền", st, err)
	}
	var completed int64
	f.db.Model(&model.Order{}).Where("user_id = ? AND status = 'completed'", student).Count(&completed)
	if completed != 1 {
		t.Fatalf("số đơn completed = %d, muốn 1 (không được 2 đơn completed cho 1 khoá)", completed)
	}
	f.assertRefundFlaggedOnly(t, orderA, "cancelled", student, course, 1, voucherID, 0)
}
// A: đơn đã huỷ (sau hạn mã) nhận tiền về SAU hạn → giữ cancelled, cảnh báo payment_after_expiry
// đúng 1 lần; lần hỏi sau không gọi ngân hàng lại.
func TestCancelledOrder_LateCreditIsFlaggedOnce(t *testing.T) {
	f := newOrderFixture(t)
	student := f.user()
	orderID, codeExpiry, _ := f.processingWithCodeExpiring(student, "QA-r4 huỷ rồi tiền muộn", -2*time.Hour)
	f.exec("UPDATE orders SET status = 'cancelled' WHERE id = ?", orderID)
	bank := &fakeBankLookup{result: bankPaid(codeExpiry.Add(time.Hour))}
	pay := f.paymentServiceWith(bank, nil)
	ctx := context.Background()

	for i := 0; i < 3; i++ {
		st, err := pay.GetPaymentStatus(ctx, orderID, student, false)
		if err != nil || st.Status != "cancelled" || !st.LatePaymentReceived {
			t.Fatalf("lần %d = %+v err=%v, muốn cancelled + late_payment_received", i+1, st, err)
		}
	}
	if n := f.historyCount(orderID, latePaymentHistoryStatus); n != 1 {
		t.Fatalf("history %s = %d, muốn đúng 1", latePaymentHistoryStatus, n)
	}
	if c := bank.calls.Load(); c != 1 {
		t.Fatalf("grpcCalls = %d, muốn 1 (đã cảnh báo thì không gọi ngân hàng lại)", c)
	}
}

// C: đơn có mã bị huỷ chặn tạo đơn trùng tới 30 phút sau mốc MUỘN HƠN giữa lúc huỷ và hạn mã.
func TestCreateOrder_CancelledIssuedOrderHold(t *testing.T) {
	cases := []struct {
		name         string
		codeOffset   time.Duration // hạn mã so với bây giờ
		cancelledAgo time.Duration
		wantBlocked  bool
	}{
		{"huỷ 10 phút trước, mã đã hết hạn → chặn", -2 * time.Hour, 10 * time.Minute, true},
		{"huỷ 31 phút trước, mã đã hết hạn → hết chặn", -2 * time.Hour, 31 * time.Minute, false},
		{"huỷ 2 giờ trước nhưng mã còn hạn → chặn tới hạn mã + 30 phút", time.Hour, 2 * time.Hour, true},
		{"huỷ lâu, mã hết hạn 29 phút trước → vẫn chặn", -29 * time.Minute, 5 * time.Hour, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			f := newOrderFixture(t)
			student := f.user()
			orderID, _, course := f.processingWithCodeExpiring(student, "QA-r4 chặn sau huỷ", c.codeOffset)
			f.exec("UPDATE orders SET status = 'cancelled' WHERE id = ?", orderID)
			if err := f.db.Create(&model.OrderStatusHistory{
				ID: uuid.New(), CreatedAt: time.Now().Add(-c.cancelledAgo), OrderID: orderID,
				FromStatus: "processing", ToStatus: "cancelled", Reason: "QA",
			}).Error; err != nil {
				t.Fatalf("tạo history: %v", err)
			}
			_, err := f.svc.CreateOrder(context.Background(), student, buyNow(course))
			if c.wantBlocked && !errors.Is(err, ErrPaymentVerificationPending) {
				t.Fatalf("err = %v, muốn chặn ErrPaymentVerificationPending", err)
			}
			if !c.wantBlocked && err != nil {
				t.Fatalf("err = %v, muốn tạo được đơn mới", err)
			}
		})
	}
}

// C (đua): lần hoàn tất đọc đơn lúc còn processing, nhưng trước khi ghi thì đơn đã bị huỷ. Hoàn tất
// đọc lại DƯỚI KHOÁ DÒNG: quyết định 1 — không hoàn tất đơn đã đóng, gắn cờ hoàn tiền.
func TestCompletePaidOrder_CancelWonTheRaceFlagsRefund(t *testing.T) {
	f := newOrderFixture(t)
	student := f.user()
	orderID, _, course := f.processingWithCodeExpiring(student, "QA-r4 đua huỷ/hoàn tất", time.Hour)
	voucherID := f.attachVoucher(orderID, 0)
	stale := f.loadOrder(orderID)
	f.exec("UPDATE orders SET status = 'cancelled' WHERE id = ?", orderID)
	pay := f.paymentServiceWith(&fakeBankLookup{result: bankPaid(time.Now())}, nil)

	st, err := pay.reconcileIssuedOrder(context.Background(), &stale, reconcileOptions{})
	if err != nil || st.Status != "cancelled" || !st.LatePaymentReceived {
		t.Fatalf("resp=%+v err=%v, muốn cancelled + cờ hoàn tiền", st, err)
	}
	if n := f.historyCount(orderID, "completed"); n != 0 {
		t.Fatalf("history completed = %d, muốn 0", n)
	}
	f.assertRefundFlaggedOnly(t, orderID, "cancelled", student, course, 0, voucherID, 0)
}

// attachVoucher gắn một voucher thật vào đơn, used_count cho trước. Revert quyết định 1 (khôi phục)
// cộng lại used_count trực tiếp trong DB, nên assert used_count bắt được.
func (f *orderFixture) attachVoucher(orderID uuid.UUID, usedCount int32) uuid.UUID {
	f.t.Helper()
	v := model.Voucher{Code: "QAV" + uuid.NewString()[:8], Name: "QA voucher", DiscountUnit: model.DiscountUnitMoney,
		DiscountMethod: model.DiscountMethodFixed, UsedCount: usedCount, IsActive: true}
	if err := f.db.Create(&v).Error; err != nil {
		f.t.Fatalf("tạo voucher: %v", err)
	}
	f.exec("UPDATE orders SET voucher_id = ? WHERE id = ?", v.ID, orderID)
	return v.ID
}

func (f *orderFixture) voucherUsed(voucherID uuid.UUID) int32 {
	f.t.Helper()
	var v model.Voucher
	if err := f.db.Unscoped().Where("id = ?", voucherID).First(&v).Error; err != nil {
		f.t.Fatalf("đọc voucher: %v", err)
	}
	return v.UsedCount
}

// assertRefundFlaggedOnly — tiền về cho đơn đã đóng: trạng thái giữ nguyên, cờ hoàn tiền đúng 1
// lần (và hiện ở API), không ghi danh thêm, không dùng voucher.
func (f *orderFixture) assertRefundFlaggedOnly(t *testing.T, orderID uuid.UUID, wantStatus string, student, course uuid.UUID, wantEnrollments int64, voucherID uuid.UUID, wantVoucherUsed int32) {
	t.Helper()
	if got := f.loadOrder(orderID).Status; got != wantStatus {
		t.Fatalf("status DB = %q, muốn giữ %q (không khôi phục)", got, wantStatus)
	}
	if n := f.historyCount(orderID, latePaymentHistoryStatus); n != 1 {
		t.Fatalf("cờ hoàn tiền (history %s) = %d, muốn đúng 1", latePaymentHistoryStatus, n)
	}
	var enrollments int64
	f.db.Model(&model.Enrollment{}).Where("user_id = ? AND course_id = ?", student, course).Count(&enrollments)
	if enrollments != wantEnrollments {
		t.Fatalf("ghi danh = %d, muốn %d (đơn đã đóng không được ghi danh)", enrollments, wantEnrollments)
	}
	if used := f.voucherUsed(voucherID); used != wantVoucherUsed {
		t.Fatalf("voucher used_count = %d, muốn %d (đơn đã đóng không được dùng voucher)", used, wantVoucherUsed)
	}
	var logs int64
	f.db.Model(&model.VoucherLog{}).Where("order_id = ?", orderID).Count(&logs)
	if logs != 0 {
		t.Fatalf("voucher_logs của đơn = %d, muốn 0", logs)
	}
	resp, err := f.svc.GetOrderByID(context.Background(), orderID, student, false)
	if err != nil || !resp.RefundNeeded {
		t.Fatalf("GetOrderByID: refund_needed=%v err=%v, muốn true", resp != nil && resp.RefundNeeded, err)
	}
}