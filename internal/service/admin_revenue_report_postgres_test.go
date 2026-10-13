package service

// GetRevenueReport — ngữ nghĩa gross/refund/net. Postgres thật, schema tạm riêng (newOrderFixture).
//
// Lỗi gốc: gross chỉ cộng đơn `completed`, còn refund cộng đơn `refunded`; một đơn đã hoàn có
// status `refunded` (không còn `completed`) nên tiền của nó KHÔNG nằm trong gross, vậy mà net =
// gross − refund vẫn trừ nó một lần nữa -> net bị trừ hai lần (có thể âm).

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"study.com/v1/internal/dto"
	"study.com/v1/internal/model"
)

func (f *orderFixture) orderWith(status string, total, fee int64) {
	f.t.Helper()
	student := f.user()
	o := model.Order{
		UserID:            student,
		OrderNumber:       "QA-REV-" + uuid.NewString()[:12],
		Subtotal:          decimal.NewFromInt(total),
		TotalAmount:       decimal.NewFromInt(total),
		Status:            status,
		PlatformFeeAmount: decimal.NewFromInt(fee),
	}
	if err := f.db.Create(&o).Error; err != nil {
		f.t.Fatalf("tạo order %s: %v", status, err)
	}
}

func (f *orderFixture) revenueReport() *dto.RevenueReportResponse {
	f.t.Helper()
	rep, err := f.adminOrderService().GetRevenueReport(context.Background(), time.Now().Add(-time.Hour), time.Now().Add(time.Hour))
	if err != nil {
		f.t.Fatalf("GetRevenueReport: %v", err)
	}
	return rep
}

func wantAmount(t *testing.T, name string, got decimal.Decimal, want int64) {
	t.Helper()
	if !got.Equal(decimal.NewFromInt(want)) {
		t.Errorf("%s = %s, muốn %d", name, got, want)
	}
}

// 1 đơn completed 100k (phí 10k) + 1 đơn refunded 50k (phí đã chốt 5k, được hoàn lại khi refund):
// gross = mọi đơn đã thu tiền = 150k, refund = 50k, net = gross − refund = đúng phần completed = 100k.
// Phí nền tảng chỉ tính trên phần còn giữ lại (đơn completed — khớp teacherEarningsJoins), nên
// phần GV = net − phí = 90k.
func TestRevenueReport_RefundNotDoubleCounted(t *testing.T) {
	f := newOrderFixture(t)
	f.orderWith("completed", 100000, 10000)
	f.orderWith("refunded", 50000, 5000)

	rep := f.revenueReport()
	wantAmount(t, "gross_revenue", rep.GrossRevenue, 150000)
	wantAmount(t, "refund_amount", rep.RefundAmount, 50000)
	wantAmount(t, "net_revenue", rep.NetRevenue, 100000)
	wantAmount(t, "platform_fee_amount", rep.PlatformFeeAmount, 10000)
	wantAmount(t, "teacher_share_amount", rep.TeacherShareAmount, 90000)
	if rep.CompletedCount != 1 || rep.RefundedCount != 1 {
		t.Errorf("completed/refunded count = %d/%d, muốn 1/1", rep.CompletedCount, rep.RefundedCount)
	}
}

// Kỳ chỉ có đơn đã hoàn: net = 0 (không âm), phần GV = 0.
func TestRevenueReport_OnlyRefundedNetIsZeroNotNegative(t *testing.T) {
	f := newOrderFixture(t)
	f.orderWith("refunded", 50000, 5000)

	rep := f.revenueReport()
	wantAmount(t, "gross_revenue", rep.GrossRevenue, 50000)
	wantAmount(t, "refund_amount", rep.RefundAmount, 50000)
	wantAmount(t, "net_revenue", rep.NetRevenue, 0)
	wantAmount(t, "platform_fee_amount", rep.PlatformFeeAmount, 0)
	wantAmount(t, "teacher_share_amount", rep.TeacherShareAmount, 0)
}

// Đơn chưa/không thu tiền (pending/failed/cancelled) không vào gross.
func TestRevenueReport_UnpaidOrdersExcluded(t *testing.T) {
	f := newOrderFixture(t)
	f.orderWith("completed", 100000, 0)
	f.orderWith("pending", 70000, 0)
	f.orderWith("failed", 30000, 0)

	rep := f.revenueReport()
	wantAmount(t, "gross_revenue", rep.GrossRevenue, 100000)
	wantAmount(t, "refund_amount", rep.RefundAmount, 0)
	wantAmount(t, "net_revenue", rep.NetRevenue, 100000)
	wantAmount(t, "teacher_share_amount", rep.TeacherShareAmount, 100000)
}
