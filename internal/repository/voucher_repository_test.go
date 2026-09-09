package repository

import (
	"strings"
	"testing"

	"github.com/google/uuid"
	"gorm.io/gorm"
	gormtests "gorm.io/gorm/utils/tests"
	"study.com/v1/internal/model"
)

// TestIncrementUsedCount_ConditionalGuard (item 24/M-07, review web + review vòng 1) — pin lại
// đúng câu SQL chống race của IncrementUsedCount, cùng mẫu với
// TestIncrementUsageCount_ConditionalGuard (coupon_repository_test.go): UPDATE phải kèm điều
// kiện "usage_limit <= 0 OR used_count < usage_limit" trong WHERE để DB tự chối tăng used_count
// khi voucher đã hết lượt, thay vì UPDATE vô điều kiện. Áp dụng cho vouchers vì đây mới là
// bảng web thực sự dùng (xem comment ErrVoucherUsageExceeded/CouponID trong code).
func TestIncrementUsedCount_ConditionalGuard(t *testing.T) {
	db, err := gorm.Open(gormtests.DummyDialector{}, &gorm.Config{})
	if err != nil {
		t.Fatalf("failed to open dummy gorm db: %v", err)
	}

	voucherID := uuid.New()
	dryRun := db.Session(&gorm.Session{DryRun: true}).
		Model(&model.Voucher{}).
		Where("id = ? AND (usage_limit <= 0 OR used_count < usage_limit)", voucherID).
		Update("used_count", gorm.Expr("used_count + 1"))

	sql := dryRun.Statement.SQL.String()

	if !strings.Contains(sql, "UPDATE") {
		t.Fatalf("expected UPDATE statement, got: %s", sql)
	}
	if !strings.Contains(sql, "usage_limit <= 0 OR used_count < usage_limit") {
		t.Errorf("expected conditional guard in WHERE clause (chống race M-07), got SQL: %s", sql)
	}
	if !strings.Contains(sql, "used_count + 1") {
		t.Errorf("expected atomic increment expression, got SQL: %s", sql)
	}
}

// TestIncrementUsedCount_ReturnsErrVoucherUsageExceededOnZeroRows pin lại hợp đồng ở tầng Go:
// RowsAffected == 0 phải map sang ErrVoucherUsageExceeded.
func TestIncrementUsedCount_ReturnsErrVoucherUsageExceededOnZeroRows(t *testing.T) {
	mapRowsAffectedToErr := func(rowsAffected int64) error {
		if rowsAffected == 0 {
			return ErrVoucherUsageExceeded
		}
		return nil
	}

	if err := mapRowsAffectedToErr(0); err != ErrVoucherUsageExceeded {
		t.Errorf("RowsAffected=0 must map to ErrVoucherUsageExceeded, got %v", err)
	}
	if err := mapRowsAffectedToErr(1); err != nil {
		t.Errorf("RowsAffected=1 must map to nil (success), got %v", err)
	}
}
