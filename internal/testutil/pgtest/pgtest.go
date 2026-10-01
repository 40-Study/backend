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
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// tb là phần của *testing.T mà pgtest dùng. Tách thành interface để test được các nhánh t.Skip /
// t.Fatalf bằng bản giả (testing.TB có method private nên không giả được).
type tb interface {
	Helper()
	Cleanup(func())
	Skip(args ...any)
	Fatalf(format string, args ...any)
	Errorf(format string, args ...any)
	Logf(format string, args ...any)
}

const (
	// tempSchemaPattern là tên schema tạm do IsolatedSchema sinh ra ("t_" + 12 hex).
	tempSchemaPattern = `^t_[0-9a-f]{12}$`
	// commentPrefix đánh dấu thời điểm tạo trong comment của schema (xem sweepOrphans).
	commentPrefix = "pgtest:created="
	// appNamePrefix gắn vào application_name của kết nối dùng schema, để lượt quét biết schema nào còn
	// phiên đang dùng (pg_stat_activity.application_name).
	appNamePrefix = "pgtest:"
	// orphanMaxAge: schema tạm già hơn ngưỡng này mà không còn phiên nào dùng thì coi là mồ côi. Dài
	// hơn nhiều so với một lần chạy test (-test.timeout mặc định 10 phút) nên không đụng lần chạy
	// song song đang sống.
	orphanMaxAge = 2 * time.Hour
	// dropLockTimeout: DROP chờ khoá tối đa chừng này rồi báo lỗi thay vì treo test.
	dropLockTimeout = "5s"
)

var (
	tempSchemaRE = regexp.MustCompile(tempSchemaPattern)
	createdRE    = regexp.MustCompile(`^` + regexp.QuoteMeta(commentPrefix) + `([0-9]+)$`)
	sweepOnce    sync.Once
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

func open(t tb, extra string) *gorm.DB {
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

func closeDB(db *gorm.DB) {
	if sqlDB, err := db.DB(); err == nil {
		_ = sqlDB.Close()
	}
}

// Open trả kết nối tới DB cấu hình (DB_*). Chỉ dùng cho test tự ROLLBACK hoặc chỉ dùng bảng TEMP.
func Open(t *testing.T) *gorm.DB {
	t.Helper()
	db := open(t, "")
	t.Cleanup(func() { closeDB(db) })
	return db
}

// IsolatedSchema tạo schema tạm `t_<random>`, mở kết nối có search_path trỏ vào nó, chạy migrate,
// và DROP SCHEMA ... CASCADE khi test kết thúc.
//
// Không rò schema: Cleanup drop được đăng ký NGAY sau khi CREATE SCHEMA thành công, trước mọi bước có
// thể t.Skip/t.Fatalf (mở kết nối thứ hai, migrate); kết nối admin được đóng ở mọi nhánh. Nếu tiến
// trình test bị giết (timeout, taskkill) thì Cleanup không chạy được — schema mồ côi đó do
// sweepOrphans dọn ở lần chạy sau.
func IsolatedSchema(t *testing.T, migrate func(*gorm.DB) error) *gorm.DB {
	t.Helper()
	return isolatedSchema(t, open, migrate)
}

// isolatedSchema là thân của IsolatedSchema, nhận hàm mở kết nối để test chèn lỗi/skip.
func isolatedSchema(t tb, openDB func(tb, string) *gorm.DB, migrate func(*gorm.DB) error) *gorm.DB {
	t.Helper()
	admin := openDB(t, "")
	// Đăng ký đóng admin trước tiên: Cleanup chạy ngược thứ tự nên nó đóng SAU khi đã DROP xong.
	t.Cleanup(func() { closeDB(admin) })

	sweepOnce.Do(func() { sweepOrphans(t, admin, time.Now(), orphanMaxAge) })

	buf := make([]byte, 6)
	_, _ = rand.Read(buf)
	schema := "t_" + hex.EncodeToString(buf)
	// Tạo schema và ghi thời điểm tạo trong CÙNG một transaction: schema nào tồn tại cũng có dấu thời
	// gian, nên lượt quét mồ côi luôn xác định được tuổi.
	err := admin.Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec("CREATE SCHEMA " + schema).Error; err != nil {
			return err
		}
		return tx.Exec(fmt.Sprintf("COMMENT ON SCHEMA %s IS '%s%d'", schema, commentPrefix, time.Now().Unix())).Error
	})
	if err != nil {
		t.Fatalf("tạo schema tạm %s: %v", schema, err)
	}
	t.Cleanup(func() {
		if err := dropWithRetry(func() error { return dropSchema(admin, schema) }, time.Sleep); err != nil {
			t.Errorf("xoá schema tạm %s: %v", schema, err)
		}
	})

	db := openDB(t, fmt.Sprintf(" search_path=%s application_name=%s%s", schema, appNamePrefix, schema))
	t.Cleanup(func() { closeDB(db) }) // chạy trước DROP: DROP không phải chờ kết nối của chính test.
	if migrate != nil {
		if err := migrate(db); err != nil {
			t.Fatalf("migrate schema tạm %s: %v", schema, err)
		}
	}
	return db
}

// dropSchema DROP schema trong 1 transaction có lock_timeout để không treo khi còn phiên khác giữ khoá.
func dropSchema(admin *gorm.DB, schema string) error {
	if !tempSchemaRE.MatchString(schema) {
		return fmt.Errorf("tên schema %q không khớp %s, từ chối drop", schema, tempSchemaPattern)
	}
	return admin.Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec("SET LOCAL lock_timeout = '" + dropLockTimeout + "'").Error; err != nil {
			return err
		}
		return tx.Exec("DROP SCHEMA IF EXISTS " + schema + " CASCADE").Error
	})
}

// dropWithRetry chạy drop, lỗi thì thử lại đúng một lần sau một nhịp ngắn rồi mới trả lỗi.
func dropWithRetry(drop func() error, sleep func(time.Duration)) error {
	err := drop()
	if err == nil {
		return nil
	}
	sleep(500 * time.Millisecond)
	if err2 := drop(); err2 != nil {
		return fmt.Errorf("%w (lần thử đầu: %v)", err2, err)
	}
	return nil
}

// createdAt đọc thời điểm tạo từ comment schema do IsolatedSchema ghi. ok=false khi comment thiếu/sai
// dạng — khi đó KHÔNG xác định được tuổi nên không bao giờ drop.
func createdAt(comment string) (time.Time, bool) {
	m := createdRE.FindStringSubmatch(comment)
	if m == nil {
		return time.Time{}, false
	}
	sec, err := strconv.ParseInt(m[1], 10, 64)
	if err != nil {
		return time.Time{}, false
	}
	return time.Unix(sec, 0), true
}

// isOrphan: schema đã tồn tại quá ngưỡng (theo comment thời điểm tạo).
func isOrphan(comment string, now time.Time, maxAge time.Duration) bool {
	created, ok := createdAt(comment)
	return ok && now.Sub(created) > maxAge
}

// sweepOrphans drop schema `t_<12 hex>` mồ côi do lần chạy test trước bị giết giữa chừng. Điều kiện
// drop (đủ cả ba):
//  1. tên khớp ^t_[0-9a-f]{12}$;
//  2. comment `pgtest:created=<unix>` cho biết đã tồn tại quá maxAge — schema không có comment (tạo
//     bởi bản cũ của IsolatedSchema) KHÔNG xác định được tuổi, nên được để yên;
//  3. không có phiên nào mang application_name `pgtest:<schema>` (kết nối của một lần chạy đang sống).
//
// Lỗi ở đây chỉ được ghi log: dọn rác không được làm hỏng test.
func sweepOrphans(t tb, admin *gorm.DB, now time.Time, maxAge time.Duration) {
	t.Helper()
	var rows []struct {
		Name    string
		Comment string
	}
	err := admin.Raw(`SELECT n.nspname AS name, coalesce(obj_description(n.oid, 'pg_namespace'), '') AS comment
		FROM pg_namespace n WHERE n.nspname ~ ?`, tempSchemaPattern).Scan(&rows).Error
	if err != nil {
		t.Logf("pgtest: không liệt kê được schema tạm để dọn: %v", err)
		return
	}
	for _, r := range rows {
		if !isOrphan(r.Comment, now, maxAge) {
			continue
		}
		var inUse int64
		if err := admin.Raw("SELECT count(*) FROM pg_stat_activity WHERE application_name = ?", appNamePrefix+r.Name).
			Scan(&inUse).Error; err != nil {
			t.Logf("pgtest: không kiểm được phiên đang dùng %s, bỏ qua: %v", r.Name, err)
			continue
		}
		if inUse > 0 {
			continue
		}
		if err := dropWithRetry(func() error { return dropSchema(admin, r.Name) }, time.Sleep); err != nil {
			t.Logf("pgtest: không drop được schema mồ côi %s: %v", r.Name, err)
			continue
		}
		t.Logf("pgtest: đã drop schema mồ côi %s (tạo lúc %s)", r.Name, r.Comment)
	}
}
