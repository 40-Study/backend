package service

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/google/uuid"
	"gorm.io/gorm"
	gormtests "gorm.io/gorm/utils/tests"
)

// TestReserveVoucherUsage_ConditionalGuard (M3-06, review vòng 4) — ReserveVoucherUsage/
// ReleaseVoucherUsage (H2-05, review vòng 3) là 2 hàm rủi ro cao nhất của commit theo review
// vòng 4 (SQL có điều kiện + map RowsAffected -> lỗi) nhưng KHÔNG có test nào. Dùng lại đúng mẫu
// DummyDialector + DryRun + gọi hàm build*Query production thật (không hand-roll lại query
// trong test) đã dùng cho buildIncrementUsedCountQuery/buildRestoreAndReactivateQuery.
func TestReserveVoucherUsage_ConditionalGuard(t *testing.T) {
	db, err := gorm.Open(gormtests.DummyDialector{}, &gorm.Config{})
	if err != nil {
		t.Fatalf("failed to open dummy gorm db: %v", err)
	}
	dryRunDB := db.Session(&gorm.Session{DryRun: true})
	vs := &VoucherService{}

	dryRun := vs.buildReserveVoucherUsageQuery(context.Background(), dryRunDB, uuid.New())
	sql := dryRun.Statement.SQL.String()

	if !strings.Contains(sql, "UPDATE") {
		t.Fatalf("expected UPDATE statement, got: %s", sql)
	}
	if !strings.Contains(sql, "usage_limit") || !strings.Contains(sql, "used_count <") {
		t.Errorf("expected usage_limit/used_count guard condition in WHERE clause, got SQL: %s", sql)
	}
	if !strings.Contains(sql, "used_count + 1") {
		t.Errorf("expected atomic increment expression, got SQL: %s", sql)
	}
}

// TestReserveVoucherUsage_ReturnsErrOnZeroRows (M3-06, review vòng 4) — gọi ĐÚNG
// ReserveVoucherUsage thật trên DryRun DB: DryRun không thực thi câu UPDATE thật nên
// RowsAffected LUÔN = 0 (không có kết nối DB thật để trả số dòng ảnh hưởng) — nhánh "hết lượt"
// CHẮC CHẮN được thực thi một cách xác định. Xóa nhánh RowsAffected==0 trong ReserveVoucherUsage
// sẽ làm test này đỏ.
func TestReserveVoucherUsage_ReturnsErrOnZeroRows(t *testing.T) {
	db, err := gorm.Open(gormtests.DummyDialector{}, &gorm.Config{})
	if err != nil {
		t.Fatalf("failed to open dummy gorm db: %v", err)
	}
	dryRunDB := db.Session(&gorm.Session{DryRun: true})
	vs := &VoucherService{}

	err = vs.ReserveVoucherUsage(context.Background(), dryRunDB, uuid.New())
	if !errors.Is(err, ErrVoucherUsageLimitExceeded) {
		t.Errorf("expected ErrVoucherUsageLimitExceeded on DryRun (RowsAffected always 0), got %v", err)
	}
}

// TestReleaseVoucherUsage_QueryShape (M3-06, review vòng 4) — pin đúng hình dạng câu UPDATE +
// điều kiện "used_count > 0" (chặn giảm xuống âm khi bị gọi trùng lặp).
func TestReleaseVoucherUsage_QueryShape(t *testing.T) {
	db, err := gorm.Open(gormtests.DummyDialector{}, &gorm.Config{})
	if err != nil {
		t.Fatalf("failed to open dummy gorm db: %v", err)
	}
	dryRunDB := db.Session(&gorm.Session{DryRun: true})
	vs := &VoucherService{}

	dryRun := vs.buildReleaseVoucherUsageQuery(context.Background(), dryRunDB, uuid.New())
	sql := dryRun.Statement.SQL.String()

	if !strings.Contains(sql, "UPDATE") {
		t.Fatalf("expected UPDATE statement, got: %s", sql)
	}
	if !strings.Contains(sql, "used_count > 0") {
		t.Errorf("expected 'used_count > 0' guard against going negative, got SQL: %s", sql)
	}
	if !strings.Contains(sql, "used_count - 1") {
		t.Errorf("expected atomic decrement expression, got SQL: %s", sql)
	}
}

// TestReleaseVoucherUsage_DelegatesToQueryBuilder (M3-06 vòng 4, viết lại theo I-03 review vòng
// 5) — bản vòng 4 CHỈ assert `err == nil` trên DryRun DB, mà DryRun không thực thi gì cả nên
// `err == nil` đúng với BẤT KỲ implementation nào — kể cả `func ReleaseVoucherUsage(...) error {
// return nil }` rỗng hoàn toàn. Reviewer vòng 5 (I-03) CHỨNG MINH bằng mutation test thật: gutting
// thân hàm thành `return nil` vẫn để CẢ 4 test trong file này (thời điểm đó) xanh — đúng lỗi
// TestRestoreAndReactivate_DelegatesToQueryBuilder (H3-02, vòng 4) đã sửa cho enrollment_repository
// nhưng bản ReleaseVoucherUsage này KHÔNG được áp dụng cùng kỹ thuật.
//
// Sửa: gọi ĐÚNG ReleaseVoucherUsage thật và bắt câu SQL nó SINH RA bằng GORM callback —
// ReleaseVoucherUsage chỉ trả về `.Error` (không trả `*gorm.DB`), nên không có cách nào đọc
// Statement.SQL từ giá trị trả về; đăng ký callback chạy SAU "gorm:update" (DryRun chỉ bỏ qua
// bước Exec thật, KHÔNG bỏ qua việc build Statement.SQL hay các callback đăng ký sau đó) để
// "chụp" lại Statement.SQL ngay khi ReleaseVoucherUsage thực thi — callback không bao giờ fire
// nếu thân hàm bị gut thành `return nil`.
//
// Tự kiểm chứng: sửa tạm ReleaseVoucherUsage thành `return nil`, chạy lại — test này FAIL đúng
// như kỳ vọng (capturedSQL rỗng); revert lại — PASS. Không giữ mutation trong working tree.
func TestReleaseVoucherUsage_DelegatesToQueryBuilder(t *testing.T) {
	db, err := gorm.Open(gormtests.DummyDialector{}, &gorm.Config{})
	if err != nil {
		t.Fatalf("failed to open dummy gorm db: %v", err)
	}
	dryRunDB := db.Session(&gorm.Session{DryRun: true})

	var capturedSQL string
	if err := dryRunDB.Callback().Update().After("gorm:update").
		Register("test:capture_sql", func(tx *gorm.DB) {
			capturedSQL = tx.Statement.SQL.String()
		}); err != nil {
		t.Fatalf("failed to register capture callback: %v", err)
	}

	vs := &VoucherService{}
	voucherID := uuid.New()

	if err := vs.ReleaseVoucherUsage(context.Background(), dryRunDB, voucherID); err != nil {
		t.Fatalf("ReleaseVoucherUsage() on DryRun DB unexpected error: %v", err)
	}

	if !strings.Contains(capturedSQL, "UPDATE") || !strings.Contains(capturedSQL, "vouchers") {
		t.Fatalf("expected ReleaseVoucherUsage to actually run an UPDATE ... vouchers ... statement, captured SQL: %q", capturedSQL)
	}
	if !strings.Contains(capturedSQL, "used_count - 1") {
		t.Errorf("expected atomic decrement expression (used_count - 1), captured SQL: %q", capturedSQL)
	}
	if !strings.Contains(capturedSQL, "used_count > 0") {
		t.Errorf("expected 'used_count > 0' guard against going negative, captured SQL: %q", capturedSQL)
	}
}
