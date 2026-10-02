package database

// Test Postgres THẬT cho backfill orders.payment_code (order_payment_code_backfill.go), gọi qua
// RunPostMigrations như lúc API khởi động: bỏ lời gọi khỏi RunPostMigrations, bỏ điều kiện status (chép cả đơn
// completed, tức chép nhầm mã giao dịch ngân hàng thành mã thanh toán), hoặc bỏ điều kiện IS NULL (ghi đè mã
// đã có) thì test ĐỎ.

import (
	"testing"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"study.com/v1/internal/model"
	"study.com/v1/internal/testutil/pgtest"
)

func TestOrderPaymentCodeBackfill_RecoversOpenOrdersOnly_Idempotent(t *testing.T) {
	db := pgtest.IsolatedSchema(t, Migrate)
	user := backfillUser(t, db, "paycode")
	ptr := func(s string) *string { return &s }
	newOrder := func(status string, txID, code *string) uuid.UUID {
		o := model.Order{UserID: user, OrderNumber: "QA-PC-" + uuid.NewString()[:12], Status: status,
			Subtotal: decimal.NewFromInt(100000), TotalAmount: decimal.NewFromInt(100000),
			PaymentTransactionID: txID, PaymentCode: code}
		if err := db.Create(&o).Error; err != nil {
			t.Fatalf("tạo đơn %s: %v", status, err)
		}
		return o.ID
	}

	cases := []struct {
		name string
		id   uuid.UUID
		want *string // payment_code kỳ vọng sau backfill
	}{
		{"processing chép mã", newOrder("processing", ptr("PAYCODE-1"), nil), ptr("PAYCODE-1")},
		{"expired chép mã", newOrder("expired", ptr("PAYCODE-2"), nil), ptr("PAYCODE-2")},
		{"cancelled chép mã", newOrder("cancelled", ptr("PAYCODE-3"), nil), ptr("PAYCODE-3")},
		{"completed KHÔNG chép (đang là mã giao dịch ngân hàng)", newOrder("completed", ptr("FT-BANK-1"), nil), nil},
		{"refunded KHÔNG chép", newOrder("refunded", ptr("FT-BANK-2"), nil), nil},
		{"pending không có mã", newOrder("pending", nil, nil), nil},
		{"mã rỗng không chép", newOrder("processing", ptr(""), nil), nil},
		{"đã có payment_code thì giữ nguyên", newOrder("processing", ptr("OTHER"), ptr("KEEP-ME")), ptr("KEEP-ME")},
	}
	for run := 1; run <= 2; run++ {
		if err := RunPostMigrations(db); err != nil {
			t.Fatalf("lần %d RunPostMigrations: %v", run, err)
		}
		for _, c := range cases {
			var got model.Order
			if err := db.First(&got, "id = ?", c.id).Error; err != nil {
				t.Fatalf("đọc đơn: %v", err)
			}
			switch {
			case c.want == nil && got.PaymentCode != nil:
				t.Errorf("lần %d, %s: muốn NULL, nhận %q", run, c.name, *got.PaymentCode)
			case c.want != nil && (got.PaymentCode == nil || *got.PaymentCode != *c.want):
				t.Errorf("lần %d, %s: muốn %q, nhận %v", run, c.name, *c.want, got.PaymentCode)
			}
		}
	}
}
