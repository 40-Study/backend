package service

// L8 (đợt dọn cuối của L6) trên Postgres thật, schema tạm (orderFixture):
//   1. đường ghi payment_code của UpdatePaymentCode có test hồi quy (xoá dòng ghi cột thì đỏ);
//   3. deploy cuốn chiếu: đơn có payment_code NULL không mất mã khi hoàn tất;
//   4. đơn completed không hỏi ngân hàng không được đặt lại bộ đếm lỗi liên tiếp của ngắt mạch;
//   5. job nền không tự chốt expired "chưa xác minh" khi ngân hàng không trả lời.

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"study.com/v1/internal/grpc"
	"study.com/v1/internal/repository"
)

// Mục 1: cấp mã qua đường thật (CreatePaymentIntent -> UpdatePaymentCode), mã phải nằm ở CẢ HAI cột ngay lúc
// cấp, lần cấp thứ hai bị từ chối và không đổi mã, và sau khi hoàn tất payment_code vẫn là mã thanh toán trong
// khi payment_transaction_id đã thành mã giao dịch ngân hàng.
//
// Kiểm cột NGAY SAU KHI cấp (không chỉ sau khi hoàn tất): từ mục 3, bước hoàn tất tự chép mã sang payment_code
// nếu cột còn trống, nên chỉ kiểm sau hoàn tất thì xoá dòng ghi cột khỏi UpdatePaymentCode vẫn xanh.
func caseTestUpdatePaymentCode_WritesColumnAndSurvivesCompletion(t *testing.T, f *orderFixture) {
	student := f.user()
	ctx := context.Background()
	order := f.createOrder(student, f.course("L8 ghi payment_code"))

	bank := &codeAwareBank{}
	pay := f.paymentServiceWith(bank, nil)
	intent, err := pay.CreatePaymentIntent(ctx, student, order.ID, false, "qr_transfer")
	if err != nil || intent.PaymentCode == "" {
		t.Fatalf("CreatePaymentIntent: intent=%+v err=%v", intent, err)
	}
	issued := intent.PaymentCode
	if txID, code := f.orderCodes(order.ID); txID != issued || code != issued {
		t.Fatalf("sau khi cấp mã: payment_transaction_id=%q payment_code=%q, muốn cả hai = %q", txID, code, issued)
	}

	// Lần cấp thứ hai (race hoặc gọi lại) bị từ chối ở tầng repository và KHÔNG đổi mã đã phát cho khách.
	repo := repository.NewOrderRepository(f.db)
	if err := repo.UpdatePaymentCode(order.ID, "L8-OTHER-CODE", time.Now().Add(time.Hour)); !errors.Is(err, repository.ErrOrderConflict) {
		t.Fatalf("UpdatePaymentCode lần hai: err=%v, muốn ErrOrderConflict", err)
	}
	if txID, code := f.orderCodes(order.ID); txID != issued || code != issued {
		t.Fatalf("sau lần cấp bị từ chối: payment_transaction_id=%q payment_code=%q, muốn vẫn là %q", txID, code, issued)
	}
	// Đường service cũng trả lại ĐÚNG mã cũ (idempotent), không sinh mã mới.
	again, err := pay.CreatePaymentIntent(ctx, student, order.ID, false, "qr_transfer")
	if err != nil || again.PaymentCode != issued {
		t.Fatalf("CreatePaymentIntent lần hai: %+v err=%v, muốn cùng mã %q", again, err, issued)
	}

	// Hoàn tất bằng một giao dịch ngân hàng: payment_transaction_id bị ghi đè, payment_code phải còn nguyên.
	bank.code = issued
	bank.result = bankPaidMany(grpc.BankTransaction{TransactionID: "L8-BANK-1", Amount: "499000", TransactionDate: bankDate(time.Now().Add(-time.Minute))})
	if _, err := pay.CheckAndProcessPayment(ctx, order.ID, student, false); err != nil {
		t.Fatalf("hoàn tất đơn: %v", err)
	}
	if txID, code := f.orderCodes(order.ID); txID != "L8-BANK-1" || code != issued {
		t.Fatalf("sau hoàn tất: payment_transaction_id=%q payment_code=%q, muốn mã giao dịch L8-BANK-1 và mã thanh toán %q còn nguyên", txID, code, issued)
	}
}

// Mục 3: instance cũ (chưa biết cột payment_code) cấp mã chỉ vào payment_transaction_id trong lúc triển khai cuốn
// chiếu. Hoàn tất đơn đó phải chép mã sang payment_code TRƯỚC khi ghi đè payment_transaction_id, để job đối chiếu
// còn tra lại được khoản chuyển dư; mã đã có thì không bị đụng.
func caseTestCompletion_CopiesLegacyCodeBeforeOverwrite(t *testing.T, f *orderFixture) {
	f.parkExistingOrders()
	student := f.user()
	ctx := context.Background()

	legacy, _, _ := f.processingWithCodeExpiring(student, "L8 đơn do instance cũ cấp mã", time.Hour)
	f.exec("UPDATE orders SET payment_transaction_id = 'PAYOLD-0001', payment_code = NULL WHERE id = ?", legacy)
	kept, _, _ := f.processingWithCodeExpiring(student, "L8 đơn đã có payment_code", time.Hour)
	f.exec("UPDATE orders SET payment_transaction_id = 'PAYSTALE-X', payment_code = 'PAYKEEP-0002' WHERE id = ?", kept)

	paid := grpc.BankTransaction{TransactionID: "L8-C1", Amount: "499000", TransactionDate: bankDate(time.Now().Add(-time.Minute))}
	bankFor := func(code string) TransactionServiceInterface {
		return &codeAwareBank{code: code, result: bankPaidMany(paid)}
	}
	if _, err := f.paymentServiceWith(bankFor("PAYOLD-0001"), nil).CheckAndProcessPayment(ctx, legacy, student, false); err != nil {
		t.Fatalf("hoàn tất đơn cũ: %v", err)
	}
	if txID, code := f.orderCodes(legacy); txID != "L8-C1" || code != "PAYOLD-0001" {
		t.Fatalf("đơn cũ sau hoàn tất: payment_transaction_id=%q payment_code=%q, muốn mã giao dịch L8-C1 và payment_code PAYOLD-0001 (mã thanh toán không được mất)", txID, code)
	}

	// Đơn đã có payment_code: bất biến, KHÔNG bị thay bằng payment_transaction_id.
	paid2 := grpc.BankTransaction{TransactionID: "L8-C2", Amount: "499000", TransactionDate: bankDate(time.Now().Add(-time.Minute))}
	if _, err := f.paymentServiceWith(&codeAwareBank{code: "PAYKEEP-0002", result: bankPaidMany(paid2)}, nil).CheckAndProcessPayment(ctx, kept, student, false); err != nil {
		t.Fatalf("hoàn tất đơn đã có mã: %v", err)
	}
	if txID, code := f.orderCodes(kept); txID != "L8-C2" || code != "PAYKEEP-0002" {
		t.Fatalf("đơn đã có mã sau hoàn tất: payment_transaction_id=%q payment_code=%q, muốn L8-C2 và PAYKEEP-0002 giữ nguyên", txID, code)
	}

	// Hệ quả thật: job quét tra được đơn cũ bằng mã thanh toán và bắt khoản chuyển dư.
	extra := grpc.BankTransaction{TransactionID: "L8-C3", Amount: "499000", TransactionDate: bankDate(time.Now())}
	bank := &codeAwareBank{code: "PAYOLD-0001", result: bankPaidMany(paid, extra)}
	res, err := f.paymentServiceWith(bank, nil).ReconcileSweep(ctx, time.Now())
	if err != nil || res.ExtraFlagged != 1 {
		t.Fatalf("sweep: res=%+v err=%v (mã hỏi ngân hàng: %v), muốn 1 khoản dư của đơn cũ được gắn cờ", res, err, bank.askedCodes())
	}
	mustFlagFor(t, lateFlagReasons(f, legacy), "L8-C3")
}

// completedWithUsage đưa đơn về completed có mã thanh toán; withUsage = false mô phỏng đơn hoàn tất không có bản
// ghi bank_transaction_usage (đơn cũ / hoàn tất đường khác), sweep không hỏi ngân hàng cho đơn đó.
func (f *orderFixture) completedWithUsage(student uuid.UUID, title string, paidAgo time.Duration, withUsage bool) uuid.UUID {
	f.t.Helper()
	id, _, _ := f.processingWithCodeExpiring(student, title, time.Hour)
	f.exec("UPDATE orders SET status = 'completed', paid_at = ?, payment_transaction_id = ? WHERE id = ?",
		time.Now().Add(-paidAgo), "L8-FT-"+id.String()[:8], id)
	if withUsage {
		f.exec("INSERT INTO bank_transaction_usages (bank_transaction_id, reference_type, reference_id, created_at) VALUES (?, 'order', ?, now())",
			"L8-FT-"+id.String()[:8], id)
	}
	return id
}

// Mục 4: sweep xử lý đơn chờ trước rồi tới đơn hoàn tất theo paid_at tăng dần. Chuỗi: lỗi, lỗi (đơn chờ), rồi một
// đơn completed KHÔNG có usage (không hỏi ngân hàng), rồi lỗi. Đơn không hỏi ngân hàng không được đặt lại bộ đếm,
// nên lỗi thứ ba (liên tiếp theo các lần HỎI ngân hàng) phải mở ngắt mạch và đơn completed cuối không bị hỏi.
func caseTestReconcileSweep_UnaskedCompletedOrderDoesNotResetBreaker(t *testing.T, f *orderFixture) {
	f.parkExistingOrders()
	student := f.user()
	f.processingWithCodeExpiring(student, "L8 chờ 1", time.Hour)
	f.processingWithCodeExpiring(student, "L8 chờ 2", time.Hour)
	f.completedWithUsage(student, "L8 hoàn tất không usage", 3*time.Hour, false)
	f.completedWithUsage(student, "L8 hoàn tất 2", 2*time.Hour, true)
	f.completedWithUsage(student, "L8 hoàn tất 3", time.Hour, true)

	bank := &fakeBankLookup{result: bankError}
	res, err := f.paymentServiceWith(bank, nil).ReconcileSweep(context.Background(), time.Now())
	if err != nil {
		t.Fatalf("sweep: %v", err)
	}
	if !res.BankDown || res.BankErrors != reconcileMaxConsecutiveBankErrors || bank.calls.Load() != int32(reconcileMaxConsecutiveBankErrors) {
		t.Fatalf("res=%+v calls=%d, muốn ngắt mạch sau đúng %d lần hỏi ngân hàng lỗi (đơn hoàn tất không hỏi ngân hàng không được xoá chuỗi lỗi)",
			res, bank.calls.Load(), reconcileMaxConsecutiveBankErrors)
	}
}

// Đối chứng: đơn không hỏi ngân hàng KHÔNG làm hỏng chiều ngược lại. Một lần ngân hàng trả lời được (kể cả "không
// thấy giao dịch") vẫn đặt lại bộ đếm.
func caseTestReconcileSweep_BankAnswerStillResetsBreaker(t *testing.T, f *orderFixture) {
	f.parkExistingOrders()
	student := f.user()
	f.completedWithUsage(student, "L8 hoàn tất a", 4*time.Hour, true)
	f.completedWithUsage(student, "L8 hoàn tất b", 3*time.Hour, true)
	f.completedWithUsage(student, "L8 hoàn tất c", 2*time.Hour, true)
	f.completedWithUsage(student, "L8 hoàn tất d", time.Hour, true)
	f.completedWithUsage(student, "L8 hoàn tất e", 30*time.Minute, true)
	// lỗi, lỗi, trả lời (không thấy), lỗi, lỗi: không lần nào đủ 3 lỗi liên tiếp.
	bank := &fakeBankLookup{seq: []*grpc.CheckTransactionResult{bankError, bankError, bankNotFound, bankError, bankError}}
	res, err := f.paymentServiceWith(bank, nil).ReconcileSweep(context.Background(), time.Now())
	if err != nil || res.BankDown || res.BankErrors != 4 || bank.calls.Load() != 5 {
		t.Fatalf("res=%+v err=%v calls=%d, muốn quét đủ 5 đơn, 4 lỗi ngân hàng, không ngắt mạch", res, err, bank.calls.Load())
	}
}

// Mục 5: ngân hàng không trả lời và đơn đã quá hạn mã hơn 24h. Người dùng bấm kiểm tra vẫn được chốt expired
// "chưa xác minh" (quyết định D), còn job nền KHÔNG được chốt: đơn giữ nguyên processing, không có history
// unverified_expiry, lượt sau thử lại. Trước đây mỗi lượt vẫn tự chốt tối đa 3 đơn.
func caseTestReconcileSweep_DoesNotAutoExpireWhileBankIsDown(t *testing.T, f *orderFixture) {
	f.parkExistingOrders()
	student := f.user()
	ctx := context.Background()
	var orders []uuid.UUID
	for i := 0; i < 4; i++ {
		id, _, _ := f.processingWithCodeExpiring(student, "L8 quá hạn không xác minh được", -(unverifiedExpiryAfter + time.Hour))
		orders = append(orders, id)
	}

	bank := &fakeBankLookup{result: bankError}
	pay := f.paymentServiceWith(bank, nil)
	res, err := pay.ReconcileSweep(ctx, time.Now())
	if err != nil || !res.BankDown || res.BankErrors != reconcileMaxConsecutiveBankErrors {
		t.Fatalf("sweep: res=%+v err=%v, muốn ngắt mạch sau %d lỗi ngân hàng", res, err, reconcileMaxConsecutiveBankErrors)
	}
	for _, id := range orders {
		if got := f.orderStatus(id); got != "processing" {
			t.Fatalf("đơn %s: status=%q, muốn vẫn processing (job nền không được tự chốt expired khi chưa xác minh được)", id, got)
		}
		if n := f.historyCount(id, unverifiedExpiryHistoryStatus); n != 0 {
			t.Fatalf("đơn %s có %d dòng history %s do job nền ghi", id, n, unverifiedExpiryHistoryStatus)
		}
	}

	// Đối chứng: cùng đơn, người dùng bấm kiểm tra thì vẫn được chốt (không kẹt vĩnh viễn).
	resp, err := pay.CheckAndProcessPayment(ctx, orders[0], student, false)
	if err != nil || resp.Status != "expired" || !resp.BankUnavailable {
		t.Fatalf("người dùng kiểm tra: resp=%+v err=%v, muốn expired (quyết định D vẫn giữ cho luồng người dùng)", resp, err)
	}
	if n := f.historyCount(orders[0], unverifiedExpiryHistoryStatus); n != 1 {
		t.Fatalf("history %s = %d, muốn 1", unverifiedExpiryHistoryStatus, n)
	}
}

func TestPaymentL8(t *testing.T) {
	root := newOrderFixture(t)
	cases := []struct {
		name string
		fn   func(*testing.T, *orderFixture)
	}{
		{"UpdatePaymentCode_WritesColumnAndSurvivesCompletion", caseTestUpdatePaymentCode_WritesColumnAndSurvivesCompletion},
		{"Completion_CopiesLegacyCodeBeforeOverwrite", caseTestCompletion_CopiesLegacyCodeBeforeOverwrite},
		{"ReconcileSweep_UnaskedCompletedOrderDoesNotResetBreaker", caseTestReconcileSweep_UnaskedCompletedOrderDoesNotResetBreaker},
		{"ReconcileSweep_BankAnswerStillResetsBreaker", caseTestReconcileSweep_BankAnswerStillResetsBreaker},
		{"ReconcileSweep_DoesNotAutoExpireWhileBankIsDown", caseTestReconcileSweep_DoesNotAutoExpireWhileBankIsDown},
	}
	for _, c := range cases {
		c := c
		t.Run(c.name, func(t *testing.T) { c.fn(t, root.forT(t)) })
	}
}
