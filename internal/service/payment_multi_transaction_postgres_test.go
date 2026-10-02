package service

// L1 — khách chuyển khoản NHIỀU lần vào cùng một mã đơn. Service Python trước đây chỉ trả giao dịch
// khớp đầu tiên của sao kê nên lần chuyển thứ hai không bao giờ tới được Go và luồng cờ hoàn tiền
// muộn không bật. Giờ kết quả mang mọi giao dịch (CheckTransactionResult.Transactions) và Go xử lý
// TỪNG giao dịch với dedupe theo mã giao dịch. Postgres thật, schema tạm (orderFixture).

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"study.com/v1/internal/grpc"
	"study.com/v1/internal/model"
)

// bankPaidMany dựng kết quả tra cứu có nhiều giao dịch: field đơn lẻ = giao dịch đầu (đúng hợp đồng
// của service Python mới), Transactions = tất cả.
func bankPaidMany(txs ...grpc.BankTransaction) *grpc.CheckTransactionResult {
	first := txs[0]
	return &grpc.CheckTransactionResult{
		Found: true, Status: "success",
		TransactionID: first.TransactionID, Amount: first.Amount, Currency: first.Currency,
		Description: first.Description, TransactionDate: first.TransactionDate,
		Transactions: txs,
	}
}

func lateFlagReasons(f *orderFixture, orderID uuid.UUID) []string {
	f.t.Helper()
	var rows []model.OrderStatusHistory
	if err := f.db.Where("order_id = ? AND to_status = ?", orderID, latePaymentHistoryStatus).Find(&rows).Error; err != nil {
		f.t.Fatalf("đọc cờ hoàn tiền: %v", err)
	}
	out := make([]string, 0, len(rows))
	for _, r := range rows {
		out = append(out, r.Reason)
	}
	return out
}

func mustFlagFor(t *testing.T, reasons []string, txID string) {
	t.Helper()
	for _, r := range reasons {
		if strings.Contains(r, "transaction "+txID+" ") {
			return
		}
	}
	t.Fatalf("không có cờ hoàn tiền cho giao dịch %s trong %v", txID, reasons)
}

// Ca thật của lỗi: đơn đã huỷ, giao dịch 1 đã gắn cờ, admin đã hoàn xong; khách chuyển thêm lần 2.
// Sao kê giờ có cả hai: lần 2 phải được gắn cờ (cần hoàn tiền trở lại), lần 1 không bị ghi trùng, và
// poll lại không ghi thêm.
func caseTestCheckAndProcessPayment_SecondTransferAfterRefundIsFlagged(t *testing.T, f *orderFixture) {
	student := f.user()
	ctx := context.Background()
	orderID, codeExpiry, _ := f.processingWithCodeExpiring(student, "L1 chuyển lần 2", -2*time.Hour)
	f.exec("UPDATE orders SET status = 'cancelled' WHERE id = ?", orderID)
	tx1 := grpc.BankTransaction{TransactionID: "L1-TX1", Amount: "499000", TransactionDate: bankDate(codeExpiry.Add(-time.Minute))}
	tx2 := grpc.BankTransaction{TransactionID: "L1-TX2", Amount: "499000", TransactionDate: bankDate(codeExpiry.Add(-30 * time.Second))}

	if _, err := f.paymentServiceWith(&fakeBankLookup{result: bankPaidMany(tx1)}, nil).CheckAndProcessPayment(ctx, orderID, student, false); err != nil {
		t.Fatalf("gắn cờ lần 1: %v", err)
	}
	if _, err := f.adminOrderService().MarkLatePaymentRefunded(ctx, uuid.New(), orderID, "đã hoàn lần 1", "FT-REFUND-1"); err != nil {
		t.Fatalf("admin ghi hoàn: %v", err)
	}
	if needed, _ := lateRefundState(f.db, ptrOrder(f.loadOrder(orderID))); needed {
		t.Fatalf("sau khi admin hoàn, đơn không được còn cờ cần hoàn")
	}

	pay := f.paymentServiceWith(&fakeBankLookup{result: bankPaidMany(tx1, tx2)}, nil)
	resp, err := pay.CheckAndProcessPayment(ctx, orderID, student, false)
	if err != nil {
		t.Fatalf("đối chiếu có lần chuyển 2: %v", err)
	}
	if !resp.LatePaymentReceived {
		t.Fatalf("late_payment_received=false, muốn true khi có lần chuyển mới")
	}
	reasons := lateFlagReasons(f, orderID)
	if len(reasons) != 2 {
		t.Fatalf("số cờ = %d (%v), muốn 2 (lần 1 + lần 2, không trùng)", len(reasons), reasons)
	}
	mustFlagFor(t, reasons, "L1-TX1")
	mustFlagFor(t, reasons, "L1-TX2")
	if needed, _ := lateRefundState(f.db, ptrOrder(f.loadOrder(orderID))); !needed {
		t.Fatalf("lần chuyển 2 chưa hoàn mà đơn không báo cần hoàn tiền")
	}

	// Idempotent: admin hoàn lần 2 rồi poll lại cùng sao kê thì không có cờ mới.
	if _, err := f.adminOrderService().MarkLatePaymentRefunded(ctx, uuid.New(), orderID, "đã hoàn lần 2", "FT-REFUND-2"); err != nil {
		t.Fatalf("admin ghi hoàn 2: %v", err)
	}
	if _, err := pay.CheckAndProcessPayment(ctx, orderID, student, false); err != nil {
		t.Fatalf("poll lại: %v", err)
	}
	if n := len(lateFlagReasons(f, orderID)); n != 2 {
		t.Fatalf("poll lại cùng sao kê: số cờ = %d, muốn vẫn 2", n)
	}
}

// Đơn processing, mã còn hạn: khách chuyển sai số tiền trước rồi chuyển đúng sau. Trước đây chỉ thấy
// giao dịch đầu (sai tiền) nên đơn kẹt ErrPaymentAmountMismatch; giờ giao dịch đúng hoàn tất đơn.
func caseTestCheckAndProcessPayment_CorrectSecondTransferCompletesOrder(t *testing.T, f *orderFixture) {
	student := f.user()
	orderID, _, _ := f.processingWithCodeExpiring(student, "L1 sai tiền rồi đúng", time.Hour)
	wrong := grpc.BankTransaction{TransactionID: "L1-WRONG", Amount: "100000", TransactionDate: bankDate(time.Now().Add(-2 * time.Minute))}
	right := grpc.BankTransaction{TransactionID: "L1-RIGHT", Amount: "499000", TransactionDate: bankDate(time.Now().Add(-time.Minute))}
	bank := &fakeBankLookup{result: bankPaidMany(wrong, right)}

	resp, err := f.paymentServiceWith(bank, nil).CheckAndProcessPayment(context.Background(), orderID, student, false)
	if err != nil {
		t.Fatalf("CheckAndProcessPayment: %v", err)
	}
	if resp.Status != "completed" || f.orderStatus(orderID) != "completed" {
		t.Fatalf("status=%q db=%q, muốn completed nhờ giao dịch đúng", resp.Status, f.orderStatus(orderID))
	}
	var usage model.BankTransactionUsage
	if err := f.db.Where("reference_id = ?", orderID).First(&usage).Error; err != nil || usage.BankTransactionID != "L1-RIGHT" {
		t.Fatalf("giao dịch ghi dùng cho đơn = %q err=%v, muốn L1-RIGHT", usage.BankTransactionID, err)
	}
}

// Đơn processing đã quá hạn mã, cả hai lần chuyển đều về SAU hạn: chốt expired và cả hai lần đều có cờ.
func caseTestCheckAndProcessPayment_BothLateTransfersFlaggedWhenOrderExpires(t *testing.T, f *orderFixture) {
	student := f.user()
	orderID, codeExpiry := f.processingWithExpiredCode(student, "L1 hai lần chuyển muộn")
	tx1 := grpc.BankTransaction{TransactionID: "L1-LATE1", Amount: "499000", TransactionDate: bankDate(codeExpiry.Add(2 * time.Minute))}
	tx2 := grpc.BankTransaction{TransactionID: "L1-LATE2", Amount: "250000", TransactionDate: bankDate(codeExpiry.Add(5 * time.Minute))}

	resp, err := f.paymentServiceWith(&fakeBankLookup{result: bankPaidMany(tx1, tx2)}, nil).
		CheckAndProcessPayment(context.Background(), orderID, student, false)
	if err != nil {
		t.Fatalf("CheckAndProcessPayment: %v", err)
	}
	if resp.Status != "expired" || !resp.LatePaymentReceived {
		t.Fatalf("status=%q late=%v, muốn expired + late_payment_received", resp.Status, resp.LatePaymentReceived)
	}
	reasons := lateFlagReasons(f, orderID)
	if len(reasons) != 2 {
		t.Fatalf("số cờ = %d (%v), muốn 2", len(reasons), reasons)
	}
	mustFlagFor(t, reasons, "L1-LATE1")
	mustFlagFor(t, reasons, "L1-LATE2")
}

// Tương thích ngược: service Python cũ không gửi danh sách, chỉ các field đơn lẻ -> hành vi cũ
// (đúng một cờ cho giao dịch đó).
func caseTestCheckAndProcessPayment_OldServerWithoutTransactionListStillWorks(t *testing.T, f *orderFixture) {
	student := f.user()
	ctx := context.Background()
	orderID, codeExpiry, _ := f.processingWithCodeExpiring(student, "L1 server cũ", -2*time.Hour)
	f.exec("UPDATE orders SET status = 'cancelled' WHERE id = ?", orderID)
	old := &grpc.CheckTransactionResult{Found: true, Status: "success", TransactionID: "L1-OLD", Amount: "499000", TransactionDate: bankDate(codeExpiry.Add(-time.Minute))}

	if _, err := f.paymentServiceWith(&fakeBankLookup{result: old}, nil).CheckAndProcessPayment(ctx, orderID, student, false); err != nil {
		t.Fatalf("CheckAndProcessPayment: %v", err)
	}
	reasons := lateFlagReasons(f, orderID)
	if len(reasons) != 1 {
		t.Fatalf("số cờ = %d, muốn 1", len(reasons))
	}
	mustFlagFor(t, reasons, "L1-OLD")
}

func ptrOrder(o model.Order) *model.Order { return &o }

// TestPaymentMultiTransaction chạy các kịch bản trên MỘT schema Postgres (mỗi schema tạm tốn vài giây để migrate; package
// service đã sát giới hạn 10 phút mặc định của go test trong CI), mỗi kịch bản là một subtest độc lập.
func TestPaymentMultiTransaction(t *testing.T) {
	root := newOrderFixture(t)
	cases := []struct {
		name string
		fn   func(*testing.T, *orderFixture)
	}{
		{"CheckAndProcessPayment_SecondTransferAfterRefundIsFlagged", caseTestCheckAndProcessPayment_SecondTransferAfterRefundIsFlagged},
		{"CheckAndProcessPayment_CorrectSecondTransferCompletesOrder", caseTestCheckAndProcessPayment_CorrectSecondTransferCompletesOrder},
		{"CheckAndProcessPayment_BothLateTransfersFlaggedWhenOrderExpires", caseTestCheckAndProcessPayment_BothLateTransfersFlaggedWhenOrderExpires},
		{"CheckAndProcessPayment_OldServerWithoutTransactionListStillWorks", caseTestCheckAndProcessPayment_OldServerWithoutTransactionListStillWorks},
	}
	for _, c := range cases {
		c := c
		t.Run(c.name, func(t *testing.T) { c.fn(t, root.forT(t)) })
	}
}

// forT trả bản sao fixture gắn với *testing.T của subtest (Fatalf phải gọi trên đúng test đang chạy).
func (f *orderFixture) forT(t *testing.T) *orderFixture { c := *f; c.t = t; return &c }
