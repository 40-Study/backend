package service

// L9 (đợt MINOR cuối của review L8) trên Postgres thật, schema tạm (orderFixture):
//   1. đơn chờ lỗi dai dẳng RIÊNG TỪNG ĐƠN không được chặn đầu hàng của sweep (repro review: 3 đơn cũ lỗi + 3 đơn
//      trẻ đã có tiền, hoàn tất 0/3); ngân hàng chết hẳn vẫn ngắt mạch và vẫn chỉ tốn 3 lần gọi mỗi lượt;
//   2. payment_transaction_id dài hơn payment_code varchar(64) không được làm hỏng UPDATE hoàn tất;
//   6. đơn bị hoãn chốt expired được đếm riêng và có log info.

import (
	"bytes"
	"context"
	"fmt"
	"log"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"study.com/v1/internal/grpc"
)

// l9CodeBank mô phỏng service Python theo TỪNG mã: mã nào có trong results thì trả đúng kết quả đó, mã lạ trả "không
// thấy". Ghi lại thứ tự các mã đã hỏi (fakeBankLookup bỏ qua mã nên không dựng được lỗi riêng từng đơn).
type l9CodeBank struct {
	mu      sync.Mutex
	results map[string]*grpc.CheckTransactionResult
	asked   []string
}

func (b *l9CodeBank) CheckTransaction(_ context.Context, code string, _, _ time.Time) (*grpc.CheckTransactionResult, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.asked = append(b.asked, code)
	if r, ok := b.results[code]; ok {
		return r, nil
	}
	return bankNotFound, nil
}

func (b *l9CodeBank) IsHealthy(context.Context) (bool, error) { return true, nil }

// takeAsked trả các mã đã hỏi kể từ lần gọi trước.
func (b *l9CodeBank) takeAsked() []string {
	b.mu.Lock()
	defer b.mu.Unlock()
	out := b.asked
	b.asked = nil
	return out
}

// l9Processing: đơn chờ có mã riêng, tuổi `age` (created_at = now - age) để thứ tự created_at của sweep xác định.
func (f *orderFixture) l9Processing(student uuid.UUID, title, code string, age time.Duration) uuid.UUID {
	f.t.Helper()
	id, _, _ := f.processingWithCodeExpiring(student, title, time.Hour)
	f.exec("UPDATE orders SET payment_transaction_id = ?, payment_code = ?, created_at = ? WHERE id = ?",
		code, code, time.Now().Add(-age), id)
	return id
}

func l9Paid(code string) *grpc.CheckTransactionResult {
	return bankPaidMany(grpc.BankTransaction{TransactionID: "L9-TX-" + code, Amount: "499000", TransactionDate: bankDate(time.Now().Add(-time.Minute))})
}

// captureLog gom log chuẩn trong lúc test chạy.
func captureLog(t *testing.T) *bytes.Buffer {
	t.Helper()
	var buf bytes.Buffer
	prev := log.Writer()
	log.SetOutput(&buf)
	t.Cleanup(func() { log.SetOutput(prev) })
	return &buf
}

// Mục 1 (repro của review): 3 đơn chờ CŨ NHẤT mà ngân hàng luôn báo lỗi cho riêng mã của chúng, cộng 3 đơn trẻ hơn đã
// có tiền. Sweep đi theo created_at nên cả 3 lần lỗi nằm ở đầu hàng và ngắt mạch dừng lượt: trước đây đơn trẻ hoàn tất
// 0/3 qua mọi lượt. Nay đơn đã lỗi lượt trước xếp sau, nên lượt kế tiếp hoàn tất đủ 3 đơn trẻ, và lỗi riêng đơn
// (ngân hàng vẫn trả lời các đơn khác) không bị tính là "ngân hàng chết".
func caseTestReconcileSweep_PersistentPerOrderFailureDoesNotStarveYoungerOrders(t *testing.T, f *orderFixture) {
	f.parkExistingOrders()
	student := f.user()
	ctx := context.Background()

	results := map[string]*grpc.CheckTransactionResult{}
	var bad, good []uuid.UUID
	for i, age := range []time.Duration{40 * time.Hour, 39 * time.Hour, 38 * time.Hour} {
		code := fmt.Sprintf("L9-BAD-%d", i)
		bad = append(bad, f.l9Processing(student, "L9 đơn lỗi riêng "+code, code, age))
		results[code] = bankError
	}
	for i, age := range []time.Duration{3 * time.Hour, 2 * time.Hour, time.Hour} {
		code := fmt.Sprintf("L9-OK-%d", i)
		good = append(good, f.l9Processing(student, "L9 đơn đã có tiền "+code, code, age))
		results[code] = l9Paid(code)
	}
	bank := &l9CodeBank{results: results}
	pay := f.paymentServiceWith(bank, nil)

	completedYoung := func() int {
		n := 0
		for _, id := range good {
			if f.orderStatus(id) == "completed" {
				n++
			}
		}
		return n
	}

	res1, err := pay.ReconcileSweep(ctx, time.Now())
	if err != nil {
		t.Fatalf("sweep 1: %v", err)
	}
	res2, err := pay.ReconcileSweep(ctx, time.Now())
	if err != nil {
		t.Fatalf("sweep 2: %v", err)
	}
	if n := completedYoung(); n != 3 {
		t.Fatalf("sau 2 lượt chỉ %d/3 đơn trẻ (đã có tiền) hoàn tất: 3 đơn cũ lỗi riêng từng đơn đang chặn đầu hàng\nsweep1=%+v\nsweep2=%+v", n, res1, res2)
	}
	// Lượt 2: ngân hàng trả lời các đơn trẻ nên 3 lần lỗi của đơn cũ là lỗi RIÊNG ĐƠN, không phải ngân hàng chết.
	if res2.BankDown || res2.BankErrors != 0 || res2.OrderErrors != 3 {
		t.Fatalf("sweep 2: %+v, muốn 3 lỗi riêng đơn, 0 lỗi ngân hàng, không ngắt mạch (ngân hàng vẫn trả lời các đơn khác)", res2)
	}
	for _, id := range bad {
		if got := f.orderStatus(id); got != "processing" {
			t.Fatalf("đơn lỗi riêng %s: status=%q, muốn vẫn processing (không được chốt khi chưa xác minh được)", id, got)
		}
	}
}

// Mục 1, chiều ngược lại: ngân hàng chết HẲN (mọi đơn đều lỗi) vẫn ngắt mạch sau đúng 3 lần gọi mỗi lượt, nhưng các
// lượt kế tiếp hỏi những đơn KHÁC nhau (xoay vòng) thay vì lặp lại cùng 3 đơn đầu hàng.
func caseTestReconcileSweep_DeadBankStillTripsAndRotatesOrders(t *testing.T, f *orderFixture) {
	f.parkExistingOrders()
	student := f.user()
	ctx := context.Background()

	results := map[string]*grpc.CheckTransactionResult{}
	var codes []string
	for i := 1; i <= 7; i++ { // đơn 1 cũ nhất
		code := fmt.Sprintf("L9-DEAD-%d", i)
		codes = append(codes, code)
		f.l9Processing(student, "L9 ngân hàng chết "+code, code, time.Duration(8-i)*time.Hour)
		results[code] = bankError
	}
	bank := &l9CodeBank{results: results}
	pay := f.paymentServiceWith(bank, nil)

	want := [][]string{
		{codes[0], codes[1], codes[2]},
		{codes[3], codes[4], codes[5]}, // đơn đã lỗi xếp sau: đơn 4-6 chưa từng lỗi nên lên trước
		{codes[6], codes[0], codes[1]}, // còn đơn 7 chưa lỗi, rồi quay lại đơn lỗi lâu nhất
	}
	for i, w := range want {
		res, err := pay.ReconcileSweep(ctx, time.Now())
		if err != nil || !res.BankDown || res.BankErrors != reconcileMaxConsecutiveBankErrors {
			t.Fatalf("lượt %d: res=%+v err=%v, muốn ngắt mạch sau đúng %d lỗi ngân hàng", i+1, res, err, reconcileMaxConsecutiveBankErrors)
		}
		if got := bank.takeAsked(); fmt.Sprint(got) != fmt.Sprint(w) {
			t.Fatalf("lượt %d hỏi %v, muốn %v (xoay vòng sang đơn khác, đúng %d lần gọi)", i+1, got, w, reconcileMaxConsecutiveBankErrors)
		}
	}
}

// Mục 2: payment_code là varchar(64), payment_transaction_id varchar(255). Đơn processing chưa có payment_code mà mã
// cũ dài hơn 64: UPDATE hoàn tất trước đây lỗi SQLSTATE 22001 và rollback cả giao dịch dù khách đã trả tiền. Nay đơn
// vẫn hoàn tất, payment_code để trống và có log; đúng 64 ký tự thì vẫn chép (biên).
func caseTestCompletion_OverlongLegacyCodeDoesNotBreakUpdate(t *testing.T, f *orderFixture) {
	f.parkExistingOrders()
	student := f.user()
	ctx := context.Background()
	logs := captureLog(t)

	cases := []struct {
		name       string
		length     int
		wantCopied bool
	}{
		{"đúng 64 ký tự", 64, true},
		{"65 ký tự", 65, false},
		{"70 ký tự (repro review)", 70, false},
	}
	for _, c := range cases {
		code := "PAYLEGACY-" + strings.Repeat("X", c.length-len("PAYLEGACY-"))
		id, _, _ := f.processingWithCodeExpiring(student, "L9 mã cũ "+c.name, time.Hour)
		f.exec("UPDATE orders SET payment_transaction_id = ?, payment_code = NULL WHERE id = ?", code, id)
		txID := "L9-LONG-" + fmt.Sprint(c.length)
		bank := &codeAwareBank{code: code, result: bankPaidMany(grpc.BankTransaction{TransactionID: txID, Amount: "499000", TransactionDate: bankDate(time.Now().Add(-time.Minute))})}

		if _, err := f.paymentServiceWith(bank, nil).CheckAndProcessPayment(ctx, id, student, false); err != nil {
			t.Fatalf("%s: hoàn tất đơn lỗi: %v (UPDATE không được hỏng vì mã dài hơn cột payment_code)", c.name, err)
		}
		if got := f.orderStatus(id); got != "completed" {
			t.Fatalf("%s: status=%q, muốn completed", c.name, got)
		}
		gotTx, gotCode := f.orderCodes(id)
		if gotTx != txID {
			t.Fatalf("%s: payment_transaction_id=%q, muốn mã giao dịch %q", c.name, gotTx, txID)
		}
		wantCode := ""
		if c.wantCopied {
			wantCode = code
		}
		if gotCode != wantCode {
			t.Fatalf("%s: payment_code=%q, muốn %q", c.name, gotCode, wantCode)
		}
		logged := strings.Contains(logs.String(), "[PAYMENT-CODE] order="+id.String())
		if logged == c.wantCopied {
			t.Fatalf("%s: log bỏ qua chép mã = %v, muốn %v\n%s", c.name, logged, !c.wantCopied, logs.String())
		}
	}
}

// Mục 6: job nền hoãn chốt expired cho đơn quá hạn mà ngân hàng không trả lời; số đơn bị hoãn nằm riêng trong kết quả
// sweep và có dòng log info. Đơn chưa quá hạn chốt thì không được đếm.
func caseTestReconcileSweep_CountsDeferredExpiry(t *testing.T, f *orderFixture) {
	f.parkExistingOrders()
	student := f.user()
	ctx := context.Background()

	f.processingWithCodeExpiring(student, "L9 chưa quá hạn chốt", time.Hour)
	for i := 0; i < 2; i++ {
		f.processingWithCodeExpiring(student, "L9 quá hạn không xác minh được", -(unverifiedExpiryAfter + time.Hour))
	}
	logs := captureLog(t)
	pay := f.paymentServiceWith(&fakeBankLookup{result: bankError}, nil)

	res, err := pay.ReconcileSweep(ctx, time.Now())
	if err != nil {
		t.Fatalf("sweep: %v", err)
	}
	if res.DeferredExpiry != 2 || res.BankErrors != 3 {
		t.Fatalf("res=%+v, muốn đúng 2 đơn quá hạn bị hoãn chốt (đơn chưa quá hạn không tính) trong 3 lần ngân hàng lỗi", res)
	}

	// Đường vào của job nền ghi dòng log mức info kèm số đếm.
	pay2 := f.paymentServiceWith(&fakeBankLookup{result: bankError}, nil)
	if err := pay2.RunReconcileSweep(ctx); err != nil {
		t.Fatalf("RunReconcileSweep: %v", err)
	}
	out := logs.String()
	if !strings.Contains(out, "[PAYMENT-SWEEP] INFO: 2 đơn đã quá hạn mã") || !strings.Contains(out, "hoãn chốt=2") {
		t.Fatalf("thiếu log info về đơn bị hoãn chốt:\n%s", out)
	}

	// Không có đơn nào bị hoãn thì không có dòng info.
	f.parkExistingOrders()
	f.processingWithCodeExpiring(student, "L9 chỉ một đơn chưa quá hạn", time.Hour)
	logs.Reset()
	if err := f.paymentServiceWith(&fakeBankLookup{result: bankError}, nil).RunReconcileSweep(ctx); err != nil {
		t.Fatalf("RunReconcileSweep: %v", err)
	}
	if strings.Contains(logs.String(), "INFO") {
		t.Fatalf("không có đơn bị hoãn mà vẫn có log info:\n%s", logs.String())
	}
}

func TestPaymentL9(t *testing.T) {
	root := newOrderFixture(t)
	cases := []struct {
		name string
		fn   func(*testing.T, *orderFixture)
	}{
		{"ReconcileSweep_PersistentPerOrderFailureDoesNotStarveYoungerOrders", caseTestReconcileSweep_PersistentPerOrderFailureDoesNotStarveYoungerOrders},
		{"ReconcileSweep_DeadBankStillTripsAndRotatesOrders", caseTestReconcileSweep_DeadBankStillTripsAndRotatesOrders},
		{"Completion_OverlongLegacyCodeDoesNotBreakUpdate", caseTestCompletion_OverlongLegacyCodeDoesNotBreakUpdate},
		{"ReconcileSweep_CountsDeferredExpiry", caseTestReconcileSweep_CountsDeferredExpiry},
	}
	for _, c := range cases {
		c := c
		t.Run(c.name, func(t *testing.T) { c.fn(t, root.forT(t)) })
	}
}
