package service

// L10: ghim hai hành vi của sổ lỗi sweep (payment_sweep_failures.go) mà review L9 chỉ ra là không test nào bắt
// (đột biến M2 và M7 sống sót). Mỗi test phải ĐỎ khi áp đột biến tương ứng:
//   - M2: bỏ quota reconcileMaxKnownFailingPerSweep (điều kiện `knownFailingTried >= ...` trong skipKnownFailing);
//   - M7: bỏ s.sweepFailures.clear(id) trong closure answered.

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/google/uuid"
	"study.com/v1/internal/grpc"
)

// M2: ngân hàng đang trả lời bình thường nhưng 6 đơn đã lỗi ở lượt trước vẫn lỗi. Mỗi lượt chỉ được thử đúng
// reconcileMaxKnownFailingPerSweep đơn trong số đó, phần còn lại bị để sang lượt sau (SkippedKnownFailing) và KHÔNG gọi
// ngân hàng. Bỏ quota thì cả 6 đơn bị gọi (7 lần gọi thay vì 4) và SkippedKnownFailing = 0.
func caseTestReconcileSweep_KnownFailingOrdersCappedPerSweep(t *testing.T, f *orderFixture) {
	f.parkExistingOrders()
	student := f.user()
	ctx := context.Background()

	const knownFailing = 6
	if knownFailing <= reconcileMaxKnownFailingPerSweep {
		t.Fatalf("test hỏng: cần nhiều đơn lỗi hơn quota %d", reconcileMaxKnownFailingPerSweep)
	}
	results := map[string]*grpc.CheckTransactionResult{}
	ids := make([]uuid.UUID, 0, knownFailing)
	for i := 0; i < knownFailing; i++ {
		code := fmt.Sprintf("L10-BAD-%d", i)
		ids = append(ids, f.l9Processing(student, "L10 đơn đã lỗi "+code, code, time.Duration(40-i)*time.Hour))
		results[code] = bankError
	}
	// Một đơn chưa từng lỗi, ngân hàng trả lời được: nó xếp trước và chứng minh ngân hàng còn sống trong lượt.
	results["L10-OK"] = bankNotFound
	f.l9Processing(student, "L10 đơn khoẻ", "L10-OK", time.Hour)

	bank := &l9CodeBank{results: results}
	pay := f.paymentServiceWith(bank, nil)
	base := time.Now().Add(-time.Hour)
	for i, id := range ids { // đơn i lỗi cách đây (knownFailing-i) phút: đơn 0 lâu nhất nên được thử trước
		pay.sweepFailures.recordFailure(id, base.Add(time.Duration(i)*time.Minute))
	}

	res, err := pay.ReconcileSweep(ctx, time.Now())
	if err != nil {
		t.Fatalf("sweep: %v", err)
	}
	asked := bank.takeAsked()
	wantAsked := 1 + reconcileMaxKnownFailingPerSweep // đơn khoẻ + đúng quota đơn đã lỗi
	if len(asked) != wantAsked {
		t.Fatalf("hỏi ngân hàng %d lần %v, muốn %d (1 đơn khoẻ + quota %d đơn đã lỗi): quota bị bỏ", len(asked), asked, wantAsked, reconcileMaxKnownFailingPerSweep)
	}
	if res.SkippedKnownFailing != knownFailing-reconcileMaxKnownFailingPerSweep {
		t.Fatalf("SkippedKnownFailing = %d, muốn %d (res=%+v)", res.SkippedKnownFailing, knownFailing-reconcileMaxKnownFailingPerSweep, res)
	}
	if res.OrderErrors != reconcileMaxKnownFailingPerSweep || res.BankDown || res.BankErrors != 0 {
		t.Fatalf("res=%+v, muốn %d lỗi riêng đơn, 0 lỗi ngân hàng, không ngắt mạch", res, reconcileMaxKnownFailingPerSweep)
	}
	// Đơn lỗi lâu nhất được thử trước (xoay vòng): ba mã đầu sau đơn khoẻ.
	for i := 0; i < reconcileMaxKnownFailingPerSweep; i++ {
		if want := fmt.Sprintf("L10-BAD-%d", i); asked[1+i] != want {
			t.Fatalf("thứ tự hỏi %v, muốn đơn lỗi lâu nhất trước (%s ở vị trí %d)", asked, want, 1+i)
		}
	}
}

// M7: khi ngân hàng trả lời được một đơn từng lỗi thì đơn đó phải rời sổ lỗi; đơn vẫn lỗi thì ở lại. Bỏ `clear` thì
// đơn đã hồi phục bị coi là "đang lỗi" mãi: bị xếp sau, chịu quota và không còn tính vào ngắt mạch.
func caseTestReconcileSweep_AnsweredOrderLeavesFailureLedger(t *testing.T, f *orderFixture) {
	f.parkExistingOrders()
	student := f.user()
	ctx := context.Background()

	recovered := f.l9Processing(student, "L10 đơn hồi phục", "L10-RECOVERED", 3*time.Hour)
	stillBad := f.l9Processing(student, "L10 đơn vẫn lỗi", "L10-STILLBAD", 2*time.Hour)
	bank := &l9CodeBank{results: map[string]*grpc.CheckTransactionResult{
		"L10-RECOVERED": bankNotFound, // ngân hàng trả lời được (chưa có tiền, còn hạn): đơn vẫn processing
		"L10-STILLBAD":  bankError,
	}}
	pay := f.paymentServiceWith(bank, nil)
	pay.sweepFailures.recordFailure(recovered, time.Now().Add(-time.Hour))

	if _, err := pay.ReconcileSweep(ctx, time.Now()); err != nil {
		t.Fatalf("sweep: %v", err)
	}
	if got := f.orderStatus(recovered); got != "processing" {
		t.Fatalf("đơn hồi phục status=%q, muốn vẫn processing (nếu không test này không còn chứng minh được gì)", got)
	}
	if pay.sweepFailures.has(recovered) {
		t.Fatalf("đơn được ngân hàng trả lời vẫn nằm trong sổ lỗi: clear bị bỏ")
	}
	if !pay.sweepFailures.has(stillBad) {
		t.Fatalf("đơn ngân hàng vẫn báo lỗi phải được ghi vào sổ lỗi")
	}
}

func TestPaymentL10(t *testing.T) {
	root := newOrderFixture(t)
	cases := []struct {
		name string
		fn   func(*testing.T, *orderFixture)
	}{
		{"ReconcileSweep_KnownFailingOrdersCappedPerSweep", caseTestReconcileSweep_KnownFailingOrdersCappedPerSweep},
		{"ReconcileSweep_AnsweredOrderLeavesFailureLedger", caseTestReconcileSweep_AnsweredOrderLeavesFailureLedger},
	}
	for _, c := range cases {
		c := c
		t.Run(c.name, func(t *testing.T) { c.fn(t, root.forT(t)) })
	}
}
