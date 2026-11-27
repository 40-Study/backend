package database

// Q9-PG-05: migration thêm users.self_deleted_at phải idempotent và tự thêm lại cột nếu thiếu (DB tạo trước khi có cột mà
// AutoMigrate không chạy). Bỏ entry trong RunPostMigrations -> bước "thêm lại sau DROP" ĐỎ.

import (
	"testing"

	"study.com/v1/internal/testutil/pgtest"
)

func TestQ9_PG_SelfDeletedAtColumn_MigrationIdempotent(t *testing.T) {
	db := pgtest.IsolatedSchema(t, Migrate)
	hasColumn := func() bool {
		var n int64
		if err := db.Raw(`SELECT count(*) FROM information_schema.columns
			WHERE table_schema = current_schema() AND table_name = 'users' AND column_name = 'self_deleted_at'`).Scan(&n).Error; err != nil {
			t.Fatal(err)
		}
		return n == 1
	}
	if !hasColumn() {
		t.Fatal("Migrate phải tạo users.self_deleted_at")
	}
	if err := db.Exec(`ALTER TABLE users DROP COLUMN self_deleted_at`).Error; err != nil {
		t.Fatal(err)
	}
	for i := 1; i <= 2; i++ {
		if err := RunPostMigrations(db); err != nil {
			t.Fatalf("RunPostMigrations lần %d: %v", i, err)
		}
		if !hasColumn() {
			t.Fatalf("lần %d: RunPostMigrations không thêm lại users.self_deleted_at", i)
		}
	}
}
