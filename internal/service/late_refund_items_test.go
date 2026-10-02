package service

// L6 mục 3 — dựng danh sách khoản tiền về muộn từ history (hàm thuần, không DB). Pin: khoản nào đã
// hoàn, khoản nào còn chờ; dòng "đã hoàn" CŨ (không có tiền tố) phủ theo thời gian như trước; dòng
// MỚI chỉ phủ đúng các khoản được liệt kê.

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"study.com/v1/internal/model"
)

func flagRow(at time.Time, txID, amount string) model.OrderStatusHistory {
	return model.OrderStatusHistory{
		ID: uuid.New(), CreatedAt: at, ToStatus: latePaymentHistoryStatus,
		Reason: lateFlagReasonHead(txID, amount, "01/10/2026 10:00:00") + " for order already cancelled (payment code expired at x). Order is NOT restored; refund manually.",
	}
}

func doneRow(at time.Time, reason string) model.OrderStatusHistory {
	return model.OrderStatusHistory{ID: uuid.New(), CreatedAt: at, ToStatus: lateRefundDoneHistoryStatus, Reason: reason}
}

func TestSummarizeLateRefund(t *testing.T) {
	t0 := time.Date(2026, 10, 1, 10, 0, 0, 0, time.UTC)
	at := func(min int) time.Time { return t0.Add(time.Duration(min) * time.Minute) }

	t.Run("không có dòng nào", func(t *testing.T) {
		s := summarizeLateRefund(nil)
		if s.HasFlags() || s.Needed() {
			t.Fatalf("muốn rỗng, nhận %+v", s)
		}
	})

	t.Run("hai khoản chờ: đếm và cộng tiền", func(t *testing.T) {
		s := summarizeLateRefund([]model.OrderStatusHistory{flagRow(at(1), "TX1", "499000"), flagRow(at(2), "TX2", "250000.50")})
		if s.PendingCount != 2 || s.PendingAmount.String() != "749000.5" {
			t.Fatalf("pending=%d amount=%s, muốn 2 và 749000.5", s.PendingCount, s.PendingAmount)
		}
		if got := s.PendingRefs(); len(got) != 2 || got[0] != "TX1" || got[1] != "TX2" {
			t.Fatalf("PendingRefs = %v", got)
		}
	})

	t.Run("dòng hoàn MỚI chỉ phủ khoản được liệt kê", func(t *testing.T) {
		s := summarizeLateRefund([]model.OrderStatusHistory{
			flagRow(at(1), "TX1", "100"), flagRow(at(2), "TX2", "200"),
			doneRow(at(3), lateDoneMarker([]string{"TX1"})+"Late payment refunded by admin x (ref=R): note"),
		})
		if s.PendingCount != 1 || s.PendingRefs()[0] != "TX2" || s.PendingAmount.String() != "200" {
			t.Fatalf("muốn còn đúng TX2 (200), nhận %+v", s)
		}
		if !s.Items[0].Refunded || s.Items[0].RefundedAt == nil || s.Items[1].Refunded {
			t.Fatalf("trạng thái từng khoản sai: %+v", s.Items)
		}
	})

	t.Run("dòng hoàn CŨ phủ mọi khoản gắn cờ trước hoặc cùng lúc, không phủ khoản sau", func(t *testing.T) {
		s := summarizeLateRefund([]model.OrderStatusHistory{
			flagRow(at(1), "TX1", "100"), flagRow(at(2), "TX2", "200"),
			doneRow(at(2), "Late payment refunded by admin x (ref=R): legacy"),
			flagRow(at(5), "TX3", "300"),
		})
		if s.PendingCount != 1 || s.PendingRefs()[0] != "TX3" {
			t.Fatalf("muốn chỉ TX3 còn chờ, nhận %+v", s)
		}
	})

	t.Run("ghi chú của admin chứa chuỗi giống tiền tố không đánh lừa bộ phân tích", func(t *testing.T) {
		s := summarizeLateRefund([]model.OrderStatusHistory{
			flagRow(at(1), "TX1", "100"), flagRow(at(2), "TX2", "200"),
			doneRow(at(3), lateDoneMarker([]string{"TX1"})+"Late payment refunded by admin x (ref=[refunded_tx=TX2] ): [refunded_tx=TX2] "),
		})
		if s.PendingCount != 1 || s.PendingRefs()[0] != "TX2" {
			t.Fatalf("ghi chú tự do làm lệch phân tích: %+v", s)
		}
	})

	t.Run("cờ cũ không có mã giao dịch có khoá riêng theo id dòng", func(t *testing.T) {
		row := model.OrderStatusHistory{ID: uuid.New(), CreatedAt: at(1), ToStatus: latePaymentHistoryStatus, Reason: "something unparseable"}
		s := summarizeLateRefund([]model.OrderStatusHistory{row})
		if s.PendingCount != 1 || s.Items[0].Ref != "history:"+row.ID.String() || s.Items[0].TransactionID != "" {
			t.Fatalf("muốn khoá history:<id>, nhận %+v", s.Items)
		}
	})

	t.Run("lateRefundStateOf: đã hoàn hết thì trả mốc hoàn gần nhất", func(t *testing.T) {
		s := summarizeLateRefund([]model.OrderStatusHistory{
			flagRow(at(1), "TX1", "100"), doneRow(at(3), lateDoneMarker([]string{"TX1"})+"x"),
		})
		needed, at3 := lateRefundStateOf(s)
		if needed || at3 == nil || !at3.Equal(at(3)) {
			t.Fatalf("needed=%v refundedAt=%v, muốn false và %v", needed, at3, at(3))
		}
	})
}
