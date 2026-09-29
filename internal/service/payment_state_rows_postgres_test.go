package service

// Review #76 final (MINOR 6): các ô còn thiếu của bảng trạng thái — sai số tiền, completed/refunded,
// đơn đã đóng khi ngân hàng lỗi (web phân biệt "ngân hàng lỗi" với "chưa có giao dịch"), huỷ đơn
// sau 24h khi ngân hàng lỗi.

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"study.com/v1/internal/grpc"
	"study.com/v1/internal/repository"
)

func bankPaidAmount(at time.Time, amount string) *grpc.CheckTransactionResult {
	r := bankPaid(at)
	r.Amount = amount
	return r
}

// Sai số tiền: giữ hành vi cũ. Mã còn hạn → lỗi ErrPaymentAmountMismatch, đơn không đổi; mã đã quá
// ân hạn → expired + cờ hoàn tiền; đơn đã đóng → giữ nguyên + cờ hoàn tiền.
func TestAmountMismatch_Rows(t *testing.T) {
	ctx := context.Background()

	t.Run("processing mã còn hạn", func(t *testing.T) {
		f := newOrderFixture(t)
		student := f.user()
		orderID, _, _ := f.processingWithCodeExpiring(student, "QA-final sai tiền còn hạn", time.Hour)
		_, err := f.paymentServiceWith(&fakeBankLookup{result: bankPaidAmount(time.Now(), "100000")}, nil).CheckAndProcessPayment(ctx, orderID, student, false)
		if !errors.Is(err, ErrPaymentAmountMismatch) || f.loadOrder(orderID).Status != "processing" {
			t.Fatalf("err=%v db=%q, muốn ErrPaymentAmountMismatch và giữ processing", err, f.loadOrder(orderID).Status)
		}
		if n := f.historyCount(orderID, latePaymentHistoryStatus); n != 0 {
			t.Fatalf("cờ hoàn tiền = %d, muốn 0 khi mã còn hạn", n)
		}
	})

	t.Run("processing quá ân hạn", func(t *testing.T) {
		f := newOrderFixture(t)
		student := f.user()
		orderID, codeExpiry, _ := f.processingWithCodeExpiring(student, "QA-final sai tiền quá hạn", -time.Hour)
		st, err := f.paymentServiceWith(&fakeBankLookup{result: bankPaidAmount(codeExpiry.Add(-time.Minute), "100000")}, nil).CheckAndProcessPayment(ctx, orderID, student, false)
		if err != nil || st.Status != "expired" || !st.LatePaymentReceived || f.historyCount(orderID, latePaymentHistoryStatus) != 1 {
			t.Fatalf("resp=%+v err=%v, muốn expired + cờ hoàn tiền đúng 1", st, err)
		}
	})

	t.Run("cancelled có mã", func(t *testing.T) {
		f := newOrderFixture(t)
		student := f.user()
		orderID, codeExpiry, course := f.processingWithCodeExpiring(student, "QA-final sai tiền đơn huỷ", -2*time.Hour)
		voucherID := f.attachVoucher(orderID, 0)
		f.exec("UPDATE orders SET status = 'cancelled' WHERE id = ?", orderID)
		st, err := f.paymentServiceWith(&fakeBankLookup{result: bankPaidAmount(codeExpiry.Add(-time.Minute), "100000")}, nil).CheckAndProcessPayment(ctx, orderID, student, false)
		if err != nil || st.Status != "cancelled" || !st.LatePaymentReceived {
			t.Fatalf("resp=%+v err=%v, muốn cancelled + cờ hoàn tiền", st, err)
		}
		f.assertRefundFlaggedOnly(t, orderID, "cancelled", student, course, 0, voucherID, 0)
	})
}

// completed / refunded là trạng thái cuối: poll trả nguyên trạng thái, KHÔNG gọi ngân hàng, không
// huỷ được. (Admin refund completed → refunded: TestRefundOrder_WaitsForWithdrawalPayout.)
func TestFinalStates_NoBankCallAndNotCancellable(t *testing.T) {
	for _, status := range []string{"completed", "refunded"} {
		t.Run(status, func(t *testing.T) {
			f := newOrderFixture(t)
			student := f.user()
			orderID, _, _ := f.processingWithCodeExpiring(student, "QA-final trạng thái cuối "+status, time.Hour)
			f.exec("UPDATE orders SET status = ?, paid_at = now() WHERE id = ?", status, orderID)
			bank := &fakeBankLookup{result: bankPaid(time.Now())}
			pay := f.paymentServiceWith(bank, nil)
			f.svc.SetPaymentReconciler(pay)

			st, err := pay.GetPaymentStatus(context.Background(), orderID, student, false)
			if err != nil || st.Status != status {
				t.Fatalf("poll = %+v err=%v, muốn %s", st, err, status)
			}
			if c := bank.calls.Load(); c != 0 {
				t.Fatalf("grpcCalls = %d, muốn 0", c)
			}
			if err := f.svc.CancelOrder(context.Background(), student, orderID, false, "QA"); !errors.Is(err, ErrInvalidStateTransition) {
				t.Fatalf("huỷ đơn %s: err=%v, muốn ErrInvalidStateTransition", status, err)
			}
			if got := f.loadOrder(orderID).Status; got != status {
				t.Fatalf("status DB = %q, muốn giữ %q", got, status)
			}
		})
	}
}

// Web phân biệt "ngân hàng lỗi, thử lại sau" với "chưa có giao dịch": kiểm đơn đã đóng khi ngân
// hàng lỗi trả bank_unavailable, không gắn cờ; ngân hàng trả not_found thì không có cờ đó.
func TestClosedOrder_BankErrorIsReportedAsUnavailable(t *testing.T) {
	f := newOrderFixture(t)
	student := f.user()
	orderID, _, _ := f.processingWithCodeExpiring(student, "QA-final ngân hàng lỗi đơn huỷ", -2*time.Hour)
	f.exec("UPDATE orders SET status = 'cancelled' WHERE id = ?", orderID)
	ctx := context.Background()

	st, err := f.paymentServiceWith(&fakeBankLookup{result: bankError}, nil).CheckAndProcessPayment(ctx, orderID, student, false)
	if err != nil || st.Status != "cancelled" || !st.BankUnavailable || st.LatePaymentReceived {
		t.Fatalf("ngân hàng lỗi: resp=%+v err=%v, muốn cancelled + bank_unavailable, không cờ hoàn tiền", st, err)
	}
	st, err = f.paymentServiceWith(&fakeBankLookup{result: bankNotFound}, nil).CheckAndProcessPayment(ctx, orderID, student, false)
	if err != nil || st.Status != "cancelled" || st.BankUnavailable {
		t.Fatalf("not_found: resp=%+v err=%v, muốn cancelled, không bank_unavailable", st, err)
	}
	if n := f.historyCount(orderID, latePaymentHistoryStatus); n != 0 {
		t.Fatalf("cờ hoàn tiền = %d, muốn 0", n)
	}
}

// Quyết định 1 (admin): danh sách đơn admin trả refund_needed cho đơn đã gắn cờ để trang đơn hàng
// hiện badge "Cần hoàn tiền"; đơn khác của cùng học viên không có cờ.
func TestAdminListOrders_ReportsRefundNeeded(t *testing.T) {
	f := newOrderFixture(t)
	student := f.user()
	flaggedID, codeExpiry, _ := f.processingWithCodeExpiring(student, "QA-final admin cờ hoàn tiền", -2*time.Hour)
	f.exec("UPDATE orders SET status = 'cancelled' WHERE id = ?", flaggedID)
	otherID, _, _ := f.processingWithCodeExpiring(student, "QA-final admin đơn không cờ", time.Hour)
	if _, err := f.paymentServiceWith(&fakeBankLookup{result: bankPaid(codeExpiry.Add(-time.Minute))}, nil).CheckAndProcessPayment(context.Background(), flaggedID, student, false); err != nil {
		t.Fatalf("gắn cờ: %v", err)
	}

	admin := NewAdminOrderService(repository.NewOrderRepository(f.db), repository.NewOrderItemRepository(f.db),
		repository.NewEnrollmentRepository(f.db), repository.NewCourseRepository(f.db))
	list, err := admin.ListOrders(context.Background(), repository.AdminOrderFilter{UserID: &student, Page: 1, Limit: 50})
	if err != nil {
		t.Fatalf("ListOrders: %v", err)
	}
	got := map[uuid.UUID]bool{}
	for _, it := range list.Items {
		got[it.ID] = it.RefundNeeded
	}
	if flagged, ok := got[flaggedID]; !ok || !flagged {
		t.Fatalf("đơn đã gắn cờ: refund_needed=%v (có trong danh sách: %v), muốn true", flagged, ok)
	}
	if other, ok := got[otherID]; !ok || other {
		t.Fatalf("đơn không cờ: refund_needed=%v (có trong danh sách: %v), muốn false", other, ok)
	}
}

// MINOR 3: nhánh "hoàn tất thắng, huỷ đọc lại" và "vừa chốt expired" trả đúng lỗi 409.
func TestClosedOrderCancelError(t *testing.T) {
	cases := map[string]error{"completed": ErrPaymentAlreadyDone, "expired": ErrOrderExpired, "cancelled": nil, "processing": nil}
	for status, want := range cases {
		if got := closedOrderCancelError(status); got != want {
			t.Errorf("closedOrderCancelError(%q) = %v, muốn %v", status, got, want)
		}
	}
}
