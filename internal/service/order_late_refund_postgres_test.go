package service

// Lane P: admin đánh dấu "đã hoàn tiền xong" cho khoản tiền về muộn (cờ refund_needed, history
// payment_after_expiry). Postgres thật, schema tạm riêng (newOrderFixture).

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"study.com/v1/internal/repository"
)

func (f *orderFixture) adminOrderService() *AdminOrderService {
	return NewAdminOrderService(repository.NewOrderRepository(f.db), repository.NewOrderItemRepository(f.db),
		repository.NewEnrollmentRepository(f.db), repository.NewCourseRepository(f.db))
}

// flaggedClosedOrder dựng đơn đã huỷ có mã chuyển khoản và đã nhận tiền về muộn (đã gắn cờ
// payment_after_expiry) qua đường kiểm tra thanh toán THẬT, không chèn history bằng tay.
func (f *orderFixture) flaggedClosedOrder(student uuid.UUID, title string) uuid.UUID {
	f.t.Helper()
	orderID, codeExpiry, _ := f.processingWithCodeExpiring(student, title, -2*time.Hour)
	f.exec("UPDATE orders SET status = 'cancelled' WHERE id = ?", orderID)
	pay := f.paymentServiceWith(&fakeBankLookup{result: bankPaid(codeExpiry.Add(-time.Minute))}, nil)
	if _, err := pay.CheckAndProcessPayment(context.Background(), orderID, student, false); err != nil {
		f.t.Fatalf("gắn cờ hoàn tiền: %v", err)
	}
	if n := f.historyCount(orderID, latePaymentHistoryStatus); n != 1 {
		f.t.Fatalf("cờ payment_after_expiry = %d, muốn 1", n)
	}
	return orderID
}

func (f *orderFixture) adminListItem(student, orderID uuid.UUID) (needed bool, refundedAt *time.Time) {
	f.t.Helper()
	list, err := f.adminOrderService().ListOrders(context.Background(), repository.AdminOrderFilter{UserID: &student, Page: 1, Limit: 50})
	if err != nil {
		f.t.Fatalf("ListOrders: %v", err)
	}
	for _, it := range list.Items {
		if it.ID == orderID {
			return it.RefundNeeded, it.LateRefundedAt
		}
	}
	f.t.Fatalf("đơn %s không có trong danh sách admin", orderID)
	return false, nil
}

// Admin ghi nhận đã hoàn: cờ refund_needed tắt, có mốc late_refunded_at ở danh sách admin lẫn chi
// tiết đơn (học viên cũng thấy), đúng 1 dòng history late_refund_done kèm mã giao dịch hoàn.
func TestMarkLatePaymentRefunded_ClearsRefundNeededAndRecordsOnce(t *testing.T) {
	f := newOrderFixture(t)
	student := f.user()
	orderID := f.flaggedClosedOrder(student, "QA-P hoàn tiền muộn")
	admin := f.adminOrderService()
	actor := uuid.New()
	ctx := context.Background()

	if needed, at := f.adminListItem(student, orderID); !needed || at != nil {
		t.Fatalf("trước khi hoàn: refund_needed=%v late_refunded_at=%v, muốn true/nil", needed, at)
	}

	first, err := admin.MarkLatePaymentRefunded(ctx, actor, orderID, "Đã CK lại cho học viên", "FT-LATE-1")
	if err != nil || first.AlreadyRecorded || first.RefundNeeded {
		t.Fatalf("lần 1: resp=%+v err=%v, muốn ghi mới, refund_needed=false", first, err)
	}
	if needed, at := f.adminListItem(student, orderID); needed || at == nil {
		t.Fatalf("sau khi hoàn: refund_needed=%v late_refunded_at=%v, muốn false/có mốc", needed, at)
	}
	detail, err := f.svc.GetOrderByID(ctx, orderID, student, false)
	if err != nil || detail.RefundNeeded || detail.LateRefundedAt == nil {
		t.Fatalf("chi tiết đơn: %+v err=%v, muốn refund_needed=false và có late_refunded_at", detail, err)
	}

	// Idempotent: gọi lại vẫn 200, không ghi thêm dòng, trả lại đúng mốc lần đầu.
	second, err := admin.MarkLatePaymentRefunded(ctx, actor, orderID, "gọi lại", "FT-LATE-2")
	if err != nil || !second.AlreadyRecorded || !second.LateRefundedAt.Equal(first.LateRefundedAt) {
		t.Fatalf("lần 2: resp=%+v err=%v, muốn already_recorded và cùng mốc %s", second, err, first.LateRefundedAt)
	}
	if n := f.historyCount(orderID, lateRefundDoneHistoryStatus); n != 1 {
		t.Fatalf("history late_refund_done = %d, muốn đúng 1", n)
	}
	if got := f.orderStatus(orderID); got != "cancelled" {
		t.Fatalf("status đơn = %q, muốn giữ cancelled (chỉ ghi nhận hoàn, không đổi trạng thái)", got)
	}
}

// Hai admin bấm cùng lúc: khoá dòng order tuần tự hoá, chỉ một dòng history.
func TestMarkLatePaymentRefunded_ConcurrentCallsRecordOneRow(t *testing.T) {
	f := newOrderFixture(t)
	student := f.user()
	orderID := f.flaggedClosedOrder(student, "QA-P hoàn tiền muộn song song")
	admin := f.adminOrderService()

	var wg sync.WaitGroup
	errs := make([]error, 4)
	for i := range errs {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_, errs[i] = admin.MarkLatePaymentRefunded(context.Background(), uuid.New(), orderID, "", "")
		}(i)
	}
	wg.Wait()
	for i, err := range errs {
		if err != nil {
			t.Fatalf("goroutine %d: %v", i, err)
		}
	}
	if n := f.historyCount(orderID, lateRefundDoneHistoryStatus); n != 1 {
		t.Fatalf("history late_refund_done = %d, muốn đúng 1", n)
	}
}

// Đơn không có cờ tiền về muộn thì không có gì để đánh dấu; đơn không tồn tại thì not found.
func TestMarkLatePaymentRefunded_RejectsUnflaggedAndUnknownOrders(t *testing.T) {
	f := newOrderFixture(t)
	student := f.user()
	admin := f.adminOrderService()
	ctx := context.Background()

	unflagged, _, _ := f.processingWithCodeExpiring(student, "QA-P đơn không cờ", -2*time.Hour)
	f.exec("UPDATE orders SET status = 'cancelled' WHERE id = ?", unflagged)
	if _, err := admin.MarkLatePaymentRefunded(ctx, uuid.New(), unflagged, "", ""); !errors.Is(err, ErrLateRefundNotNeeded) {
		t.Fatalf("đơn không cờ: err=%v, muốn ErrLateRefundNotNeeded", err)
	}
	if n := f.historyCount(unflagged, lateRefundDoneHistoryStatus); n != 0 {
		t.Fatalf("đơn không cờ bị ghi %d dòng late_refund_done, muốn 0", n)
	}
	if _, err := admin.MarkLatePaymentRefunded(ctx, uuid.New(), uuid.New(), "", ""); !errors.Is(err, ErrOrderNotFound) {
		t.Fatalf("đơn không tồn tại: err=%v, muốn ErrOrderNotFound", err)
	}
}
