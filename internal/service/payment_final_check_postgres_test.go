package service

// Re-review #76 vòng 2 — test tích hợp Postgres THẬT cho bước đối chiếu gRPC lần cuối trước khi chốt
// "expired" (MAJOR, tiền), cùng 2 lỗ test reviewer chỉ ra (M2a: 409 khi CreatePaymentIntent thắng
// đua lúc đổi giá; M5a: hoàn lượt voucher khi đơn hết hạn). Dùng lại orderFixture (schema tạm).

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"gorm.io/gorm"
	"study.com/v1/internal/grpc"
	"study.com/v1/internal/model"
	"study.com/v1/internal/repository"
)

// fakeBankLookup — TransactionServiceInterface giả, trả kết quả/lỗi cấu hình sẵn và đếm số lần gọi.
type fakeBankLookup struct {
	result *grpc.CheckTransactionResult
	err    error
	calls  atomic.Int32
}

func (f *fakeBankLookup) CheckTransaction(ctx context.Context, paymentCode string, fromTime, toTime time.Time) (*grpc.CheckTransactionResult, error) {
	f.calls.Add(1)
	return f.result, f.err
}

func (f *fakeBankLookup) IsHealthy(ctx context.Context) (bool, error) { return true, nil }

// bankDate định dạng như mbbank ("dd/MM/yyyy HH:mm:ss", giờ Việt Nam).
func bankDate(t time.Time) string { return t.In(bankTimeZone).Format("02/01/2006 15:04:05") }

func (f *orderFixture) paymentServiceWith(bank TransactionServiceInterface, vouchers VoucherServiceInterface) *PaymentService {
	return NewPaymentService(
		repository.NewOrderRepository(f.db),
		repository.NewOrderStatusHistoryRepository(f.db),
		repository.NewEnrollmentRepository(f.db),
		vouchers,
		bank,
		nil,
	)
}

// processingWithExpiredCode: đơn 499.000đ đã mở phiên thanh toán, mã hết hạn 1 phút trước.
func (f *orderFixture) processingWithExpiredCode(student uuid.UUID, title string) (orderID uuid.UUID, codeExpiry time.Time) {
	f.t.Helper()
	order := f.createOrder(student, f.course(title))
	codeExpiry = time.Now().Add(-time.Minute).Truncate(time.Second)
	f.exec("UPDATE orders SET status = 'processing', payment_transaction_id = 'PAYQA-FINAL', payment_code_expired_at = ? WHERE id = ?", codeExpiry, order.ID)
	return order.ID, codeExpiry
}

func (f *orderFixture) historyCount(orderID uuid.UUID, toStatus string) int64 {
	var n int64
	f.db.Model(&model.OrderStatusHistory{}).Where("order_id = ? AND to_status = ?", orderID, toStatus).Count(&n)
	return n
}

// Nhánh 1: tiền khớp, ngân hàng ghi nhận TRƯỚC hạn mã, nhưng được đối chiếu lần đầu SAU hạn → hoàn
// tất đơn như bình thường (trước đây: chốt expired, không gọi gRPC, tiền mất dấu vết).
func TestCheckAndProcessPayment_PaidBeforeCodeExpiryIsCompletedEvenIfCheckedLate(t *testing.T) {
	f := newOrderFixture(t)
	student := f.user()
	orderID, codeExpiry := f.processingWithExpiredCode(student, "QA-final trả trong hạn")
	bank := &fakeBankLookup{result: &grpc.CheckTransactionResult{
		Found: true, Status: "success", TransactionID: "BANK-INTIME-" + uuid.NewString()[:8], Amount: "499000",
		TransactionDate: bankDate(codeExpiry.Add(-2 * time.Minute)),
	}}

	resp, err := f.paymentServiceWith(bank, nil).CheckAndProcessPayment(context.Background(), orderID, student, false)
	if err != nil {
		t.Fatalf("CheckAndProcessPayment: %v", err)
	}
	if resp.Status != "completed" || bank.calls.Load() != 1 {
		t.Fatalf("status=%q grpcCalls=%d, muốn completed sau đúng 1 lần đối chiếu", resp.Status, bank.calls.Load())
	}
	if got := f.loadOrder(orderID).Status; got != "completed" {
		t.Fatalf("status DB = %q, muốn completed", got)
	}
	var enrolled int64
	f.db.Model(&model.Enrollment{}).Where("user_id = ?", student).Count(&enrolled)
	if enrolled != 1 {
		t.Fatalf("enrollment = %d, muốn 1", enrolled)
	}
}

// Nhánh 2: tiền về SAU hạn mã → vẫn chốt expired, nhưng có history payment_after_expiry (admin hoàn
// tiền) và response báo late_payment_received; các lần đọc sau vẫn thấy cờ này.
func TestCheckAndProcessPayment_PaidAfterCodeExpiryIsFlaggedForRefund(t *testing.T) {
	f := newOrderFixture(t)
	student := f.user()
	orderID, codeExpiry := f.processingWithExpiredCode(student, "QA-final trả sau hạn")
	bank := &fakeBankLookup{result: &grpc.CheckTransactionResult{
		Found: true, Status: "success", TransactionID: "BANK-LATE-" + uuid.NewString()[:8], Amount: "499000",
		TransactionDate: bankDate(codeExpiry.Add(30 * time.Second)),
	}}
	svc := f.paymentServiceWith(bank, nil)

	resp, err := svc.CheckAndProcessPayment(context.Background(), orderID, student, false)
	if err != nil {
		t.Fatalf("CheckAndProcessPayment: %v", err)
	}
	if resp.Status != "expired" || !resp.LatePaymentReceived {
		t.Fatalf("resp = %+v, muốn expired + late_payment_received", resp)
	}
	if got := f.loadOrder(orderID).Status; got != "expired" {
		t.Fatalf("status DB = %q, muốn expired", got)
	}
	if n := f.historyCount(orderID, latePaymentHistoryStatus); n != 1 {
		t.Fatalf("history %s = %d dòng, muốn 1", latePaymentHistoryStatus, n)
	}

	// Poll lại (web) sau khi đã chốt: vẫn biết đã nhận tiền, không ghi cảnh báo lần 2, không gọi gRPC nữa.
	again, err := svc.GetPaymentStatus(context.Background(), orderID, student, false)
	if err != nil {
		t.Fatalf("GetPaymentStatus: %v", err)
	}
	if again.Status != "expired" || !again.LatePaymentReceived {
		t.Fatalf("poll lại = %+v, muốn expired + late_payment_received", again)
	}
	if n := f.historyCount(orderID, latePaymentHistoryStatus); n != 1 || bank.calls.Load() != 1 {
		t.Fatalf("history=%d grpcCalls=%d, muốn 1 và 1", n, bank.calls.Load())
	}
}

// Nhánh 3: không đối chiếu được (lỗi gRPC hoặc service báo status "error") → KHÔNG chốt expired,
// trả trạng thái hiện tại để lần poll sau thử lại.
func TestCheckAndProcessPayment_UnverifiableLookupNeverExpires(t *testing.T) {
	cases := map[string]*fakeBankLookup{
		"lỗi gRPC/timeout":           {err: errors.New("rpc error: code = DeadlineExceeded")},
		"service báo status=error": {result: &grpc.CheckTransactionResult{Found: false, Status: "error", ErrorMessage: "mbbank login failed"}},
	}
	for name, bank := range cases {
		t.Run(name, func(t *testing.T) {
			f := newOrderFixture(t)
			student := f.user()
			orderID, _ := f.processingWithExpiredCode(student, "QA-final "+name)

			resp, err := f.paymentServiceWith(bank, nil).CheckAndProcessPayment(context.Background(), orderID, student, false)
			if err != nil {
				t.Fatalf("CheckAndProcessPayment: %v", err)
			}
			if resp.Status != "processing" {
				t.Fatalf("status = %q, muốn giữ processing khi chưa xác minh", resp.Status)
			}
			if got := f.loadOrder(orderID).Status; got != "processing" {
				t.Fatalf("status DB = %q, muốn processing", got)
			}
			if bank.calls.Load() != 1 {
				t.Fatalf("grpcCalls = %d, muốn 1", bank.calls.Load())
			}
		})
	}
}

// Đối chứng: đã đối chiếu, không có giao dịch → chốt expired, không có cờ tiền về muộn.
func TestCheckAndProcessPayment_VerifiedNoTransactionExpires(t *testing.T) {
	f := newOrderFixture(t)
	student := f.user()
	orderID, _ := f.processingWithExpiredCode(student, "QA-final không có tiền")
	bank := &fakeBankLookup{result: &grpc.CheckTransactionResult{Found: false, Status: "not_found"}}

	resp, err := f.paymentServiceWith(bank, nil).CheckAndProcessPayment(context.Background(), orderID, student, false)
	if err != nil {
		t.Fatalf("CheckAndProcessPayment: %v", err)
	}
	if resp.Status != "expired" || resp.LatePaymentReceived || bank.calls.Load() != 1 {
		t.Fatalf("resp=%+v grpcCalls=%d, muốn expired, không cờ, 1 lần gọi", resp, bank.calls.Load())
	}
}

// Đường phụ: mở lại phiên thanh toán cho đơn processing mã đã hết hạn cũng đi qua đối chiếu cuối:
// tiền đã về trong hạn → đơn hoàn tất, không cấp mã mới.
func TestCreatePaymentIntent_ExpiredCodeGoesThroughFinalCheck(t *testing.T) {
	f := newOrderFixture(t)
	student := f.user()
	orderID, codeExpiry := f.processingWithExpiredCode(student, "QA-final intent")
	bank := &fakeBankLookup{result: &grpc.CheckTransactionResult{
		Found: true, Status: "success", TransactionID: "BANK-INTENT-" + uuid.NewString()[:8], Amount: "499000",
		TransactionDate: bankDate(codeExpiry.Add(-time.Minute)),
	}}

	_, err := f.paymentServiceWith(bank, nil).CreatePaymentIntent(context.Background(), student, orderID, false, "qr_transfer")
	if !errors.Is(err, ErrPaymentAlreadyDone) {
		t.Fatalf("err = %v, muốn ErrPaymentAlreadyDone (đơn đã được thanh toán trong hạn)", err)
	}
	if got := f.loadOrder(orderID).Status; got != "completed" {
		t.Fatalf("status DB = %q, muốn completed", got)
	}
}

// Đường phụ: lazy-sweep lúc tạo đơn mới KHÔNG được tự chốt expired đơn processing đã có mã (chưa
// đối chiếu ngân hàng); đơn đó cũng không chặn việc tạo đơn mới.
func TestCreateOrder_SweepLeavesUnverifiedProcessingOrder(t *testing.T) {
	f := newOrderFixture(t)
	student := f.user()
	orderID, _ := f.processingWithExpiredCode(student, "QA-final sweep")

	f.createOrder(student, f.course("QA-final sweep khoá khác"))
	if got := f.loadOrder(orderID).Status; got != "processing" {
		t.Fatalf("status đơn có mã sau sweep = %q, muốn processing (chờ đối chiếu)", got)
	}
}

// M2a (re-review): đúng lúc CreateOrder chuẩn bị huỷ đơn pending lệch giá thì CreatePaymentIntent
// (không giữ advisory lock) chuyển đơn đó sang processing → CreateOrder phải trả 409, không huỷ
// đơn đã có mã và không mở thêm đơn thứ hai.
func TestCreateOrder_RepriceLosesRaceToPaymentIntentIsConflict(t *testing.T) {
	f := newOrderFixture(t)
	student := f.user()
	course := f.course("QA-final M2a")
	old := f.createOrder(student, course)
	f.exec("UPDATE courses SET discount_price = 299000 WHERE id = ?", course)

	pay := f.paymentService()
	var once sync.Once
	var intentErr error
	afterOpenOrderCheckHook = func() {
		once.Do(func() {
			_, intentErr = pay.CreatePaymentIntent(context.Background(), student, old.ID, false, "qr_transfer")
		})
	}
	t.Cleanup(func() { afterOpenOrderCheckHook = nil })

	_, err := f.svc.CreateOrder(context.Background(), student, buyNow(course))
	if intentErr != nil {
		t.Fatalf("CreatePaymentIntent trong hook: %v", intentErr)
	}
	if !errors.Is(err, ErrOrderInProgress) {
		t.Fatalf("err = %v, muốn ErrOrderInProgress", err)
	}
	if got := f.loadOrder(old.ID).Status; got != "processing" {
		t.Fatalf("đơn cũ = %q, muốn processing (không được huỷ đơn đã có mã)", got)
	}
	if n := f.countOpenOrders(student); n != 1 {
		t.Fatalf("số đơn mở = %d, muốn 1", n)
	}
}

// MINOR (re-review): request khác vừa hoàn tất đơn giữa lúc đọc và lúc UPDATE expired → không được
// báo "expired" cho đơn đã thanh toán; phải đọc lại và trả trạng thái thật.
func TestExpireOrderTx_LostRaceReportsRealStatus(t *testing.T) {
	f := newOrderFixture(t)
	student := f.user()
	created := f.createOrder(student, f.course("QA-final applied=false"))
	stale := f.loadOrder(created.ID) // bản đọc lúc đơn còn pending
	f.exec("UPDATE orders SET status = 'completed', paid_at = now() WHERE id = ?", created.ID)

	expired, err := f.paymentService().expireOrderTx(context.Background(), &stale, "test")
	if err != nil {
		t.Fatalf("expireOrderTx: %v", err)
	}
	if expired || stale.Status != "completed" {
		t.Fatalf("expired=%t status=%q, muốn false và completed", expired, stale.Status)
	}
	if got := f.loadOrder(created.ID).Status; got != "completed" {
		t.Fatalf("status DB = %q, muốn completed", got)
	}
}

// countingVoucherService — chỉ ReleaseVoucherUsage được gọi trong luồng hết hạn.
type countingVoucherService struct {
	VoucherServiceInterface
	released atomic.Int32
}

func (c *countingVoucherService) ReleaseVoucherUsage(ctx context.Context, tx *gorm.DB, voucherID uuid.UUID) error {
	c.released.Add(1)
	return nil
}

// M5a (re-review): đơn có voucher quá hạn giữ → chốt expired phải hoàn lượt voucher, đúng 1 lần kể
// cả khi nhiều request đồng thời cùng phát hiện hết hạn.
func TestExpiredOrder_ReleasesVoucherExactlyOnce(t *testing.T) {
	f := newOrderFixture(t)
	student := f.user()
	order := f.createOrder(student, f.course("QA-final M5a"))
	fixed := decimal.NewFromInt(10000)
	voucher := model.Voucher{Code: "QA-" + uuid.NewString()[:8], Name: "QA voucher", DiscountUnit: model.DiscountUnitMoney, DiscountMethod: model.DiscountMethodFixed, DiscountAmountMoney: &fixed}
	if err := f.db.Create(&voucher).Error; err != nil {
		t.Fatalf("tạo voucher: %v", err)
	}
	f.exec("UPDATE orders SET voucher_id = ?, created_at = ? WHERE id = ?", voucher.ID, time.Now().Add(-30*time.Hour), order.ID)

	vouchers := &countingVoucherService{}
	svc := f.paymentServiceWith(&fakeBankLookup{}, vouchers)
	var wg sync.WaitGroup
	for i := 0; i < 6; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, _ = svc.GetPaymentStatus(context.Background(), order.ID, student, false)
		}()
	}
	wg.Wait()

	if got := f.loadOrder(order.ID).Status; got != "expired" {
		t.Fatalf("status = %q, muốn expired", got)
	}
	if n := vouchers.released.Load(); n != 1 {
		t.Fatalf("ReleaseVoucherUsage = %d lần, muốn đúng 1", n)
	}
}
