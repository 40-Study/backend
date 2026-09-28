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
// (409 ERR_PAYMENT_VERIFYING), và khi tiền (chuyển trong hạn) về thì đơn huỷ được khôi phục thành
// completed, không phải "tiền về muộn".
func TestCancelOrder_CodeStillValid_BlocksDuplicateAndRestoresOnInTimeCredit(t *testing.T) {
	f := newOrderFixture(t)
	student := f.user()
	orderID, _, course := f.processingWithCodeExpiring(student, "QA-r4 huỷ khi mã còn hạn", time.Hour)
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
	if resp, err := f.svc.GetOrderByID(ctx, orderID, student, false); err != nil || !resp.PaymentCodeIssued {
		t.Fatalf("GetOrder đơn huỷ có mã: payment_code_issued=%v err=%v, muốn true", resp != nil && resp.PaymentCodeIssued, err)
	}

	if _, err := f.svc.CreateOrder(ctx, student, buyNow(course)); !errors.Is(err, ErrPaymentVerificationPending) {
		t.Fatalf("CreateOrder cùng khoá ngay sau khi huỷ đơn có mã: err = %v, muốn ErrPaymentVerificationPending (409 ERR_PAYMENT_VERIFYING)", err)
	}

	st, err := pay.GetPaymentStatus(ctx, orderID, student, false)
	if err != nil {
		t.Fatalf("GetPaymentStatus đơn huỷ: %v", err)
	}
	if st.Status != "completed" || f.loadOrder(orderID).Status != "completed" {
		t.Fatalf("tiền về trong hạn cho đơn đã huỷ: resp=%+v db=%q, muốn khôi phục completed", st, f.loadOrder(orderID).Status)
	}
	if n := f.historyCount(orderID, latePaymentHistoryStatus); n != 0 {
		t.Fatalf("history %s = %d, muốn 0 (tiền về trong hạn không phải tiền về muộn)", latePaymentHistoryStatus, n)
	}
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

// A + D: đơn đã chốt expired vì ngân hàng lỗi quá 24h; sau đó ngân hàng trả giao dịch khớp tiền về
// TRƯỚC hạn mã → khôi phục completed, không coi là tiền về muộn.
func TestUnverifiedExpiredOrder_InTimeCreditRestoresCompleted(t *testing.T) {
	f := newOrderFixture(t)
	student := f.user()
	orderID, codeExpiry, _ := f.processingWithCodeExpiring(student, "QA-r4 khôi phục sau unverified", -25*time.Hour)
	bank := &fakeBankLookup{seq: []*grpc.CheckTransactionResult{bankError, bankPaid(codeExpiry.Add(-time.Minute))}}
	pay := f.paymentServiceWith(bank, nil)
	ctx := context.Background()

	if st, err := pay.GetPaymentStatus(ctx, orderID, student, false); err != nil || st.Status != "expired" {
		t.Fatalf("poll 1 = %+v err=%v, muốn expired (unverified)", st, err)
	}
	st, err := pay.GetPaymentStatus(ctx, orderID, student, false)
	if err != nil || st.Status != "completed" || f.loadOrder(orderID).Status != "completed" {
		t.Fatalf("poll 2 = %+v err=%v db=%q, muốn khôi phục completed", st, err, f.loadOrder(orderID).Status)
	}
	if n := f.historyCount(orderID, latePaymentHistoryStatus); n != 0 {
		t.Fatalf("history %s = %d, muốn 0", latePaymentHistoryStatus, n)
	}
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
// đọc lại DƯỚI KHOÁ DÒNG và khôi phục completed thay vì rollback im lặng (trước vòng 4:
// ErrOrderConflict, không cảnh báo).
func TestCompletePaidOrder_CancelWonTheRaceRestores(t *testing.T) {
	f := newOrderFixture(t)
	student := f.user()
	orderID, _, _ := f.processingWithCodeExpiring(student, "QA-r4 đua huỷ/hoàn tất", time.Hour)
	stale := f.loadOrder(orderID)
	f.exec("UPDATE orders SET status = 'cancelled' WHERE id = ?", orderID)
	pay := f.paymentServiceWith(&fakeBankLookup{result: bankPaid(time.Now())}, nil)

	st, err := pay.reconcileIssuedOrder(context.Background(), &stale)
	if err != nil || st.Status != "completed" || f.loadOrder(orderID).Status != "completed" {
		t.Fatalf("resp=%+v err=%v db=%q, muốn completed (khôi phục)", st, err, f.loadOrder(orderID).Status)
	}
	if n := f.historyCount(orderID, "completed"); n != 1 {
		t.Fatalf("history completed = %d, muốn 1", n)
	}
}
