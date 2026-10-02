package database

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	"gorm.io/gorm"
	"study.com/v1/internal/testutil/pgtest"
)

// testMigrateLockTimeout — hạn chờ khoá đủ rộng cho các test mà khoá chỉ bị tranh trong thời gian ngắn.
const testMigrateLockTimeout = 5 * time.Minute

// L9 mục 3: tiến trình giữ khoá bị treo thì tiến trình đến sau phải DỪNG với lỗi rõ ràng sau hạn chờ, không chờ vô
// hạn và không chạy migrate. Sau khi khoá được nhả thì khởi động lại chạy bình thường (không kẹt trạng thái nào).
func TestMigrateAtStartup_LockWaitIsBoundedAndSkipsMigrate(t *testing.T) {
	base := pgtest.IsolatedSchema(t, func(*gorm.DB) error { return nil }) // schema trống, chưa migrate

	// "Tiến trình treo": một pool khác giữ khoá bằng một kết nối riêng và không nhả.
	holderDB := pgtest.OpenSameSchema(t, base)
	sqlDB, err := holderDB.DB()
	if err != nil {
		t.Fatal(err)
	}
	holder, err := sqlDB.Conn(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer holder.Close()
	var got bool
	if err := holder.QueryRowContext(context.Background(), "SELECT pg_try_advisory_lock($1)", migrateStartupLockKey).Scan(&got); err != nil || !got {
		t.Fatalf("không giữ được khoá để dựng tình huống treo: got=%v err=%v", got, err)
	}

	waiter := pgtest.OpenSameSchema(t, base)
	const wait = 700 * time.Millisecond
	type outcome struct {
		err     error
		elapsed time.Duration
	}
	done := make(chan outcome, 1)
	go func() {
		start := time.Now()
		err := MigrateAtStartup(waiter, wait)
		done <- outcome{err, time.Since(start)}
	}()
	var res outcome
	select {
	case res = <-done:
	case <-time.After(30 * time.Second): // chờ vô hạn là đúng lỗi cần bắt: test phải đỏ chứ không treo
		t.Fatal("MigrateAtStartup vẫn chờ khoá sau 30s dù hạn chờ chỉ 700ms (chờ vô hạn)")
	}
	if res.err == nil {
		t.Fatal("khoá đang bị giữ mà MigrateAtStartup trả nil")
	}
	if !strings.Contains(res.err.Error(), "chờ khoá migration quá") || !strings.Contains(res.err.Error(), "MIGRATE_LOCK_TIMEOUT_MINUTES") {
		t.Fatalf("lỗi không nêu rõ nguyên nhân và cách xử lý: %v", res.err)
	}
	if res.elapsed < wait || res.elapsed > 10*time.Second {
		t.Fatalf("chờ %s, muốn khoảng %s (không sớm hơn hạn, không kéo dài)", res.elapsed, wait)
	}
	if base.Migrator().HasTable("classes") {
		t.Fatal("hết hạn chờ mà vẫn chạy migrate (đã có bảng classes)")
	}

	// Nhả khoá rồi khởi động lại: chạy bình thường.
	if _, err := holder.ExecContext(context.Background(), "SELECT pg_advisory_unlock($1)", migrateStartupLockKey); err != nil {
		t.Fatal(err)
	}
	if err := MigrateAtStartup(waiter, testMigrateLockTimeout); err != nil {
		t.Fatalf("khởi động lại sau khi khoá được nhả: %v", err)
	}
	if !base.Migrator().HasTable("classes") {
		t.Fatal("sau khi khoá được nhả mà migrate không chạy")
	}
}

// Hạn chờ không dương bị từ chối (0 không được hiểu là "chờ mãi").
func TestMigrateAtStartup_RejectsNonPositiveTimeout(t *testing.T) {
	db := pgtest.IsolatedSchema(t, func(*gorm.DB) error { return nil })
	for _, d := range []time.Duration{0, -time.Second} {
		if err := MigrateAtStartup(db, d); err == nil {
			t.Fatalf("timeout %s phải bị từ chối", d)
		}
	}
}

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
			errs <- MigrateAtStartup(db, testMigrateLockTimeout)
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
	if err := MigrateAtStartup(base, testMigrateLockTimeout); err != nil {
		t.Fatalf("chạy lại bước migrate khởi động: %v", err)
	}
}

// Khoá phải được nhả: sau khi MigrateAtStartup xong, kết nối khác lấy được ngay khoá cùng key (không bị giữ
// treo trên kết nối đã trả về pool).
func TestMigrateAtStartup_ReleasesLock(t *testing.T) {
	db := pgtest.IsolatedSchema(t, func(*gorm.DB) error { return nil })
	if err := MigrateAtStartup(db, testMigrateLockTimeout); err != nil {
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
