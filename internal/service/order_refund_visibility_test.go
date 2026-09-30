package service

// Lane U (UX-10): refund_reason là ghi chú đối soát NỘI BỘ của admin, học viên không được thấy.
// Postgres thật (fixture đơn hàng) vì OrderService đọc DB thật; bỏ hideInternalRefundFields khỏi
// GetOrderByID/GetUserOrders/GetOrderByNumber -> các ca học viên ĐỎ.

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"study.com/v1/internal/dto"
)

const internalRefundReason = "Khiếu nại của học viên Nguyễn Văn A, đã chuyển khoản hoàn qua VCB (NỘI BỘ)"

func refundedOrderFixture(t *testing.T) (*orderFixture, uuid.UUID, *dto.OrderResponse) {
	t.Helper()
	f := newOrderFixture(t)
	owner := f.user()
	order := f.createOrder(owner, f.course("QA-hoan-tien"))
	f.exec("UPDATE orders SET status = 'refunded', refund_reason = ?, refunded_at = now() WHERE id = ?", internalRefundReason, order.ID)
	return f, owner, order
}

func TestOrderRefundReason_HiddenFromStudent_ByID(t *testing.T) {
	f, owner, order := refundedOrderFixture(t)
	got, err := f.svc.GetOrderByID(context.Background(), order.ID, owner, false)
	if err != nil {
		t.Fatalf("GetOrderByID: %v", err)
	}
	if got.Status != "refunded" || got.RefundedAt == nil {
		t.Fatalf("học viên vẫn phải thấy đơn đã hoàn tiền: status=%q refunded_at=%v", got.Status, got.RefundedAt)
	}
	if got.RefundReason != nil {
		t.Fatalf("học viên thấy lý do hoàn tiền nội bộ: %q", *got.RefundReason)
	}
}

func TestOrderRefundReason_HiddenFromStudent_InList(t *testing.T) {
	f, owner, _ := refundedOrderFixture(t)
	list, err := f.svc.GetUserOrders(context.Background(), owner, 1, 10, "")
	if err != nil {
		t.Fatalf("GetUserOrders: %v", err)
	}
	if len(list.Orders) != 1 {
		t.Fatalf("muốn 1 đơn, có %d", len(list.Orders))
	}
	if list.Orders[0].RefundReason != nil {
		t.Fatalf("danh sách đơn của học viên lộ lý do hoàn tiền: %q", *list.Orders[0].RefundReason)
	}
}

func TestOrderRefundReason_HiddenFromStudent_ByNumber(t *testing.T) {
	f, _, order := refundedOrderFixture(t)
	got, err := f.svc.GetOrderByNumber(context.Background(), order.OrderNumber)
	if err != nil {
		t.Fatalf("GetOrderByNumber: %v", err)
	}
	if got.RefundReason != nil {
		t.Fatalf("GetOrderByNumber lộ lý do hoàn tiền: %q", *got.RefundReason)
	}
}

func TestOrderRefundReason_AdminStillSeesIt(t *testing.T) {
	f, _, order := refundedOrderFixture(t)
	got, err := f.svc.GetOrderByID(context.Background(), order.ID, uuid.Nil, true)
	if err != nil {
		t.Fatalf("GetOrderByID(admin): %v", err)
	}
	if got.RefundReason == nil || *got.RefundReason != internalRefundReason {
		t.Fatalf("admin phải thấy lý do hoàn tiền để đối soát, có %v", got.RefundReason)
	}
}
