package repository

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
	gormtests "gorm.io/gorm/utils/tests"
)

// TestUpdatePaymentInfo_ConditionalGuard (M-02, review vòng 5) — pin đúng câu SQL chống race
// của UpdatePaymentInfo: WHERE phải kèm "status IN ('pending','processing')" thay vì UPDATE vô
// điều kiện như trước — gọi ĐÚNG buildUpdatePaymentInfoQuery (hàm sản xuất thật) trên DryRun DB,
// không hand-roll lại query trong test.
func TestUpdatePaymentInfo_ConditionalGuard(t *testing.T) {
	db, err := gorm.Open(gormtests.DummyDialector{}, &gorm.Config{})
	if err != nil {
		t.Fatalf("failed to open dummy gorm db: %v", err)
	}
	repo := &OrderRepository{db: db.Session(&gorm.Session{DryRun: true})}

	dryRun := repo.buildUpdatePaymentInfoQuery(uuid.New(), "bank_transfer", "mbbank", "FT123", time.Now())
	sql := dryRun.Statement.SQL.String()

	if !strings.Contains(sql, "UPDATE") {
		t.Fatalf("expected UPDATE statement, got: %s", sql)
	}
	if !strings.Contains(sql, "status IN") && !strings.Contains(sql, "status in") {
		t.Errorf("expected conditional guard 'status IN (pending,processing)' in WHERE clause (chống race M-02), got SQL: %s", sql)
	}
	if !strings.Contains(sql, "pending") || !strings.Contains(sql, "processing") {
		t.Errorf("expected both 'pending' and 'processing' allowed in guard, got SQL: %s", sql)
	}
}

// TestUpdatePaymentInfo_ReturnsErrOrderConflictOnZeroRows (M-02, review vòng 5) — gọi ĐÚNG
// UpdatePaymentInfo thật trên DryRun DB: DryRun không thực thi câu UPDATE thật nên RowsAffected
// LUÔN LUÔN = 0 — nhánh "đơn đã đổi trạng thái bởi luồng khác" (ErrOrderConflict) CHẮC CHẮN được
// thực thi một cách xác định, không cần Postgres thật. Xóa nhánh RowsAffected==0 trong
// UpdatePaymentInfo sẽ làm test này đỏ.
func TestUpdatePaymentInfo_ReturnsErrOrderConflictOnZeroRows(t *testing.T) {
	db, err := gorm.Open(gormtests.DummyDialector{}, &gorm.Config{})
	if err != nil {
		t.Fatalf("failed to open dummy gorm db: %v", err)
	}
	repo := &OrderRepository{db: db.Session(&gorm.Session{DryRun: true})}

	err = repo.UpdatePaymentInfo(uuid.New(), "bank_transfer", "mbbank", "FT123", time.Now())
	if !errors.Is(err, ErrOrderConflict) {
		t.Errorf("expected ErrOrderConflict on DryRun (RowsAffected always 0), got %v", err)
	}
}

// TestUpdatePaymentCode_ConditionalGuard (B-03, review vòng 5) — pin đúng câu SQL chống race của
// UpdatePaymentCode: WHERE phải kèm điều kiện "status='pending' HOẶC (status='processing' VÀ
// chưa có payment_transaction_id)" thay vì UPDATE vô điều kiện như trước — gọi ĐÚNG
// buildUpdatePaymentCodeQuery (hàm sản xuất thật) trên DryRun DB.
func TestUpdatePaymentCode_ConditionalGuard(t *testing.T) {
	db, err := gorm.Open(gormtests.DummyDialector{}, &gorm.Config{})
	if err != nil {
		t.Fatalf("failed to open dummy gorm db: %v", err)
	}
	repo := &OrderRepository{db: db.Session(&gorm.Session{DryRun: true})}

	dryRun := repo.buildUpdatePaymentCodeQuery(uuid.New(), "PAY123", time.Now().Add(24*time.Hour))
	sql := dryRun.Statement.SQL.String()

	if !strings.Contains(sql, "UPDATE") {
		t.Fatalf("expected UPDATE statement, got: %s", sql)
	}
	if !strings.Contains(sql, "status = 'pending'") {
		t.Errorf("expected 'status = pending' branch in guard, got SQL: %s", sql)
	}
	if !strings.Contains(sql, "status = 'processing'") {
		t.Errorf("expected 'status = processing' branch in guard (chưa có code), got SQL: %s", sql)
	}
	if !strings.Contains(sql, "payment_transaction_id IS NULL") {
		t.Errorf("expected 'payment_transaction_id IS NULL' guard (đơn processing nhưng chưa có code), got SQL: %s", sql)
	}
}

// TestUpdatePaymentCode_ReturnsErrOrderConflictOnZeroRows (B-03, review vòng 5) — gọi ĐÚNG
// UpdatePaymentCode thật trên DryRun DB: DryRun không thực thi câu UPDATE thật nên RowsAffected
// LUÔN LUÔN = 0 — nhánh "race giữa 2 request tạo intent đồng thời" (ErrOrderConflict) CHẮC CHẮN
// được thực thi một cách xác định. Xóa nhánh RowsAffected==0 trong UpdatePaymentCode sẽ làm test
// này đỏ.
func TestUpdatePaymentCode_ReturnsErrOrderConflictOnZeroRows(t *testing.T) {
	db, err := gorm.Open(gormtests.DummyDialector{}, &gorm.Config{})
	if err != nil {
		t.Fatalf("failed to open dummy gorm db: %v", err)
	}
	repo := &OrderRepository{db: db.Session(&gorm.Session{DryRun: true})}

	err = repo.UpdatePaymentCode(uuid.New(), "PAY123", time.Now().Add(24*time.Hour))
	if !errors.Is(err, ErrOrderConflict) {
		t.Errorf("expected ErrOrderConflict on DryRun (RowsAffected always 0), got %v", err)
	}
}
