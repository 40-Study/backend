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

// fakeBankLookup — TransactionServiceInterface giả, trả kết quả/lỗi cấu hình sẵn (hoặc lần lượt
// theo seq, lặp phần tử cuối), đếm số lần gọi và ghi lại cửa sổ from/to + deadline của ctx.
type fakeBankLookup struct {
	result *grpc.CheckTransactionResult
	err    error
	seq    []*grpc.CheckTransactionResult
	calls  atomic.Int32

	mu           sync.Mutex
	lastFrom     time.Time
	lastTo       time.Time
	lastDeadline time.Time
	hadDeadline  bool
}

func (f *fakeBankLookup) CheckTransaction(ctx context.Context, paymentCode string, fromTime, toTime time.Time) (*grpc.CheckTransactionResult, error) {
	n := int(f.calls.Add(1))
	f.mu.Lock()
	f.lastFrom, f.lastTo = fromTime, toTime
	f.lastDeadline, f.hadDeadline = ctx.Deadline()
	f.mu.Unlock()
	if len(f.seq) > 0 {
		if n > len(f.seq) {
			n = len(f.seq)
		}
		return f.seq[n-1], nil
	}
	return f.result, f.err
}

func (f *fakeBankLookup) IsHealthy(ctx context.Context) (bool, error) { return true, nil }

// bankDate định dạng như mbbank ("dd/MM/yyyy HH:mm:ss", giờ Việt Nam).
func bankDate(t time.Time) string { return t.In(bankTimeZone()).Format("02/01/2006 15:04:05") }

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

// processingWithExpiredCode: đơn 499.000đ đã mở phiên thanh toán, mã hết hạn 1 phút trước (còn
// trong ân hạn đối chiếu).
func (f *orderFixture) processingWithExpiredCode(student uuid.UUID, title string) (orderID uuid.UUID, codeExpiry time.Time) {
	f.t.Helper()
	id, exp, _ := f.processingWithCodeExpiring(student, title, -time.Minute)
	return id, exp
}

// processingWithCodeExpiring: đơn processing có mã, hạn mã = now + offset (âm = đã hết hạn).
func (f *orderFixture) processingWithCodeExpiring(student uuid.UUID, title string, offset time.Duration) (orderID uuid.UUID, codeExpiry time.Time, courseID uuid.UUID) {
	f.t.Helper()
	courseID = f.course(title)
	order := f.createOrder(student, courseID)
	codeExpiry = time.Now().Add(offset).Truncate(time.Second)
	// Đơn thật tạo TRƯỚC khi cấp mã (hạn mã <= created_at + 24h): cửa sổ tra cứu cố định (vòng 4)
	// tính từ ngày cấp mã nên created_at phải nằm trước hạn mã như dữ liệu thật.
	f.exec("UPDATE orders SET status = 'processing', payment_transaction_id = 'PAYQA-FINAL', payment_code_expired_at = ?, created_at = ? WHERE id = ?",
		codeExpiry, codeExpiry.Add(-23*time.Hour), order.ID)
	return order.ID, codeExpiry, courseID
}

var (
	bankNotFound = &grpc.CheckTransactionResult{Found: false, Status: "not_found"}
	bankError    = &grpc.CheckTransactionResult{Found: false, Status: "error", ErrorMessage: "mbbank login failed"}
)

func bankPaid(at time.Time) *grpc.CheckTransactionResult {
	return &grpc.CheckTransactionResult{Found: true, Status: "success", TransactionID: "BANK-" + uuid.NewString()[:8], Amount: "499000", TransactionDate: bankDate(at)}
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
		"lỗi gRPC/timeout":         {err: errors.New("rpc error: code = DeadlineExceeded")},
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

// Quá ân hạn đối chiếu (30 phút sau hạn mã) mà ngân hàng vẫn không có giao dịch → chốt expired,
// không có cờ tiền về muộn.
func TestCheckAndProcessPayment_VerifiedNoTransactionAfterGraceExpires(t *testing.T) {
	f := newOrderFixture(t)
	student := f.user()
	orderID, _, _ := f.processingWithCodeExpiring(student, "QA-final không có tiền", -paymentReconcileGracePeriod-time.Minute)
	bank := &fakeBankLookup{result: bankNotFound}

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
// đối chiếu ngân hàng). Test dùng khoá khác nên đơn mới tạo được; cùng khoá thì đơn này chặn (vòng
// 4, TestCreateOrder_UnreconciledProcessingOrderBlocksSameCourse).
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

// ===== Review vòng 3 =====

// R1 (MAJOR 1, ân hạn 30 phút): lần poll đầu sau hạn mã ngân hàng chưa ghi có (not_found) → giữ
// processing + reconciling; ngân hàng ghi có (ngày trước hạn) ở lần poll sau → hoàn tất.
func TestCheckAndProcessPayment_DelayedBankCreditWithinGraceCompletes(t *testing.T) {
	f := newOrderFixture(t)
	student := f.user()
	orderID, codeExpiry := f.processingWithExpiredCode(student, "QA-r3 ghi có chậm")
	bank := &fakeBankLookup{seq: []*grpc.CheckTransactionResult{bankNotFound, bankPaid(codeExpiry.Add(-time.Minute))}}
	svc := f.paymentServiceWith(bank, nil)

	first, err := svc.GetPaymentStatus(context.Background(), orderID, student, false)
	if err != nil {
		t.Fatalf("poll 1: %v", err)
	}
	if first.Status != "processing" || !first.Reconciling {
		t.Fatalf("poll 1 = %+v, muốn processing + reconciling (đang trong ân hạn)", first)
	}
	if got := f.loadOrder(orderID).Status; got != "processing" {
		t.Fatalf("status DB sau poll 1 = %q, muốn processing", got)
	}

	second, err := svc.GetPaymentStatus(context.Background(), orderID, student, false)
	if err != nil {
		t.Fatalf("poll 2: %v", err)
	}
	if second.Status != "completed" || f.loadOrder(orderID).Status != "completed" {
		t.Fatalf("poll 2 = %+v (db %q), muốn completed", second, f.loadOrder(orderID).Status)
	}
}

// Tiền về SAU cả mốc ân hạn (đơn đã chốt expired) vẫn phải để lại dấu vết để hoàn tiền: hỏi lại
// đơn đã đóng → tra ngân hàng → history payment_after_expiry đúng 1 lần + cờ late_payment_received.
func TestClosedOrder_PaymentArrivingAfterGraceIsFlaggedOnce(t *testing.T) {
	f := newOrderFixture(t)
	student := f.user()
	orderID, codeExpiry, _ := f.processingWithCodeExpiring(student, "QA-r3 tiền về sau ân hạn", -paymentReconcileGracePeriod-time.Minute)
	bank := &fakeBankLookup{seq: []*grpc.CheckTransactionResult{bankNotFound, bankPaid(codeExpiry.Add(paymentReconcileGracePeriod + 30*time.Second))}}
	svc := f.paymentServiceWith(bank, nil)

	if st, err := svc.GetPaymentStatus(context.Background(), orderID, student, false); err != nil || st.Status != "expired" || st.LatePaymentReceived {
		t.Fatalf("poll 1 = %+v err=%v, muốn expired chưa có cờ", st, err)
	}
	for i := 0; i < 2; i++ {
		st, err := svc.GetPaymentStatus(context.Background(), orderID, student, false)
		if err != nil || st.Status != "expired" || !st.LatePaymentReceived {
			t.Fatalf("hỏi lại lần %d = %+v err=%v, muốn expired + late_payment_received", i+1, st, err)
		}
	}
	if n := f.historyCount(orderID, latePaymentHistoryStatus); n != 1 {
		t.Fatalf("history %s = %d, muốn đúng 1", latePaymentHistoryStatus, n)
	}
}

// R2 (MAJOR 2): cùng khoá đang có đơn processing có mã CHƯA đối chiếu (kể cả mã đã hết hạn, đã quá
// ân hạn) → CreateOrder trả 409, không mở đơn thứ hai.
func TestCreateOrder_UnreconciledProcessingOrderBlocksSameCourse(t *testing.T) {
	for name, offset := range map[string]time.Duration{
		"mã hết hạn trong ân hạn": -time.Minute,
		"mã hết hạn quá ân hạn":   -paymentReconcileGracePeriod - time.Hour,
	} {
		t.Run(name, func(t *testing.T) {
			f := newOrderFixture(t)
			student := f.user()
			orderID, _, course := f.processingWithCodeExpiring(student, "QA-r3 chặn đơn trùng", offset)

			// Vòng 4 (MINOR 2): đơn đang đối chiếu không "tiếp tục thanh toán hay huỷ" được như câu
			// ErrOrderInProgress nói, nên chặn bằng 409 ERR_PAYMENT_VERIFYING kèm câu riêng.
			_, err := f.svc.CreateOrder(context.Background(), student, buyNow(course))
			if !errors.Is(err, ErrPaymentVerificationPending) {
				t.Fatalf("err = %v, muốn ErrPaymentVerificationPending", err)
			}
			var n int64
			f.db.Model(&model.Order{}).Where("user_id = ?", student).Count(&n)
			if n != 1 || f.loadOrder(orderID).Status != "processing" {
				t.Fatalf("số đơn = %d, đơn cũ = %q; muốn 1 đơn, vẫn processing", n, f.loadOrder(orderID).Status)
			}
		})
	}
}

// R6 (MAJOR 3): huỷ đơn processing có mã phải đối chiếu ngân hàng trước.
func TestCancelOrder_ProcessingReconcilesWithBankFirst(t *testing.T) {
	type tc struct {
		offset     time.Duration // hạn mã so với bây giờ
		bank       func(codeExpiry time.Time) *fakeBankLookup
		noReconcil bool
		wantErr    error
		wantStatus string
	}
	cases := map[string]tc{
		"có tiền khớp → hoàn tất, không huỷ": {offset: time.Hour, bank: func(e time.Time) *fakeBankLookup {
			return &fakeBankLookup{result: bankPaid(time.Now().Add(-time.Minute))}
		}, wantErr: ErrPaymentAlreadyDone, wantStatus: "completed"},
		"ngân hàng lỗi → không huỷ": {offset: time.Hour, bank: func(time.Time) *fakeBankLookup { return &fakeBankLookup{result: bankError} },
			wantErr: ErrPaymentVerificationPending, wantStatus: "processing"},
		"gRPC timeout → không huỷ": {offset: time.Hour, bank: func(time.Time) *fakeBankLookup { return &fakeBankLookup{err: context.DeadlineExceeded} },
			wantErr: ErrPaymentVerificationPending, wantStatus: "processing"},
		"trong ân hạn → không huỷ": {offset: -time.Minute, bank: func(time.Time) *fakeBankLookup { return &fakeBankLookup{result: bankNotFound} },
			wantErr: ErrPaymentVerificationPending, wantStatus: "processing"},
		"không nối bộ đối chiếu → không huỷ": {offset: time.Hour, bank: func(time.Time) *fakeBankLookup { return &fakeBankLookup{result: bankNotFound} },
			noReconcil: true, wantErr: ErrPaymentVerificationPending, wantStatus: "processing"},
		"mã còn hạn, ngân hàng xác nhận chưa có tiền → huỷ được": {offset: time.Hour, bank: func(time.Time) *fakeBankLookup { return &fakeBankLookup{result: bankNotFound} },
			wantStatus: "cancelled"},
		// Vòng 4 NIT: đối chiếu trước khi huỷ vừa chốt expired (quá ân hạn, không có tiền) → báo
		// "đơn đã hết hạn" (409 ERR_ORDER_EXPIRED) thay vì 400 invalid state transition.
		"quá ân hạn, không có tiền → báo hết hạn": {offset: -paymentReconcileGracePeriod - time.Hour, bank: func(time.Time) *fakeBankLookup { return &fakeBankLookup{result: bankNotFound} },
			wantErr: ErrOrderExpired, wantStatus: "expired"},
		// Review final MINOR 3: huỷ đơn quá hạn mã 24h khi ngân hàng vẫn lỗi → đối chiếu chốt expired
		// (unverified_expiry) rồi huỷ báo hết hạn.
		"quá 24h, ngân hàng lỗi → chốt expired, báo hết hạn": {offset: -25 * time.Hour, bank: func(time.Time) *fakeBankLookup { return &fakeBankLookup{result: bankError} },
			wantErr: ErrOrderExpired, wantStatus: "expired"},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			f := newOrderFixture(t)
			student := f.user()
			orderID, exp, _ := f.processingWithCodeExpiring(student, "QA-r3 huỷ "+name, c.offset)
			bank := c.bank(exp)
			if !c.noReconcil {
				f.svc.SetPaymentReconciler(f.paymentServiceWith(bank, nil))
			}

			err := f.svc.CancelOrder(context.Background(), student, orderID, false, "QA")
			if c.wantErr == nil && err != nil || c.wantErr != nil && !errors.Is(err, c.wantErr) {
				t.Fatalf("CancelOrder err = %v, muốn %v", err, c.wantErr)
			}
			if got := f.loadOrder(orderID).Status; got != c.wantStatus {
				t.Fatalf("status DB = %q, muốn %q", got, c.wantStatus)
			}
			if !c.noReconcil && bank.calls.Load() != 1 {
				t.Fatalf("grpcCalls = %d, muốn 1 (phải đối chiếu trước khi huỷ)", bank.calls.Load())
			}
		})
	}
}

// R3 (MINOR 1): CreatePaymentIntent ánh xạ đúng kết quả đối chiếu thay vì luôn "hết hạn".
func TestCreatePaymentIntent_MapsReconcileOutcome(t *testing.T) {
	f := newOrderFixture(t)
	student := f.user()

	completed := f.createOrder(student, f.course("QA-r3 intent đã trả"))
	f.exec("UPDATE orders SET status = 'completed', paid_at = now() WHERE id = ?", completed.ID)
	svc := f.paymentServiceWith(&fakeBankLookup{result: bankNotFound}, nil)
	if _, err := svc.CreatePaymentIntent(context.Background(), student, completed.ID, false, "qr_transfer"); !errors.Is(err, ErrPaymentAlreadyDone) {
		t.Fatalf("đơn completed: err = %v, muốn ErrPaymentAlreadyDone", err)
	}

	reconciling, _ := f.processingWithExpiredCode(student, "QA-r3 intent đang đối chiếu")
	if _, err := svc.CreatePaymentIntent(context.Background(), student, reconciling, false, "qr_transfer"); !errors.Is(err, ErrPaymentVerificationPending) {
		t.Fatalf("đơn trong ân hạn: err = %v, muốn ErrPaymentVerificationPending", err)
	}
}

// Timeout gRPC + cửa sổ CỐ ĐỊNH (review vòng 4 B): mỗi lần gọi ngân hàng có deadline
// bankLookupTimeout; from = lúc cấp mã - 1 ngày; to = min(hạn mã + ân hạn + 1 ngày, bây giờ). Không
// bao giờ gửi ngày tương lai, và đơn cũ không làm cửa sổ dài thêm theo thời gian.
func TestBankLookup_HasTimeoutAndFixedWindow(t *testing.T) {
	cases := map[string]time.Duration{
		"mã vừa hết hạn (to bị chặn ở bây giờ)": -time.Minute,
		"mã hết hạn 10 ngày trước (to cố định)": -10 * 24 * time.Hour,
	}
	for name, offset := range cases {
		t.Run(name, func(t *testing.T) {
			f := newOrderFixture(t)
			student := f.user()
			orderID, codeExpiry, _ := f.processingWithCodeExpiring(student, "QA-r4 cửa sổ "+name, offset)
			bank := &fakeBankLookup{result: bankError}
			start := time.Now()
			if _, err := f.paymentServiceWith(bank, nil).CheckAndProcessPayment(context.Background(), orderID, student, false); err != nil {
				t.Fatalf("CheckAndProcessPayment: %v", err)
			}
			end := time.Now()
			bank.mu.Lock()
			defer bank.mu.Unlock()
			if !bank.hadDeadline {
				t.Fatalf("ctx gọi ngân hàng không có deadline (thiếu bankLookupTimeout)")
			}
			if d := bank.lastDeadline.Sub(start); d > bankLookupTimeout+time.Second || d < bankLookupTimeout-5*time.Second {
				t.Fatalf("deadline sau %s, muốn ~%s", d, bankLookupTimeout)
			}
			// Quyết định 2: to là CUỐI NGÀY giờ VN (23:59:59 +07), không phải now, để server gRPC chạy UTC
			// hay giờ VN đều đổi ra cùng một ngày; không bao giờ là ngày tương lai theo giờ VN.
			vn := bankTimeZone()
			toVN := bank.lastTo.In(vn)
			if toVN.Hour() != 23 || toVN.Minute() != 59 || toVN.Second() != 59 {
				t.Fatalf("to = %s, muốn 23:59:59 giờ VN", toVN)
			}
			if toVN.Format("2006-01-02") != bank.lastTo.UTC().Format("2006-01-02") {
				t.Fatalf("to theo UTC (%s) khác ngày theo giờ VN (%s): server gRPC chạy UTC sẽ tra lệch ngày", bank.lastTo.UTC(), toVN)
			}
			if toVN.Format("2006-01-02") > end.In(vn).Format("2006-01-02") {
				t.Fatalf("to = %s là ngày tương lai theo giờ VN", toVN)
			}
			fixedTo := codeExpiry.Add(paymentReconcileGracePeriod + 24*time.Hour)
			if fixedTo.Before(start) && !bank.lastTo.Equal(endOfBankDay(fixedTo)) {
				t.Fatalf("to = %s, muốn cố định cuối ngày VN của hạn mã + 30 phút + 1 ngày = %s", bank.lastTo, endOfBankDay(fixedTo))
			}
			// Fixture: created_at = hạn mã - 23h (lúc cấp mã >= created_at) → from = đầu ngày VN của
			// (created_at - 1 ngày).
			if wantFrom := startOfBankDay(codeExpiry.Add(-23 * time.Hour).Add(-24 * time.Hour)); !bank.lastFrom.Equal(wantFrom) {
				t.Fatalf("from = %s, muốn đầu ngày VN của lúc cấp mã - 1 ngày = %s", bank.lastFrom, wantFrom)
			}		})
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
