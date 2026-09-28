// Package pgtest mở kết nối Postgres THẬT cho test tích hợp.
//
// Quy tắc (review Phase 4, B-2/B-11):
//   - Không kết nối được DB: bỏ qua (Skip) khi chạy local, nhưng FAIL khi CI=true — CI có service
//     Postgres, nên "không kết nối được" ở đó là lỗi cấu hình, không được lặng lẽ thành xanh.
//   - Test cần COMMIT dữ liệu (vd test race nhiều kết nối) dùng IsolatedSchema: 1 schema tạm riêng,
//     migrate đầy đủ, DROP khi test xong — không để lại dữ liệu hay constraint trên DB dev dùng chung.
//
// Package này không import internal/database để internal/database cũng dùng được (tránh import vòng);
// hàm migrate do nơi gọi truyền vào.
package pgtest

import (
	"bufio"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// MissingDBAction quyết định làm gì khi không kết nối được Postgres (hàm thuần để unit test).
func MissingDBAction(ciEnv string) string {
	if strings.EqualFold(strings.TrimSpace(ciEnv), "true") {
		return "fail"
	}
	return "skip"
}

func loadDotEnv() {
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		return
	}
	// internal/testutil/pgtest -> repo root
	envPath := filepath.Join(filepath.Dir(thisFile), "..", "..", "..", ".env")
	f, err := os.Open(envPath)
	if err != nil {
		return
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			continue
		}
		key := strings.TrimSpace(parts[0])
		if _, exists := os.LookupEnv(key); !exists {
			_ = os.Setenv(key, strings.Trim(strings.TrimSpace(parts[1]), `"'`))
		}
	}
}

func env(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func dsn(extra string) string {
	return fmt.Sprintf("host=%s user=%s password=%s dbname=%s port=%s sslmode=disable TimeZone=Asia/Ho_Chi_Minh%s",
		env("DB_HOST", "localhost"), env("DB_USER", "study_user"), os.Getenv("DB_PASSWORD"),
		env("DB_NAME", "study_db"), env("DB_PORT", "5432"), extra)
}

func open(t *testing.T, extra string) *gorm.DB {
	t.Helper()
	loadDotEnv()
	db, err := gorm.Open(postgres.Open(dsn(extra)), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err == nil {
		err = db.Exec("SELECT 1").Error
	}
	if err != nil {
		msg := fmt.Sprintf("không kết nối được Postgres (%s:%s/%s): %v",
			env("DB_HOST", "localhost"), env("DB_PORT", "5432"), env("DB_NAME", "study_db"), err)
		if MissingDBAction(os.Getenv("CI")) == "fail" {
			t.Fatalf("CI=true nhưng %s", msg)
		}
		t.Skip(msg)
	}
	return db
}

// Open trả kết nối tới DB cấu hình (DB_*). Chỉ dùng cho test tự ROLLBACK hoặc chỉ dùng bảng TEMP.
func Open(t *testing.T) *gorm.DB {
	t.Helper()
	db := open(t, "")
	t.Cleanup(func() {
		if sqlDB, err := db.DB(); err == nil {
			_ = sqlDB.Close()
		}
	})
	return db
}

// IsolatedSchema tạo schema tạm `t_<random>`, mở kết nối có search_path trỏ vào nó, chạy migrate,
// và DROP SCHEMA ... CASCADE khi test kết thúc.
func IsolatedSchema(t *testing.T, migrate func(*gorm.DB) error) *gorm.DB {
	t.Helper()
	admin := open(t, "")
	buf := make([]byte, 6)
	_, _ = rand.Read(buf)
	schema := "t_" + hex.EncodeToString(buf)
	if err := admin.Exec("CREATE SCHEMA " + schema).Error; err != nil {
		t.Fatalf("tạo schema tạm %s: %v", schema, err)
	}
	db := open(t, " search_path="+schema)
	t.Cleanup(func() {
		if sqlDB, err := db.DB(); err == nil {
			_ = sqlDB.Close()
		}
		if err := admin.Exec("DROP SCHEMA IF EXISTS " + schema + " CASCADE").Error; err != nil {
			t.Errorf("xoá schema tạm %s: %v", schema, err)
		}
		if sqlDB, err := admin.DB(); err == nil {
			_ = sqlDB.Close()
		}
	})
	if migrate != nil {
		if err := migrate(db); err != nil {
			t.Fatalf("migrate schema tạm %s: %v", schema, err)
		}
	}
	return db
}
