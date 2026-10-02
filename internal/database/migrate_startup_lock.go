package database

// migrate_startup_lock.go — L8 mục 6. Hai tiến trình khởi động cùng lúc trên một DB trống (deploy cuốn chiếu,
// API cùng cmd/seed, nhiều instance) cùng chạy AutoMigrate và đua nhau: cả hai thấy cột/bảng chưa có rồi cùng
// ADD, một bên chết với `column "organization_id" of relation "classes" already exists`. Khoá advisory CẤP PHIÊN
// bao toàn bộ bước migrate, nên tiến trình đến sau chờ rồi thấy mọi thứ đã có và không làm gì (các bước đều
// idempotent).

import (
	"context"
	"database/sql/driver"
	"errors"
	"fmt"
	"log"

	"gorm.io/gorm"
)

// migrateStartupLockKey — khoá advisory (toàn DB) của bước migrate lúc khởi động. Hằng cố định, không suy từ tên
// schema: mọi tiến trình khởi động cùng DB phải tranh CÙNG một khoá. PHẢI khác timestampConversionLockKey: khoá đó
// được lấy (kiểu giao dịch, từ kết nối khác) BÊN TRONG Migrate, trùng khoá thì Migrate tự chờ chính mình mãi mãi.
const migrateStartupLockKey int64 = 4020261003

// MigrateAtStartup chạy Migrate dưới khoá advisory cấp phiên; dùng cho mọi đường khởi động thật (API, cmd/seed).
//
// Test từng schema tạm riêng dùng thẳng Migrate: khoá này toàn DB nên sẽ xếp hàng các gói test chạy song song
// trong khi mỗi schema đã tự cô lập.
func MigrateAtStartup(db *gorm.DB) error {
	return withSessionAdvisoryLock(db, migrateStartupLockKey, func() error { return Migrate(db) })
}

// withSessionAdvisoryLock giữ pg_advisory_lock(key) trên MỘT kết nối riêng lấy từ pool trong lúc chạy fn (fn dùng
// các kết nối khác của pool, nên pool cần tối thiểu 2 kết nối). Luôn nhả khoá; nhả lỗi thì huỷ hẳn kết nối, vì
// khoá cấp phiên chỉ mất khi nhả hoặc khi kết nối thật sự đóng, còn trả kết nối về pool vẫn giữ khoá.
func withSessionAdvisoryLock(db *gorm.DB, key int64, fn func() error) error {
	sqlDB, err := db.DB()
	if err != nil {
		return fmt.Errorf("lấy pool kết nối để khoá migration: %w", err)
	}
	if sqlDB.Stats().MaxOpenConnections == 1 {
		return errors.New("khoá migration cần pool tối thiểu 2 kết nối (một giữ khoá, một chạy migration), pool đang giới hạn 1")
	}
	ctx := context.Background()
	conn, err := sqlDB.Conn(ctx)
	if err != nil {
		return fmt.Errorf("lấy kết nối giữ khoá migration: %w", err)
	}
	defer conn.Close()

	var got bool
	if err := conn.QueryRowContext(ctx, "SELECT pg_try_advisory_lock($1)", key).Scan(&got); err != nil {
		return fmt.Errorf("thử lấy khoá migration: %w", err)
	}
	if !got {
		log.Printf("[MIGRATE] tiến trình khác đang migrate, chờ khoá khởi động (key=%d)", key)
		if _, err := conn.ExecContext(ctx, "SELECT pg_advisory_lock($1)", key); err != nil {
			return fmt.Errorf("chờ khoá migration: %w", err)
		}
	}
	defer func() {
		if _, uerr := conn.ExecContext(context.Background(), "SELECT pg_advisory_unlock($1)", key); uerr != nil {
			log.Printf("[MIGRATE] không nhả được khoá migration %d, huỷ kết nối giữ khoá: %v", key, uerr)
			_ = conn.Raw(func(any) error { return driver.ErrBadConn })
		}
	}()
	return fn()
}
