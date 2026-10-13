package database

// Backfill vouchers.first_order_only cho voucher seed (Postgres thật, schema tạm). Mutation đã kiểm: bỏ
// `first_order_only = false`/đổi hằng mã (chạm voucher khác), bỏ runDataMigrationOnce (bật lại cờ admin vừa tắt mỗi
// lần khởi động), hoặc bỏ lời gọi trong RunPostMigrations làm test ĐỎ.

import (
	"testing"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"study.com/v1/internal/model"
	"study.com/v1/internal/testutil/pgtest"
)

func firstOrderFlag(t *testing.T, db *gorm.DB, id uuid.UUID) bool {
	t.Helper()
	var v bool
	if err := db.Raw("SELECT first_order_only FROM vouchers WHERE id = ?", id).Scan(&v).Error; err != nil {
		t.Fatalf("đọc cờ: %v", err)
	}
	return v
}

func newVoucherRow(t *testing.T, db *gorm.DB, code string, firstOnly bool) uuid.UUID {
	t.Helper()
	v := model.Voucher{Code: code, Name: "QA " + code, DiscountUnit: model.DiscountUnitMoney, DiscountMethod: model.DiscountMethodFixed}
	if err := db.Create(&v).Error; err != nil {
		t.Fatalf("tạo voucher %s: %v", code, err)
	}
	if err := db.Exec("UPDATE vouchers SET first_order_only = ? WHERE id = ?", firstOnly, v.ID).Error; err != nil {
		t.Fatalf("đặt cờ: %v", err)
	}
	return v.ID
}

func TestFirstOrderVoucherBackfill_FlagsSeededVoucherOnly_RunsOnce(t *testing.T) {
	db := pgtest.IsolatedSchema(t, Migrate)
	seeded := newVoucherRow(t, db, model.SeededFirstOrderVoucherCode, false)
	other := newVoucherRow(t, db, "QA-OTHER-"+uuid.NewString()[:6], false)
	adminFlagged := newVoucherRow(t, db, "QA-ADMIN-"+uuid.NewString()[:6], true)

	// Migrate đã ghi dấu trên DB trống: xoá dấu để mô phỏng DB cũ khởi động lần đầu với bản mới.
	if err := db.Exec("DELETE FROM data_migrations WHERE name = ?", firstOrderVoucherBackfillName).Error; err != nil {
		t.Fatalf("xoá dấu backfill: %v", err)
	}
	if err := runFirstOrderVoucherBackfill(db); err != nil {
		t.Fatalf("backfill lần 1: %v", err)
	}
	if !firstOrderFlag(t, db, seeded) {
		t.Error("voucher seed chưa được bật first_order_only")
	}
	if firstOrderFlag(t, db, other) {
		t.Error("voucher khác bị bật first_order_only")
	}
	if !firstOrderFlag(t, db, adminFlagged) {
		t.Error("voucher admin đã bật cờ bị tắt mất")
	}

	// Câu SQL idempotent (lần hai sửa 0 dòng) và bước chỉ chạy một lần: admin tắt cờ sau đó không bị bật lại.
	if n, err := backfillFirstOrderVoucher(db); err != nil || n != 0 {
		t.Fatalf("câu SQL lần 2 sửa %d dòng (err=%v), muốn 0", n, err)
	}
	if err := db.Exec("UPDATE vouchers SET first_order_only = false WHERE id = ?", seeded).Error; err != nil {
		t.Fatal(err)
	}
	if err := runFirstOrderVoucherBackfill(db); err != nil {
		t.Fatalf("backfill lần 3: %v", err)
	}
	if firstOrderFlag(t, db, seeded) {
		t.Error("bước đã ghi dấu không được chạy lại: cờ admin vừa tắt bị bật lại")
	}
}

// Wired, not just present: RunPostMigrations (đường API khởi động chạy) phải gọi bước này.
func TestRunPostMigrations_RunsFirstOrderVoucherBackfillOnOldDB(t *testing.T) {
	db := pgtest.IsolatedSchema(t, Migrate)
	seeded := newVoucherRow(t, db, model.SeededFirstOrderVoucherCode, false)
	if err := db.Exec("DELETE FROM data_migrations WHERE name = ?", firstOrderVoucherBackfillName).Error; err != nil {
		t.Fatalf("xoá dấu backfill: %v", err)
	}
	if err := RunPostMigrations(db); err != nil {
		t.Fatalf("RunPostMigrations: %v", err)
	}
	if !firstOrderFlag(t, db, seeded) {
		t.Error("RunPostMigrations phải bật first_order_only cho voucher seed trên DB cũ")
	}
}
