package service

// Review #76 MAJOR 2 — test tích hợp Postgres THẬT: server phải thực thi hạn giữ đơn đang hiển thị
// (orderHoldExpiresAt) ở mọi đường thanh toán, không chỉ ở lazy-sweep khi user tạo đơn mới.
// Dùng lại orderFixture (order_service_postgres_test.go): schema tạm, DROP khi xong.

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"study.com/v1/internal/model"
	"study.com/v1/internal/repository"
)

func (f *orderFixture) paymentService() *PaymentService {
	return NewPaymentService(
		repository.NewOrderRepository(f.db),
		repository.NewOrderStatusHistoryRepository(f.db),
		nil, // enrollment: các test này không đi tới fulfillment
		nil, // voucher: đơn không dùng mã giảm giá
		nil, // transaction service: không gọi gRPC cho đơn pending/quá hạn
		nil,
	)
}

func (f *orderFixture) loadOrder(orderID uuid.UUID) model.Order {
	f.t.Helper()
	var o model.Order
	if err := f.db.Where("id = ?", orderID).First(&o).Error; err != nil {
		f.t.Fatalf("đọc order: %v", err)
	}
	return o
}

// Đơn pending quá hạn giữ 24h: không mở được phiên thanh toán, không được cấp mã/hạn mới, và đơn
// chốt "expired" (có history) để mọi màn hình thấy cùng một trạng thái.
func TestCreatePaymentIntent_StalePendingIsRefusedAndExpired(t *testing.T) {
	f := newOrderFixture(t)
	student := f.user()
	order := f.createOrder(student, f.course("QA-order hạn giữ intent"))
	f.exec("UPDATE orders SET created_at = ? WHERE id = ?", time.Now().Add(-30*time.Hour), order.ID)

	resp, err := f.paymentService().CreatePaymentIntent(context.Background(), student, order.ID, false, "qr_transfer")
	if !errors.Is(err, ErrOrderExpired) {
		t.Fatalf("CreatePaymentIntent trên đơn quá hạn: resp=%+v err=%v, muốn ErrOrderExpired", resp, err)
	}

	stored := f.loadOrder(order.ID)
	if stored.Status != "expired" {
		t.Fatalf("status sau khi từ chối = %q, muốn expired", stored.Status)
	}
	if stored.PaymentTransactionID != nil || stored.PaymentCodeExpiredAt != nil {
		t.Fatalf("đơn quá hạn vẫn được cấp mã %v hạn %v", stored.PaymentTransactionID, stored.PaymentCodeExpiredAt)
	}
	var history int64
	f.db.Model(&model.OrderStatusHistory{}).Where("order_id = ? AND to_status = 'expired'", order.ID).Count(&history)
	if history != 1 {
		t.Fatalf("history expired = %d dòng, muốn 1", history)
	}

	// Gọi lại lần nữa (đơn đã expired) vẫn là cùng lỗi nghiệp vụ, không phải invalid state chung.
	if _, err := f.paymentService().CreatePaymentIntent(context.Background(), student, order.ID, false, "qr_transfer"); !errors.Is(err, ErrOrderExpired) {
		t.Fatalf("gọi lại trên đơn đã expired: err=%v, muốn ErrOrderExpired", err)
	}
}

// Đối chứng: đơn pending còn hạn vẫn mở phiên thanh toán bình thường.
func TestCreatePaymentIntent_FreshPendingStillOpens(t *testing.T) {
	f := newOrderFixture(t)
	student := f.user()
	order := f.createOrder(student, f.course("QA-order còn hạn intent"))

	resp, err := f.paymentService().CreatePaymentIntent(context.Background(), student, order.ID, false, "qr_transfer")
	if err != nil {
		t.Fatalf("CreatePaymentIntent đơn còn hạn: %v", err)
	}
	if resp.PaymentCode == "" {
		t.Fatalf("không có payment code")
	}
	if got := f.loadOrder(order.ID).Status; got != "processing" {
		t.Fatalf("status = %q, muốn processing", got)
	}
}

// Quyết định chủ dự án 28/09: hạn mã thanh toán không vượt hạn giữ đơn. Đơn còn 2h hạn giữ → mã
// hết hạn đúng created_at + 24h (còn ~2h), không phải now + 24h.
func TestCreatePaymentIntent_CodeExpiryCappedAtOrderHold(t *testing.T) {
	f := newOrderFixture(t)
	student := f.user()
	order := f.createOrder(student, f.course("QA-order hạn mã theo hạn giữ"))
	f.exec("UPDATE orders SET created_at = ? WHERE id = ?", time.Now().Add(-22*time.Hour), order.ID)
	holdDeadline := f.dbCreatedAt(order.ID).Add(pendingOrderDefaultTTL)

	resp, err := f.paymentService().CreatePaymentIntent(context.Background(), student, order.ID, false, "qr_transfer")
	if err != nil {
		t.Fatalf("CreatePaymentIntent: %v", err)
	}
	if !resp.ExpiredAt.Equal(holdDeadline) {
		t.Fatalf("hạn mã trả về = %s, muốn đúng hạn giữ đơn %s (còn %s, không phải now+24h)", resp.ExpiredAt, holdDeadline, time.Until(holdDeadline).Round(time.Minute))
	}
	if remaining := time.Until(resp.ExpiredAt); remaining > 2*time.Hour || remaining < 2*time.Hour-5*time.Minute {
		t.Fatalf("mã còn %s, muốn ~2h", remaining)
	}
	stored := f.loadOrder(order.ID)
	if stored.PaymentCodeExpiredAt == nil || !stored.PaymentCodeExpiredAt.Equal(holdDeadline) {
		t.Fatalf("payment_code_expired_at lưu = %v, muốn %s", stored.PaymentCodeExpiredAt, holdDeadline)
	}
	// expires_at hiển thị sau khi mở phiên (processing) phải giữ nguyên mốc, không lùi thêm.
	r, err := f.svc.GetOrderByID(context.Background(), order.ID, student, false)
	if err != nil {
		t.Fatalf("GetOrderByID: %v", err)
	}
	if r.ExpiresAt == nil || !r.ExpiresAt.Equal(holdDeadline) {
		t.Fatalf("expires_at hiển thị = %v, muốn %s", r.ExpiresAt, holdDeadline)
	}
}

// Luồng poll/đối chiếu (web gọi GetPaymentStatus): đơn pending quá hạn giữ phải chốt "expired",
// không trả "pending" mãi như trước.
func TestGetPaymentStatus_StalePendingBecomesExpired(t *testing.T) {
	f := newOrderFixture(t)
	student := f.user()
	order := f.createOrder(student, f.course("QA-order hạn giữ poll"))
	f.exec("UPDATE orders SET created_at = ? WHERE id = ?", time.Now().Add(-30*time.Hour), order.ID)

	status, err := f.paymentService().GetPaymentStatus(context.Background(), order.ID, student, false)
	if err != nil {
		t.Fatalf("GetPaymentStatus: %v", err)
	}
	if status.Status != "expired" {
		t.Fatalf("payment-status = %q, muốn expired", status.Status)
	}
	if got := f.loadOrder(order.ID).Status; got != "expired" {
		t.Fatalf("status DB = %q, muốn expired", got)
	}
}
