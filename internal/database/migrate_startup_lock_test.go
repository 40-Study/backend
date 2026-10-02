package database

import (
	"sync"
	"testing"

	"gorm.io/gorm"
	"study.com/v1/internal/testutil/pgtest"
)

// L8 mục 6: nhiều tiến trình khởi động cùng lúc trên một DB trống (lỗi thật gặp khi review L7:
// `column "organization_id" of relation "classes" already exists`). Mỗi worker là một pool riêng, như một tiến
// trình khác, cùng trỏ vào một schema trống rồi cùng chạy bước migrate khởi động.
func TestMigrateAtStartup_ConcurrentProcessesOnEmptyDBDoNotRace(t *testing.T) {
	base := pgtest.IsolatedSchema(t, func(*gorm.DB) error { return nil }) // schema trống, chưa migrate
	const workers = 4
	dbs := make([]*gorm.DB, workers)
	for i := range dbs {
		dbs[i] = pgtest.OpenSameSchema(t, base)
	}

	start := make(chan struct{})
	errs := make(chan error, workers)
	var done sync.WaitGroup
	for _, db := range dbs {
		done.Add(1)
		go func(db *gorm.DB) {
			defer done.Done()
			<-start
			errs <- MigrateAtStartup(db)
		}(db)
	}
	close(start)
	done.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatalf("một tiến trình khởi động lỗi khi đua migrate trên DB trống: %v", err)
		}
	}

	// Kết quả cuối đủ và chạy lại không đổi gì.
	if !base.Migrator().HasColumn("classes", "organization_id") {
		t.Fatal("sau migrate đồng thời thiếu cột classes.organization_id")
	}
	if err := MigrateAtStartup(base); err != nil {
		t.Fatalf("chạy lại bước migrate khởi động: %v", err)
	}
}

// Khoá phải được nhả: sau khi MigrateAtStartup xong, kết nối khác lấy được ngay khoá cùng key (không bị giữ
// treo trên kết nối đã trả về pool).
func TestMigrateAtStartup_ReleasesLock(t *testing.T) {
	db := pgtest.IsolatedSchema(t, func(*gorm.DB) error { return nil })
	if err := MigrateAtStartup(db); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	other := pgtest.OpenSameSchema(t, db)
	var got bool
	if err := other.Raw("SELECT pg_try_advisory_lock(?)", migrateStartupLockKey).Scan(&got).Error; err != nil {
		t.Fatalf("thử lấy khoá: %v", err)
	}
	if !got {
		t.Fatal("khoá migrate khởi động vẫn bị giữ sau khi MigrateAtStartup trả về")
	}
	// Dọn: kết nối vừa lấy khoá thuộc pool của `other`, đóng cùng pool ở Cleanup nên khoá tự mất.
}

// Khoá khởi động trùng khoá giao dịch của convertTimestampColumns thì Migrate tự chờ chính mình mãi (khoá kia được
// lấy từ kết nối khác BÊN TRONG Migrate): ghim hai hằng khác nhau.
func TestMigrateStartupLockKey_DiffersFromTimestampConversionKey(t *testing.T) {
	if migrateStartupLockKey == timestampConversionLockKey {
		t.Fatalf("migrateStartupLockKey = timestampConversionLockKey = %d: Migrate sẽ tự deadlock", migrateStartupLockKey)
	}
}
