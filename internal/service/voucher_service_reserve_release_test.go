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

// TestReleaseVoucherUsage_DelegatesToQueryBuilder (M3-06, review vòng 4) — ReleaseVoucherUsage
// chỉ trả .Error của buildReleaseVoucherUsageQuery, gọi thật trên DryRun DB (không lỗi cho một
// Statement hợp lệ).
func TestReleaseVoucherUsage_DelegatesToQueryBuilder(t *testing.T) {
	db, err := gorm.Open(gormtests.DummyDialector{}, &gorm.Config{})
	if err != nil {
		t.Fatalf("failed to open dummy gorm db: %v", err)
	}
	dryRunDB := db.Session(&gorm.Session{DryRun: true})
	vs := &VoucherService{}

	if err := vs.ReleaseVoucherUsage(context.Background(), dryRunDB, uuid.New()); err != nil {
		t.Errorf("ReleaseVoucherUsage() on DryRun DB should not error, got %v", err)
	}
}
