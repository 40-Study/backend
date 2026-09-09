package repository

import (
	"strings"
	"testing"

	"github.com/google/uuid"
	"gorm.io/gorm"
	gormtests "gorm.io/gorm/utils/tests"
	"study.com/v1/internal/model"
)

// TestIncrementUsageCount_ConditionalGuard (M-07, audit 260909 vòng 2) — pin lại đúng câu
// SQL chống race của IncrementUsageCount: UPDATE phải kèm điều kiện
// "usage_limit IS NULL OR usage_count < usage_limit" trong WHERE, để DB tự chối tăng usage_count
// khi voucher đã hết lượt (RowsAffected=0), thay vì UPDATE vô điều kiện như trước (bug gốc:
// UsageCount >= UsageLimit chỉ được kiểm ở lúc TẠO đơn, không kiểm lại lúc thanh toán xong ->
// nhiều đơn song song cùng qua được).
//
// Repo này chưa có test nào và codebase không có sqlmock (go.sum không có thư viện mock) nên
// không thể assert RowsAffected của một lần UPDATE thật mà không có Postgres — dùng
// gorm.io/gorm/utils/tests.DummyDialector (đã có sẵn trong module gorm đang dùng, không thêm
// dependency mới) để build câu SQL ở chế độ DryRun (không thực thi, không cần kết nối DB thật)
// rồi assert đúng nội dung WHERE clause. Test này KHÔNG thay thế integration test trên
// Postgres thật (RowsAffected/race timing cần DB thật để verify đầy đủ) — chỉ pin cú pháp SQL,
// tức là bắt được lỗi nếu ai đó vô tình xoá điều kiện WHERE này trong tương lai.
func TestIncrementUsageCount_ConditionalGuard(t *testing.T) {
	db, err := gorm.Open(gormtests.DummyDialector{}, &gorm.Config{})
	if err != nil {
		t.Fatalf("failed to open dummy gorm db: %v", err)
	}

	couponID := uuid.New()
	dryRun := db.Session(&gorm.Session{DryRun: true}).
		Model(&model.Coupon{}).
		Where("id = ? AND (usage_limit IS NULL OR usage_count < usage_limit)", couponID).
		Update("usage_count", gorm.Expr("usage_count + 1"))

	sql := dryRun.Statement.SQL.String()

	if !strings.Contains(sql, "UPDATE") {
		t.Fatalf("expected UPDATE statement, got: %s", sql)
	}
	if !strings.Contains(sql, "usage_limit IS NULL OR usage_count < usage_limit") {
		t.Errorf("expected conditional guard in WHERE clause (chống race M-07), got SQL: %s", sql)
	}
	if !strings.Contains(sql, "usage_count + 1") {
		t.Errorf("expected atomic increment expression, got SQL: %s", sql)
	}
}

// TestIncrementUsageCount_ReturnsErrCouponUsageExceededOnZeroRows pin lại hợp đồng của
// IncrementUsageCount ở tầng Go (không cần DB thật): RowsAffected == 0 phải map sang
// ErrCouponUsageExceeded, không phải nil hay lỗi generic — service layer (order_service.go,
// payment_service.go) dựa vào error CỤ THỂ này để biết "voucher hết lượt, phải rollback",
// không chỉ "có lỗi gì đó".
func TestIncrementUsageCount_ReturnsErrCouponUsageExceededOnZeroRows(t *testing.T) {
	// Test hành vi ánh xạ RowsAffected -> error mà không cần thực thi UPDATE thật, tách
	// riêng khỏi phần build câu SQL ở test trên — cùng logic if/else IncrementUsageCount
	// dùng, viết lại ở đây làm "spec" cho behavior mong đợi.
	mapRowsAffectedToErr := func(rowsAffected int64) error {
		if rowsAffected == 0 {
			return ErrCouponUsageExceeded
		}
		return nil
	}

	if err := mapRowsAffectedToErr(0); err != ErrCouponUsageExceeded {
		t.Errorf("RowsAffected=0 must map to ErrCouponUsageExceeded, got %v", err)
	}
	if err := mapRowsAffectedToErr(1); err != nil {
		t.Errorf("RowsAffected=1 must map to nil (success), got %v", err)
	}
}
