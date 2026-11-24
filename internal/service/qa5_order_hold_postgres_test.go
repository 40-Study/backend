package service

// QA5 S5-03 (contract C2): đơn huỷ chưa trả tiền chỉ chặn mua lại 30 phút TÍNH TỪ LÚC HUỶ (trước đây tới hạn mã
// 24 giờ + 30 phút), và 409 mang mốc hết chặn (retry_after_at).

import (
	"context"
	"errors"
	"testing"
	"time"

)

func TestCancelledOrderHold_EndsThirtyMinutesAfterCancelNotAfterCodeExpiry(t *testing.T) {
	f := newOrderFixture(t)
	student := f.user()
	// Mã chuyển khoản còn hạn 23 giờ: theo luật cũ, đơn huỷ này chặn mua lại tới hạn mã + 30 phút.
	orderID, _, course := f.processingWithCodeExpiring(student, "QA5 S5-03 hold từ lúc huỷ", 23*time.Hour)
	f.svc.SetPaymentReconciler(f.paymentServiceWith(&fakeBankLookup{result: bankNotFound}, nil))
	ctx := context.Background()
	cancelledAt := time.Now()
	if err := f.svc.CancelOrder(ctx, student, orderID, false, "QA5"); err != nil {
		t.Fatalf("CancelOrder: %v", err)
	}

	_, err := f.svc.CreateOrder(ctx, student, buyNow(course))
	if !errors.Is(err, ErrPreviousOrderVerifying) {
		t.Fatalf("ngay sau khi huỷ: err = %v, muốn ErrPreviousOrderVerifying", err)
	}
	var withRetry interface{ RetryAfter() (time.Time, bool) }
	if !errors.As(err, &withRetry) {
		t.Fatalf("lỗi không mang RetryAfter: %T", err)
	}
	at, ok := withRetry.RetryAfter()
	if !ok {
		t.Fatalf("hold do đơn huỷ phải có mốc retry_after_at")
	}
	if want := cancelledAt.Add(paymentReconcileGracePeriod); at.Sub(want) > time.Minute || want.Sub(at) > time.Minute {
		t.Fatalf("retry_after_at = %s, muốn ~%s (huỷ + 30 phút)", at, want)
	}

	// Lùi mốc huỷ 31 phút (mã vẫn còn hạn ~23 giờ): hết chặn, mua lại được.
	past := time.Now().Add(-31 * time.Minute)
	f.exec("UPDATE order_status_histories SET created_at = ? WHERE order_id = ? AND to_status = 'cancelled'", past, orderID)
	f.exec("UPDATE orders SET updated_at = ? WHERE id = ?", past, orderID)
	if _, err := f.svc.CreateOrder(ctx, student, buyNow(course)); err != nil {
		t.Fatalf("31 phút sau khi huỷ: CreateOrder = %v, muốn tạo được", err)
	}
}

// Đơn processing đang đối chiếu vẫn chặn KHÔNG kèm mốc (hết khi đối chiếu xong, không theo đồng hồ).
func TestProcessingOrderHold_HasNoRetryAfter(t *testing.T) {
	f := newOrderFixture(t)
	student := f.user()
	_, _, course := f.processingWithCodeExpiring(student, "QA5 S5-03 processing", -time.Minute)
	_, err := f.svc.CreateOrder(context.Background(), student, buyNow(course))
	if !errors.Is(err, ErrPreviousOrderVerifying) {
		t.Fatalf("err = %v, muốn ErrPreviousOrderVerifying", err)
	}
	var withRetry interface{ RetryAfter() (time.Time, bool) }
	if errors.As(err, &withRetry) {
		if _, ok := withRetry.RetryAfter(); ok {
			t.Fatalf("đơn processing không được có retry_after_at")
		}
	}
}

