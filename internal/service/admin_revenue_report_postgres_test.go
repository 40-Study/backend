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
	"study.com/v1/internal/repository"
)

func (f *orderFixture) orderWith(status string, total, fee int64) {
	f.t.Helper()
	f.orderAt(status, decimal.NewFromInt(total), decimal.NewFromInt(fee), time.Time{})
}

// orderAt tạo 1 đơn không có item; createdAt zero = để GORM đặt "bây giờ" (autoCreateTime).
func (f *orderFixture) orderAt(status string, total, fee decimal.Decimal, createdAt time.Time) uuid.UUID {
	f.t.Helper()
	student := f.user()
	o := model.Order{
		UserID:            student,
		OrderNumber:       "QA-REV-" + uuid.NewString()[:12],
		Subtotal:          total,
		TotalAmount:       total,
		Status:            status,
		PlatformFeeAmount: fee,
		CreatedAt:         createdAt,
	}
	if err := f.db.Create(&o).Error; err != nil {
		f.t.Fatalf("tạo order %s: %v", status, err)
	}
	return o.ID
}

func (f *orderFixture) revenueReport() *dto.RevenueReportResponse {
	f.t.Helper()
	return f.revenueReportBetween(time.Now().Add(-time.Hour), time.Now().Add(time.Hour))
}

func (f *orderFixture) revenueReportBetween(from, to time.Time) *dto.RevenueReportResponse {
	f.t.Helper()
	rep, err := f.adminOrderService().GetRevenueReport(context.Background(), from, to)
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

// Ranh giới kỳ (creation-cohort, quyết định chủ dự án): kỳ báo cáo lọc theo orders.created_at cho MỌI
// số liệu, kể cả refund. Đơn tạo ngoài [from,to] bị loại; đơn tạo trong kỳ rồi hoàn SAU đó vẫn nằm ở kỳ
// tạo đơn (kỳ đó được tính lại: refund tăng, net giảm) và KHÔNG xuất hiện ở kỳ mà việc hoàn diễn ra.
func TestRevenueReport_PeriodBoundaryIsCreationCohort(t *testing.T) {
	f := newOrderFixture(t)
	now := time.Now()
	earlier := now.Add(-3 * time.Hour)
	amt := decimal.NewFromInt

	f.orderAt("completed", amt(40000), amt(4000), earlier) // tạo ngoài kỳ hiện tại
	f.orderAt("refunded", amt(30000), amt(3000), earlier)  // tạo ngoài kỳ hiện tại, hoàn "bây giờ"
	f.orderAt("completed", amt(100000), amt(10000), now)   // tạo trong kỳ
	f.orderAt("refunded", amt(50000), amt(5000), now)      // tạo trong kỳ, hoàn sau đó

	current := f.revenueReportBetween(now.Add(-time.Hour), now.Add(time.Hour))
	wantAmount(t, "current gross_revenue", current.GrossRevenue, 150000)
	wantAmount(t, "current refund_amount", current.RefundAmount, 50000)
	wantAmount(t, "current net_revenue", current.NetRevenue, 100000)
	wantAmount(t, "current platform_fee_amount", current.PlatformFeeAmount, 10000)
	wantAmount(t, "current teacher_share_amount", current.TeacherShareAmount, 90000)
	if current.TransactionCount != 2 {
		t.Errorf("current transaction_count = %d, muốn 2 (đơn tạo ngoài kỳ phải bị loại)", current.TransactionCount)
	}

	prior := f.revenueReportBetween(now.Add(-4*time.Hour), now.Add(-2*time.Hour))
	wantAmount(t, "prior gross_revenue", prior.GrossRevenue, 70000)
	wantAmount(t, "prior refund_amount", prior.RefundAmount, 30000)
	wantAmount(t, "prior net_revenue", prior.NetRevenue, 40000)
	wantAmount(t, "prior platform_fee_amount", prior.PlatformFeeAmount, 4000)
	wantAmount(t, "prior teacher_share_amount", prior.TeacherShareAmount, 36000)
}

// itemSpec: 1 dòng khoá học trong đơn (course -> giảng viên của khoá) với giá cuối final_price.
type itemSpec struct {
	course uuid.UUID
	price  string
}

// orderWithItems tạo đơn có item thật (total = Σ final_price) để đi qua teacherEarningsJoins.
func (f *orderFixture) orderWithItems(status, fee string, items ...itemSpec) {
	f.t.Helper()
	total := decimal.Zero
	for _, it := range items {
		total = total.Add(decimal.RequireFromString(it.price))
	}
	id := f.orderAt(status, total, decimal.RequireFromString(fee), time.Time{})
	for _, it := range items {
		p := decimal.RequireFromString(it.price)
		oi := model.OrderItem{OrderID: id, CourseID: it.course, Price: p, FinalPrice: p}
		if err := f.db.Create(&oi).Error; err != nil {
			f.t.Fatalf("tạo order_item: %v", err)
		}
	}
}

func (f *orderFixture) instructorOf(course uuid.UUID) uuid.UUID {
	f.t.Helper()
	var c model.Course
	if err := f.db.Select("id", "instructor_id").Where("id = ?", course).First(&c).Error; err != nil {
		f.t.Fatalf("đọc course: %v", err)
	}
	return c.InstructorID
}

// Bất biến chéo: teacher_share_amount của báo cáo admin == Σ phần giảng viên theo ví
// (WalletRepository.GetTeacherEarningsByIDs) trên cùng dữ liệu — đơn nhiều item, nhiều giảng viên,
// phí lẻ để phép chia phí theo item phải làm tròn và dồn phần dư vào item cuối.
func TestRevenueReport_TeacherShareMatchesTeacherEarnings(t *testing.T) {
	f := newOrderFixture(t)
	c1, c2, c3 := f.course("rev-c1"), f.course("rev-c2"), f.course("rev-c3")
	t1, t2 := f.instructorOf(c1), f.instructorOf(c2)
	if t1 == t2 || f.instructorOf(c3) == t1 || f.instructorOf(c3) == t2 {
		t.Fatal("fixture phải cho 3 khoá thuộc 3 giảng viên khác nhau")
	}
	t3 := f.instructorOf(c3)

	// Đơn 1: 3 item / 3 giảng viên, phí 12345.67 không chia tròn theo tỉ lệ 100000:50000:30000.
	f.orderWithItems("completed", "12345.67", itemSpec{c1, "100000"}, itemSpec{c2, "50000"}, itemSpec{c3, "30000"})
	// Đơn 2: 1 item.
	f.orderWithItems("completed", "7000", itemSpec{c2, "70000"})
	// Đơn đã hoàn và đơn chưa thanh toán: không được tính ở cả hai phía.
	f.orderWithItems("refunded", "9000", itemSpec{c1, "90000"})
	f.orderWithItems("pending", "0", itemSpec{c3, "20000"})

	earnings, err := repository.NewWalletRepository(f.db).GetTeacherEarningsByIDs([]uuid.UUID{t1, t2, t3})
	if err != nil {
		t.Fatalf("GetTeacherEarningsByIDs: %v", err)
	}
	sum := decimal.Zero
	for _, v := range earnings {
		sum = sum.Add(v)
	}

	rep := f.revenueReport()
	if !rep.TeacherShareAmount.Equal(sum) {
		t.Errorf("teacher_share_amount báo cáo = %s, Σ GetTeacherEarningsByIDs = %s (%v)", rep.TeacherShareAmount, sum, earnings)
	}
	// Con số tuyệt đối: net 250000 − phí (12345.67 + 7000) = 230654.33.
	want := decimal.RequireFromString("230654.33")
	if !rep.TeacherShareAmount.Equal(want) {
		t.Errorf("teacher_share_amount = %s, muốn %s", rep.TeacherShareAmount, want)
	}
}
