package repository

// Tính năng đơn hàng+hoàn tiền+doanh thu nền tảng (quyết định chủ dự án 27/09/2026): test DryRun
// cho các câu SQL then chốt của luồng hoàn tiền + chốt phí nền tảng — cùng pattern
// enrollment_progress_lock_sql_test.go (DummyDialector, không cần Postgres thật). Test race THẬT
// (2 request hoàn cùng lúc) cần Postgres thật + FOR UPDATE, xem báo cáo kiểm sống trên cổng riêng
// (lane-rules.md "Test sống") — không thể tái hiện an toàn bằng DryRun (không có kết nối thật để
// khoá hàng).

import (
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"gorm.io/gorm"
	gormtests "gorm.io/gorm/utils/tests"
)

// capturedStatement giữ cả câu SQL THÔ (placeholder "?", chưa thay giá trị) lẫn danh sách Vars đã
// bind — GORM in SQL kèm giá trị thật ra log (SLOW SQL) khi query chậm, nhưng d.Statement.SQL chỉ
// bao giờ chứa placeholder; so khớp literal ("status = 'completed'") trên đó luôn thất bại. Kiểm
// tra ĐÚNG bằng cách đọc riêng Vars.
type capturedStatement struct {
	sql  string
	vars []interface{}
}

func dryRunOrderRepo(t *testing.T) (*OrderRepository, *capturedStatement) {
	t.Helper()
	db, err := gorm.Open(gormtests.DummyDialector{}, &gorm.Config{})
	if err != nil {
		t.Fatalf("gorm.Open: %v", err)
	}
	captured := &capturedStatement{}
	capture := func(d *gorm.DB) {
		captured.sql = d.Statement.SQL.String()
		captured.vars = append([]interface{}(nil), d.Statement.Vars...)
	}
	if err := db.Callback().Update().After("gorm:update").Register("test:capture_update", capture); err != nil {
		t.Fatal(err)
	}
	if err := db.Callback().Query().After("gorm:query").Register("test:capture_query", capture); err != nil {
		t.Fatal(err)
	}
	return &OrderRepository{db: db.Session(&gorm.Session{DryRun: true})}, captured
}

// containsVar — true nếu want xuất hiện (dạng chuỗi, so sánh bằng fmt.Sprint) trong vars.
func containsVar(vars []interface{}, want string) bool {
	for _, v := range vars {
		if s, ok := v.(string); ok && s == want {
			return true
		}
	}
	return false
}

// TestBuildRefundQuery_ConditionalOnCompletedStatus (phase-02 contract): idempotency chống
// double-refund PHẢI đi qua WHERE "status = 'completed'" — thiếu điều kiện này thì gọi lần 2 sẽ
// UPDATE đè lên đơn ĐÃ "refunded" (ghi refunded_at/refunded_by lần 2), sai hoàn toàn ý nghĩa
// "idempotent theo đúng nghĩa: bấm 2 lần chỉ 1 lần hoàn". Core lock chống race THẬT là
// GetForUpdate (SELECT ... FOR UPDATE, gọi trước hàm này trong cùng transaction, xem
// AdminOrderService.RefundOrder) — test này chỉ pin hình dạng câu UPDATE, không thay thế được
// test race thật trên Postgres.
func TestBuildRefundQuery_ConditionalOnCompletedStatus(t *testing.T) {
	repo, captured := dryRunOrderRepo(t)
	orderID := uuid.New()
	actorID := uuid.New()
	_ = repo.buildRefundQuery(orderID, "ly do", "manual_bank_transfer", "FT-QA-1", time.Now(), actorID)

	// Cấu trúc câu lệnh: phải có ĐIỀU KIỆN status trong WHERE (không phải UPDATE vô điều kiện).
	if !strings.Contains(captured.sql, "WHERE id = ? AND status = ?") {
		t.Fatalf("SQL thieu WHERE id = ? AND status = ? (chong double-refund):\n%s", captured.sql)
	}
	for _, col := range []string{"refund_reason", "refund_method", "refund_transaction_ref", "refunded_at", "refunded_by", "status"} {
		if !strings.Contains(captured.sql, col) {
			t.Fatalf("SQL thieu cot %q:\n%s", col, captured.sql)
		}
	}
	// Giá trị bind THẬT: WHERE phải so với "completed" (nguồn), SET status phải là "refunded" (đích).
	if !containsVar(captured.vars, "completed") {
		t.Fatalf("WHERE khong bind gia tri 'completed' — vars: %v", captured.vars)
	}
	if !containsVar(captured.vars, "refunded") {
		t.Fatalf("SET status khong bind gia tri 'refunded' — vars: %v", captured.vars)
	}
}

// TestRefundOrder_ZeroRowsAffectedMeansAlreadyRefunded: RowsAffected=0 (đơn không còn ở
// "completed" — đã bị refund trước đó, hoặc chưa từng completed) phải map applied=false, KHÔNG
// phải lỗi im lặng — AdminOrderService dựa vào applied=false để trả ErrOrderAlreadyRefunded.
func TestRefundOrder_ZeroRowsAffectedMeansAlreadyRefunded(t *testing.T) {
	repo, _ := dryRunOrderRepo(t)
	// DummyDialector không Exec thật -> RowsAffected luôn 0 trên DryRun, đúng nhánh cần kiểm.
	applied, err := repo.RefundOrder(uuid.New(), "ly do", "manual_bank_transfer", "FT-QA-1", time.Now(), uuid.New())
	if err != nil {
		t.Fatalf("khong ky vong loi tren DryRun: %v", err)
	}
	if applied {
		t.Fatalf("DryRun RowsAffected phai la 0 -> applied phai la false")
	}
}

// TestSetPlatformFeeSnapshot_UpdatesBothColumns (quyết định #2): chốt phí nền tảng phải ghi CẢ
// percent lẫn amount cùng lúc — thiếu 1 trong 2 cột thì báo cáo doanh thu (SUM platform_fee_amount)
// và audit trail (platform_fee_percent) lệch nhau cho cùng 1 đơn.
func TestSetPlatformFeeSnapshot_UpdatesBothColumns(t *testing.T) {
	repo, captured := dryRunOrderRepo(t)
	_ = repo.SetPlatformFeeSnapshot(uuid.New(), decimal.NewFromInt(5), decimal.NewFromInt(50000))
	for _, want := range []string{"platform_fee_percent", "platform_fee_amount"} {
		if !strings.Contains(captured.sql, want) {
			t.Fatalf("SQL thieu cot %q:\n%s", want, captured.sql)
		}
	}
}

// TestListAdmin_FiltersAppliedToWhere: mỗi tiêu chí filter (status/user_id/from/to) phải thật
// sự xuất hiện trong WHERE khi được truyền — filter "rơi mất" là lỗi âm thầm (admin lọc theo
// status nhưng API vẫn trả toàn bộ, không ai phát hiện vì response vẫn 200 hợp lệ).
func TestListAdmin_FiltersAppliedToWhere(t *testing.T) {
	repo, captured := dryRunOrderRepo(t)
	userID := uuid.New()
	from := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	to := time.Date(2026, 9, 30, 0, 0, 0, 0, time.UTC)

	_, _, _ = repo.ListAdmin(AdminOrderFilter{
		Status: "completed",
		UserID: &userID,
		From:   &from,
		To:     &to,
		Page:   1,
		Limit:  20,
	})

	for _, want := range []string{"status = ", "user_id = ", "created_at >= ", "created_at <= "} {
		if !strings.Contains(captured.sql, want) {
			t.Fatalf("WHERE thieu dieu kien %q (filter bi roi mat):\n%s", want, captured.sql)
		}
	}
	if !containsVar(captured.vars, "completed") {
		t.Fatalf("WHERE status khong bind 'completed' — vars: %v", captured.vars)
	}
}
