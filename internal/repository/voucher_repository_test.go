package repository

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/google/uuid"
	"gorm.io/gorm"
	gormtests "gorm.io/gorm/utils/tests"
)

// TestIncrementUsedCount_ConditionalGuard (item 24/M-07, review web + review vòng 1) — pin lại
// đúng câu SQL chống race của IncrementUsedCount, cùng mẫu với
// TestIncrementUsageCount_ConditionalGuard (coupon_repository_test.go): UPDATE phải kèm điều
// kiện "usage_limit <= 0 OR used_count < usage_limit" trong WHERE để DB tự chối tăng used_count
// khi voucher đã hết lượt, thay vì UPDATE vô điều kiện. Áp dụng cho vouchers vì đây mới là
// bảng web thực sự dùng (xem comment ErrVoucherUsageExceeded/CouponID trong code).
//
// M2-05 (review vòng 3): TRƯỚC ĐÂY test này hand-roll lại câu query bằng tay thay vì gọi
// IncrementUsedCount thật — xóa hẳn IncrementUsedCount vẫn không làm test này đỏ. Sửa bằng cách
// gọi buildIncrementUsedCountQuery — hàm PRODUCTION thật mà IncrementUsedCount ủy quyền tới (xem
// voucher_repository.go) — trên DryRun DB rồi đọc Statement.SQL từ kết quả trả về.
func TestIncrementUsedCount_ConditionalGuard(t *testing.T) {
	db, err := gorm.Open(gormtests.DummyDialector{}, &gorm.Config{})
	if err != nil {
		t.Fatalf("failed to open dummy gorm db: %v", err)
	}
	repo := &VoucherRepository{db: db.Session(&gorm.Session{DryRun: true})}

	voucherID := uuid.New()
	dryRun := repo.buildIncrementUsedCountQuery(context.Background(), voucherID)

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

// TestIncrementUsedCount_ReturnsErrVoucherUsageExceededOnZeroRows (M2-05, review vòng 3) — pin
// hợp đồng RowsAffected==0 -> ErrVoucherUsageExceeded bằng cách gọi ĐÚNG hàm sản xuất thật
// IncrementUsedCount (không phải một hàm map RowsAffected viết lại trong test, như trước đây —
// "green that proves nothing": xóa nhánh if trong IncrementUsedCount không làm test cũ đỏ).
//
// Chạy trên DryRun DB: DryRun không thực thi câu UPDATE thật nên RowsAffected LUÔN LUÔN bằng 0
// (không có kết nối DB thật để trả về số dòng bị ảnh hưởng) — nhánh "hết lượt" của
// IncrementUsedCount CHẮC CHẮN được thực thi một cách xác định (deterministic), không cần
// Postgres thật. Nếu ai xóa nhánh "if result.RowsAffected == 0 { return ErrVoucherUsageExceeded }"
// trong IncrementUsedCount, test này sẽ đỏ (trả về nil thay vì ErrVoucherUsageExceeded).
func TestIncrementUsedCount_ReturnsErrVoucherUsageExceededOnZeroRows(t *testing.T) {
	db, err := gorm.Open(gormtests.DummyDialector{}, &gorm.Config{})
	if err != nil {
		t.Fatalf("failed to open dummy gorm db: %v", err)
	}
	repo := &VoucherRepository{db: db.Session(&gorm.Session{DryRun: true})}

	err = repo.IncrementUsedCount(context.Background(), uuid.New())
	if !errors.Is(err, ErrVoucherUsageExceeded) {
		t.Errorf("expected ErrVoucherUsageExceeded on DryRun (RowsAffected always 0), got %v", err)
	}
}
