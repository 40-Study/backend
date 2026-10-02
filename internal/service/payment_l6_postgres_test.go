package service

// L6 mục 3, 4, 5 trên Postgres thật, schema tạm (orderFixture):
//   3. hoàn tiền muộn theo TỪNG khoản: admin thấy số khoản/tổng tiền/mã giao dịch, bấm "đã hoàn" cho
//      một khoản không tắt cờ khoản khác, dữ liệu cũ vẫn đọc đúng;
//   4. đơn đang chờ khách chuyển hai lần: hoàn tất bằng một giao dịch, giao dịch còn lại được gắn cờ;
//   5. job nền: hoàn tất đơn chờ, bắt khoản chuyển dư của đơn vừa hoàn tất, idempotent, không chạy chồng.

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"study.com/v1/internal/dto"
	"study.com/v1/internal/grpc"
	"study.com/v1/internal/model"
	"study.com/v1/internal/repository"
)

// adminItem trả dòng của đơn trong danh sách admin (đi qua ListOrders thật).
func (f *orderFixture) adminItem(student, orderID uuid.UUID) dto.AdminOrderListItem {
	f.t.Helper()
	list, err := f.adminOrderService().ListOrders(context.Background(), repository.AdminOrderFilter{UserID: &student, Page: 1, Limit: 50})
	if err != nil {
		f.t.Fatalf("ListOrders: %v", err)
	}
	for _, it := range list.Items {
		if it.ID == orderID {
			return it
		}
	}
	f.t.Fatalf("đơn %s không có trong danh sách admin", orderID)
	return dto.AdminOrderListItem{}
}

// twoFlaggedTransfers dựng đơn đã huỷ có hai khoản tiền về muộn (499000 và 250000) qua luồng đối chiếu thật.
func (f *orderFixture) twoFlaggedTransfers(student uuid.UUID, title string) uuid.UUID {
	f.t.Helper()
	orderID, codeExpiry, _ := f.processingWithCodeExpiring(student, title, -2*time.Hour)
	f.exec("UPDATE orders SET status = 'cancelled' WHERE id = ?", orderID)
	tx1 := grpc.BankTransaction{TransactionID: "L6-A", Amount: "499000", TransactionDate: bankDate(codeExpiry.Add(-time.Minute))}
	tx2 := grpc.BankTransaction{TransactionID: "L6-B", Amount: "250000", TransactionDate: bankDate(codeExpiry.Add(-30 * time.Second))}
	if _, err := f.paymentServiceWith(&fakeBankLookup{result: bankPaidMany(tx1, tx2)}, nil).
		CheckAndProcessPayment(context.Background(), orderID, student, false); err != nil {
		f.t.Fatalf("gắn cờ hai khoản: %v", err)
	}
	return orderID
}

// Mục 3: admin thấy số khoản, tổng tiền, mã giao dịch; hoàn từng khoản thì chỉ khoản đó tắt cờ.
func caseTestAdminLateRefund_PerTransaction(t *testing.T, f *orderFixture) {
	student := f.user()
	orderID := f.twoFlaggedTransfers(student, "L6 hoàn từng khoản")
	admin := f.adminOrderService()
	ctx := context.Background()
	actor := uuid.New()

	it := f.adminItem(student, orderID)
	if !it.RefundNeeded || it.LateRefunds == nil || it.LateRefunds.PendingCount != 2 || it.LateRefunds.PendingAmount.String() != "749000" {
		t.Fatalf("danh sách admin: refund_needed=%v late_refunds=%+v, muốn 2 khoản / 749000", it.RefundNeeded, it.LateRefunds)
	}
	ids := map[string]string{}
	for _, x := range it.LateRefunds.Items {
		ids[x.TransactionID] = x.Amount
		if x.Refunded {
			t.Errorf("khoản %s chưa hoàn mà báo refunded", x.TransactionID)
		}
	}
	if ids["L6-A"] != "499000" || ids["L6-B"] != "250000" {
		t.Fatalf("mã giao dịch/số tiền = %v, muốn L6-A:499000 và L6-B:250000", ids)
	}

	// Mã không thuộc đơn: từ chối, không ghi gì.
	if _, err := admin.MarkLatePaymentRefunded(ctx, actor, orderID, "", "", []string{"L6-NOPE"}); !errors.Is(err, ErrLateRefundUnknownRef) {
		t.Fatalf("mã lạ: err=%v, muốn ErrLateRefundUnknownRef", err)
	}
	if n := f.historyCount(orderID, lateRefundDoneHistoryStatus); n != 0 {
		t.Fatalf("mã lạ mà vẫn ghi %d dòng hoàn", n)
	}

	// Hoàn riêng khoản A: khoản B vẫn cần hoàn, cờ KHÔNG tắt oan.
	resp, err := admin.MarkLatePaymentRefunded(ctx, actor, orderID, "CK lại khoản A", "FT-A", []string{"L6-A"})
	if err != nil || resp.AlreadyRecorded || !resp.RefundNeeded || resp.PendingCount != 1 || len(resp.RefundedRefs) != 1 || resp.RefundedRefs[0] != "L6-A" {
		t.Fatalf("hoàn khoản A: resp=%+v err=%v, muốn refund_needed còn 1 khoản và refunded_refs=[L6-A]", resp, err)
	}
	it = f.adminItem(student, orderID)
	if !it.RefundNeeded || it.LateRefunds.PendingCount != 1 || it.LateRefunds.PendingAmount.String() != "250000" || it.LateRefundedAt != nil {
		t.Fatalf("sau khi hoàn A: refund_needed=%v late_refunds=%+v late_refunded_at=%v, muốn còn B (250000) và chưa có mốc hoàn", it.RefundNeeded, it.LateRefunds, it.LateRefundedAt)
	}
	for _, x := range it.LateRefunds.Items {
		if (x.TransactionID == "L6-A") != x.Refunded {
			t.Fatalf("khoản %s: refunded=%v sai", x.TransactionID, x.Refunded)
		}
	}

	// Gọi lại đúng khoản A: idempotent, không ghi thêm.
	again, err := admin.MarkLatePaymentRefunded(ctx, actor, orderID, "", "", []string{"L6-A"})
	if err != nil || !again.AlreadyRecorded || !again.RefundNeeded {
		t.Fatalf("gọi lại khoản A: resp=%+v err=%v, muốn already_recorded và vẫn refund_needed", again, err)
	}
	if n := f.historyCount(orderID, lateRefundDoneHistoryStatus); n != 1 {
		t.Fatalf("history late_refund_done = %d, muốn 1", n)
	}

	// Không nêu khoản nào = hoàn mọi khoản còn chờ (tương thích client cũ).
	last, err := admin.MarkLatePaymentRefunded(ctx, actor, orderID, "CK nốt", "FT-B", nil)
	if err != nil || last.RefundNeeded || last.PendingCount != 0 || len(last.RefundedRefs) != 1 || last.RefundedRefs[0] != "L6-B" {
		t.Fatalf("hoàn nốt: resp=%+v err=%v, muốn hết cờ và refunded_refs=[L6-B]", last, err)
	}
	it = f.adminItem(student, orderID)
	if it.RefundNeeded || it.LateRefundedAt == nil || it.LateRefunds.PendingCount != 0 {
		t.Fatalf("sau khi hoàn hết: refund_needed=%v late_refunded_at=%v, muốn false và có mốc", it.RefundNeeded, it.LateRefundedAt)
	}
}

// Tương thích dữ liệu hiện có: dòng "đã hoàn" ghi theo định dạng CŨ (không liệt kê khoản) vẫn phủ mọi
// khoản gắn cờ trước nó, nhưng không phủ khoản về sau.
func caseTestAdminLateRefund_LegacyDoneRowKeepsOldMeaning(t *testing.T, f *orderFixture) {
	student := f.user()
	orderID := f.twoFlaggedTransfers(student, "L6 dữ liệu cũ")
	f.exec(`INSERT INTO order_status_histories (id, created_at, order_id, from_status, to_status, reason)
		VALUES (gen_random_uuid(), now() + interval '1 second', ?, 'cancelled', ?, 'Late payment refunded by admin x (ref=OLD): legacy')`,
		orderID, lateRefundDoneHistoryStatus)

	it := f.adminItem(student, orderID)
	if it.RefundNeeded || it.LateRefunds == nil || it.LateRefunds.PendingCount != 0 || it.LateRefundedAt == nil {
		t.Fatalf("dòng hoàn cũ phải phủ cả hai khoản: refund_needed=%v late_refunds=%+v", it.RefundNeeded, it.LateRefunds)
	}
	// Khách chuyển thêm SAU dòng hoàn cũ: khoản mới cần hoàn trở lại.
	extra := grpc.BankTransaction{TransactionID: "L6-C", Amount: "100000", TransactionDate: bankDate(time.Now())}
	all := bankPaidMany(
		grpc.BankTransaction{TransactionID: "L6-A", Amount: "499000", TransactionDate: bankDate(time.Now())},
		grpc.BankTransaction{TransactionID: "L6-B", Amount: "250000", TransactionDate: bankDate(time.Now())}, extra)
	time.Sleep(1200 * time.Millisecond) // cờ mới phải muộn hơn dòng hoàn cũ (đã đặt now()+1s)
	if _, err := f.paymentServiceWith(&fakeBankLookup{result: all}, nil).CheckAndProcessPayment(context.Background(), orderID, student, false); err != nil {
		t.Fatalf("đối chiếu có khoản mới: %v", err)
	}
	it = f.adminItem(student, orderID)
	if !it.RefundNeeded || it.LateRefunds.PendingCount != 1 || it.LateRefunds.PendingAmount.String() != "100000" {
		t.Fatalf("khoản L6-C phải còn chờ: refund_needed=%v late_refunds=%+v", it.RefundNeeded, it.LateRefunds)
	}
}

// Mục 4: đơn chờ nhận hai lần chuyển (đúng tiền cả hai, hoặc chuyển sai tiền trước): hoàn tất bằng MỘT
// giao dịch, các giao dịch còn lại bật cờ cần hoàn tiền (cùng cơ chế late-refund), không ghi trùng.
func caseTestProcessingOrder_ExtraTransferIsFlaggedForRefund(t *testing.T, f *orderFixture) {
	student := f.user()
	ctx := context.Background()
	admin := f.adminOrderService()

	orderID, _, _ := f.processingWithCodeExpiring(student, "L6 chuyển hai lần", time.Hour)
	tx1 := grpc.BankTransaction{TransactionID: "L6-X1", Amount: "499000", TransactionDate: bankDate(time.Now().Add(-2 * time.Minute))}
	tx2 := grpc.BankTransaction{TransactionID: "L6-X2", Amount: "499000", TransactionDate: bankDate(time.Now().Add(-time.Minute))}
	wrong := grpc.BankTransaction{TransactionID: "L6-X0", Amount: "100000", TransactionDate: bankDate(time.Now().Add(-3 * time.Minute))}
	pay := f.paymentServiceWith(&fakeBankLookup{result: bankPaidMany(wrong, tx1, tx2)}, nil)

	resp, err := pay.CheckAndProcessPayment(ctx, orderID, student, false)
	if err != nil || resp.Status != "completed" {
		t.Fatalf("CheckAndProcessPayment: resp=%+v err=%v, muốn completed", resp, err)
	}
	if got := f.orderStatus(orderID); got != "completed" {
		t.Fatalf("status = %q, muốn completed", got)
	}
	var usage model.BankTransactionUsage
	if err := f.db.Where("reference_id = ?", orderID).First(&usage).Error; err != nil || usage.BankTransactionID != "L6-X1" {
		t.Fatalf("giao dịch dùng hoàn tất = %q (err %v), muốn giao dịch đúng tiền đầu tiên L6-X1", usage.BankTransactionID, err)
	}
	reasons := lateFlagReasons(f, orderID)
	if len(reasons) != 2 {
		t.Fatalf("số cờ = %d (%v), muốn 2 (L6-X0 sai tiền và L6-X2 chuyển dư)", len(reasons), reasons)
	}
	mustFlagFor(t, reasons, "L6-X0")
	mustFlagFor(t, reasons, "L6-X2")
	for _, r := range reasons {
		if containsTx(r, "L6-X1") {
			t.Fatalf("giao dịch đã dùng để hoàn tất L6-X1 bị gắn cờ hoàn tiền: %s", r)
		}
	}

	// Đơn vẫn completed, nhưng admin thấy hai khoản cần hoàn và hoàn được từng khoản.
	it := f.adminItem(student, orderID)
	if it.Status != "completed" || !it.RefundNeeded || it.LateRefunds == nil || it.LateRefunds.PendingCount != 2 {
		t.Fatalf("danh sách admin: status=%s refund_needed=%v late_refunds=%+v", it.Status, it.RefundNeeded, it.LateRefunds)
	}
	if _, err := admin.MarkLatePaymentRefunded(ctx, uuid.New(), orderID, "", "FT-X2", []string{"L6-X2"}); err != nil {
		t.Fatalf("hoàn khoản dư: %v", err)
	}
	if it = f.adminItem(student, orderID); !it.RefundNeeded || it.LateRefunds.PendingCount != 1 {
		t.Fatalf("còn khoản sai tiền chưa hoàn: refund_needed=%v late_refunds=%+v", it.RefundNeeded, it.LateRefunds)
	}

	// Poll lại: đơn completed trả sớm, không ghi thêm cờ.
	if _, err := pay.CheckAndProcessPayment(ctx, orderID, student, false); err != nil {
		t.Fatalf("poll lại: %v", err)
	}
	if n := len(lateFlagReasons(f, orderID)); n != 2 {
		t.Fatalf("poll lại: số cờ = %d, muốn vẫn 2", n)
	}
}

func containsTx(reason, txID string) bool {
	return len(reason) > 0 && indexOf(reason, "transaction "+txID+" ") >= 0
}

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}

// Chi tiết đơn: admin nhận danh sách khoản (trang chi tiết admin dùng), học viên chỉ thấy cờ refund_needed
// — mã giao dịch/số tiền từng khoản là đối soát nội bộ.
func caseTestOrderDetail_LateRefundItemsOnlyForAdmin(t *testing.T, f *orderFixture) {
	student := f.user()
	orderID := f.twoFlaggedTransfers(student, "L6 chi tiết đơn")
	ctx := context.Background()

	asAdmin, err := f.svc.GetOrderByID(ctx, orderID, uuid.New(), true)
	if err != nil || !asAdmin.RefundNeeded || asAdmin.LateRefunds == nil || asAdmin.LateRefunds.PendingCount != 2 || len(asAdmin.LateRefunds.Items) != 2 {
		t.Fatalf("admin: resp=%+v err=%v, muốn refund_needed và 2 khoản", asAdmin, err)
	}
	asStudent, err := f.svc.GetOrderByID(ctx, orderID, student, false)
	if err != nil || !asStudent.RefundNeeded {
		t.Fatalf("học viên: resp=%+v err=%v, muốn vẫn thấy refund_needed", asStudent, err)
	}
	if asStudent.LateRefunds != nil {
		t.Fatalf("học viên nhận danh sách khoản tiền về muộn (nội bộ): %+v", asStudent.LateRefunds)
	}
}

// parkExistingOrders đẩy mọi đơn đang có ra ngoài cửa sổ quét của job: các ca sweep dùng chung một schema nên
// đơn của ca khác (đã completed/processing) sẽ bị quét và làm lệch số đếm của ca đang chạy.
func (f *orderFixture) parkExistingOrders() {
	f.t.Helper()
	f.exec("UPDATE orders SET created_at = now() - interval '30 days', paid_at = CASE WHEN paid_at IS NULL THEN NULL ELSE now() - interval '30 days' END")
}

// Mục 5: ReconcileSweep hoàn tất đơn chờ đã có tiền (khách đóng trình duyệt), bắt khoản chuyển dư của
// đơn vừa hoàn tất, và chạy lại không đổi gì.
func caseTestReconcileSweep_CompletesWaitingAndFlagsExtraTransfers(t *testing.T, f *orderFixture) {
	f.parkExistingOrders()
	student := f.user()
	ctx := context.Background()

	waiting, _, _ := f.processingWithCodeExpiring(student, "L6 sweep chờ", time.Hour)
	paid := grpc.BankTransaction{TransactionID: "L6-S1", Amount: "499000", TransactionDate: bankDate(time.Now().Add(-time.Minute))}
	pay := f.paymentServiceWith(&fakeBankLookup{result: bankPaidMany(paid)}, nil)

	res, err := pay.ReconcileSweep(ctx, time.Now())
	if err != nil || res.Processing < 1 || res.Errors != 0 {
		t.Fatalf("sweep 1: res=%+v err=%v, muốn đối chiếu >= 1 đơn chờ, 0 lỗi", res, err)
	}
	if got := f.orderStatus(waiting); got != "completed" {
		t.Fatalf("sau sweep: status = %q, muốn completed (job hoàn tất đơn khách đã trả)", got)
	}
	if n := len(lateFlagReasons(f, waiting)); n != 0 {
		t.Fatalf("một giao dịch duy nhất mà có %d cờ hoàn tiền", n)
	}

	// Khách chuyển THÊM sau khi đơn đã hoàn tất: sweep tra lại đơn completed và gắn cờ khoản dư.
	extra := grpc.BankTransaction{TransactionID: "L6-S2", Amount: "499000", TransactionDate: bankDate(time.Now())}
	pay = f.paymentServiceWith(&fakeBankLookup{result: bankPaidMany(paid, extra)}, nil)
	res, err = pay.ReconcileSweep(ctx, time.Now())
	if err != nil || res.ExtraFlagged != 1 {
		t.Fatalf("sweep 2: res=%+v err=%v, muốn đúng 1 khoản dư mới", res, err)
	}
	reasons := lateFlagReasons(f, waiting)
	if len(reasons) != 1 {
		t.Fatalf("số cờ = %d (%v), muốn 1", len(reasons), reasons)
	}
	mustFlagFor(t, reasons, "L6-S2")

	// Idempotent: chạy lại cùng sao kê không ghi thêm gì.
	res, err = pay.ReconcileSweep(ctx, time.Now())
	if err != nil || res.ExtraFlagged != 0 {
		t.Fatalf("sweep 3: res=%+v err=%v, muốn 0 khoản mới (idempotent)", res, err)
	}
	if n := len(lateFlagReasons(f, waiting)); n != 1 {
		t.Fatalf("sweep lặp: số cờ = %d, muốn vẫn 1", n)
	}
}

// Đơn completed không có bank_transaction_usage (đơn cũ / hoàn tất đường khác): không biết giao dịch nào
// là khoản đã trả nên KHÔNG gắn cờ nhầm cho chính khoản đã thanh toán.
func caseTestReconcileSweep_CompletedWithoutUsageIsNotFlagged(t *testing.T, f *orderFixture) {
	f.parkExistingOrders()
	student := f.user()
	orderID, _, _ := f.processingWithCodeExpiring(student, "L6 completed không usage", time.Hour)
	f.exec("UPDATE orders SET status = 'completed', paid_at = now() WHERE id = ?", orderID)
	tx := grpc.BankTransaction{TransactionID: "L6-N1", Amount: "499000", TransactionDate: bankDate(time.Now())}
	bank := &fakeBankLookup{result: bankPaidMany(tx)}

	res, err := f.paymentServiceWith(bank, nil).ReconcileSweep(context.Background(), time.Now())
	if err != nil || res.ExtraFlagged != 0 {
		t.Fatalf("res=%+v err=%v, muốn 0 cờ", res, err)
	}
	if n := len(lateFlagReasons(f, orderID)); n != 0 {
		t.Fatalf("đơn không có usage bị gắn %d cờ hoàn tiền nhầm", n)
	}
}

// Cửa sổ: đơn chờ quá cũ và đơn hoàn tất quá lâu không bị quét; không có dịch vụ ngân hàng thì bỏ qua.
func caseTestReconcileSweep_WindowAndMissingBankService(t *testing.T, f *orderFixture) {
	f.parkExistingOrders()
	student := f.user()
	ctx := context.Background()

	old, _, _ := f.processingWithCodeExpiring(student, "L6 chờ quá cũ", time.Hour)
	f.exec("UPDATE orders SET created_at = now() - interval '10 days' WHERE id = ?", old)
	stale, _, _ := f.processingWithCodeExpiring(student, "L6 hoàn tất quá lâu", time.Hour)
	f.exec("UPDATE orders SET status = 'completed', paid_at = now() - interval '2 days' WHERE id = ?", stale)
	bank := &fakeBankLookup{result: bankNotFound}
	res, err := f.paymentServiceWith(bank, nil).ReconcileSweep(ctx, time.Now())
	if err != nil {
		t.Fatalf("sweep: %v", err)
	}
	// Hai đơn nằm ngoài cửa sổ nên không được tra ngân hàng lần nào (các đơn khác đã bị đẩy ra ngoài).
	if got := bank.calls.Load(); got != 0 || res.Processing != 0 || res.Completed != 0 {
		t.Fatalf("đơn ngoài cửa sổ bị quét: bankCalls=%d res=%+v, muốn 0", got, res)
	}
	if got := f.orderStatus(old); got != "processing" {
		t.Fatalf("đơn chờ quá cũ bị đụng tới: status=%q (res=%+v)", got, res)
	}

	none := NewPaymentService(repository.NewOrderRepository(f.db), repository.NewOrderStatusHistoryRepository(f.db),
		repository.NewEnrollmentRepository(f.db), nil, nil, nil)
	if res, err := none.ReconcileSweep(ctx, time.Now()); err != nil || !res.SkippedNoBankSvc {
		t.Fatalf("không có dịch vụ ngân hàng: res=%+v err=%v, muốn bỏ qua", res, err)
	}
}

// fakeSweepLocker — SweepLocker giả: busy = true mô phỏng instance khác đang giữ khoá.
type fakeSweepLocker struct {
	busy     bool
	acquired atomic.Int32
	released atomic.Int32
}

func (l *fakeSweepLocker) TryLock(context.Context, string, time.Duration) (func(), bool, error) {
	if l.busy {
		return nil, false, nil
	}
	l.acquired.Add(1)
	return func() { l.released.Add(1) }, true, nil
}

// blockingBank chặn CheckTransaction tới khi release để dựng tình huống hai lượt quét chồng nhau.
type blockingBank struct {
	fakeBankLookup
	entered chan struct{}
	release chan struct{}
	once    sync.Once
}

func (b *blockingBank) CheckTransaction(ctx context.Context, code string, from, to time.Time) (*grpc.CheckTransactionResult, error) {
	b.once.Do(func() { close(b.entered) })
	<-b.release
	return b.fakeBankLookup.CheckTransaction(ctx, code, from, to)
}

// Không chạy chồng: khoá phân tán bận thì bỏ qua lượt (không gọi ngân hàng); lượt đang chạy thì lượt
// thứ hai trong cùng tiến trình bỏ qua; khoá luôn được nhả.
func caseTestRunReconcileSweep_DoesNotOverlap(t *testing.T, f *orderFixture) {
	f.parkExistingOrders()
	student := f.user()
	ctx := context.Background()
	f.processingWithCodeExpiring(student, "L6 khoá", time.Hour)

	// 1. Instance khác giữ khoá: bỏ qua, không đụng ngân hàng.
	bank := &fakeBankLookup{result: bankNotFound}
	pay := f.paymentServiceWith(bank, nil)
	busy := &fakeSweepLocker{busy: true}
	pay.SetSweepLocker(busy)
	if err := pay.RunReconcileSweep(ctx); err != nil || bank.calls.Load() != 0 {
		t.Fatalf("khoá bận: err=%v bankCalls=%d, muốn bỏ qua hoàn toàn", err, bank.calls.Load())
	}

	// 2. Khoá rảnh: chạy và nhả khoá.
	free := &fakeSweepLocker{}
	pay.SetSweepLocker(free)
	if err := pay.RunReconcileSweep(ctx); err != nil || bank.calls.Load() == 0 {
		t.Fatalf("khoá rảnh: err=%v bankCalls=%d, muốn có chạy", err, bank.calls.Load())
	}
	if free.acquired.Load() != 1 || free.released.Load() != 1 {
		t.Fatalf("khoá lấy=%d nhả=%d, muốn 1/1", free.acquired.Load(), free.released.Load())
	}

	// 3. Hai lượt chồng nhau trong cùng tiến trình: lượt hai bỏ qua, ngân hàng chỉ bị gọi bởi lượt một.
	blocking := &blockingBank{fakeBankLookup: fakeBankLookup{result: bankNotFound}, entered: make(chan struct{}), release: make(chan struct{})}
	pay2 := f.paymentServiceWith(blocking, nil)
	pay2.SetSweepLocker(&fakeSweepLocker{})
	done := make(chan error, 1)
	go func() { done <- pay2.RunReconcileSweep(ctx) }()
	<-blocking.entered
	// Nếu lượt hai KHÔNG bỏ qua mà chạy thật thì nó sẽ treo ở blocking.release (chưa đóng) và test timeout:
	// việc nó trả về ngay chính là bằng chứng bỏ qua.
	if err := pay2.RunReconcileSweep(ctx); err != nil {
		t.Fatalf("lượt chồng: %v", err)
	}
	close(blocking.release)
	if err := <-done; err != nil {
		t.Fatalf("lượt một: %v", err)
	}
	callsAfterFirst := blocking.calls.Load()
	if callsAfterFirst < 1 {
		t.Fatalf("lượt một không gọi ngân hàng")
	}
}

func TestPaymentL6(t *testing.T) {
	root := newOrderFixture(t)
	cases := []struct {
		name string
		fn   func(*testing.T, *orderFixture)
	}{
		{"AdminLateRefund_PerTransaction", caseTestAdminLateRefund_PerTransaction},
		{"AdminLateRefund_LegacyDoneRowKeepsOldMeaning", caseTestAdminLateRefund_LegacyDoneRowKeepsOldMeaning},
		{"OrderDetail_LateRefundItemsOnlyForAdmin", caseTestOrderDetail_LateRefundItemsOnlyForAdmin},
		{"ProcessingOrder_ExtraTransferIsFlaggedForRefund", caseTestProcessingOrder_ExtraTransferIsFlaggedForRefund},
		{"ReconcileSweep_CompletesWaitingAndFlagsExtraTransfers", caseTestReconcileSweep_CompletesWaitingAndFlagsExtraTransfers},
		{"ReconcileSweep_CompletedWithoutUsageIsNotFlagged", caseTestReconcileSweep_CompletedWithoutUsageIsNotFlagged},
		{"ReconcileSweep_WindowAndMissingBankService", caseTestReconcileSweep_WindowAndMissingBankService},
		{"RunReconcileSweep_DoesNotOverlap", caseTestRunReconcileSweep_DoesNotOverlap},
	}
	for _, c := range cases {
		c := c
		t.Run(c.name, func(t *testing.T) { c.fn(t, root.forT(t)) })
	}
}
