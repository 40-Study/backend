package pgtest

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
	"testing"
	"time"

	"gorm.io/gorm"
)

// IsolatedSchema phải (1) ghi vào 1 schema tạm t_*, KHÔNG vào public của DB dùng chung, và (2)
// xoá schema đó khi test xong — review Phase 4, B-2 (không để lại dữ liệu/constraint trên study_db).
func TestIsolatedSchema_StaysOutOfPublicAndIsDropped(t *testing.T) {
	var schema string
	t.Run("inner", func(t *testing.T) {
		db := IsolatedSchema(t, func(db *gorm.DB) error {
			return db.Exec("CREATE TABLE pgtest_probe (id int)").Error
		})
		if err := db.Raw("SELECT current_schema()").Scan(&schema).Error; err != nil {
			t.Fatalf("current_schema: %v", err)
		}
		if !strings.HasPrefix(schema, "t_") {
			t.Fatalf("current_schema = %q, muốn schema tạm t_*", schema)
		}
		var inPublic *string
		db.Raw("SELECT to_regclass('public.pgtest_probe')::text").Scan(&inPublic)
		if inPublic != nil {
			t.Fatal("bảng của test lọt vào schema public")
		}
	})
	var n int64
	Open(t).Raw("SELECT count(*) FROM information_schema.schemata WHERE schema_name = ?", schema).Scan(&n)
	if n != 0 {
		t.Fatalf("schema tạm %s còn tồn tại sau khi test xong", schema)
	}
}

// Trên CI (CI=true) thiếu DB phải FAIL, không được SKIP (review Phase 4, B-2).
func TestMissingDBAction(t *testing.T) {
	cases := map[string]string{"true": "fail", "TRUE": "fail", " true ": "fail", "": "skip", "false": "skip", "1": "skip"}
	for in, want := range cases {
		if got := MissingDBAction(in); got != want {
			t.Fatalf("MissingDBAction(%q) = %q, muốn %q", in, got, want)
		}
	}
}

// ---- Không rò schema (pgtest.go: isolatedSchema / dropWithRetry / sweepOrphans) ----

// fakeTB thay *testing.T để chạy được các nhánh t.Skip/t.Fatalf mà không dừng test thật: Skip/Fatalf
// panic bằng sentinel (giống runtime.Goexit của testing), runCleanups chạy ngược thứ tự như testing.
type fakeTB struct {
	cleanups []func()
	skipped  bool
	fatal    string
	errs     []string
}

type stopSentinel struct{}

func (f *fakeTB) Helper()                   {}
func (f *fakeTB) Logf(string, ...any)       {}
func (f *fakeTB) Cleanup(fn func())         { f.cleanups = append(f.cleanups, fn) }
func (f *fakeTB) Errorf(s string, a ...any) { f.errs = append(f.errs, fmt.Sprintf(s, a...)) }
func (f *fakeTB) Skip(...any)               { f.skipped = true; panic(stopSentinel{}) }
func (f *fakeTB) Fatalf(s string, a ...any) { f.fatal = fmt.Sprintf(s, a...); panic(stopSentinel{}) }
func (f *fakeTB) runCleanups() {
	for i := len(f.cleanups) - 1; i >= 0; i-- {
		f.cleanups[i]()
	}
}

// run chạy fn, nuốt sentinel của Skip/Fatalf rồi chạy cleanup như testing làm khi test kết thúc.
func (f *fakeTB) run(fn func()) {
	func() {
		defer func() {
			if r := recover(); r != nil {
				if _, ok := r.(stopSentinel); !ok {
					panic(r)
				}
			}
		}()
		fn()
	}()
	f.runCleanups()
}

func schemaExists(t *testing.T, name string) bool {
	t.Helper()
	var n int64
	Open(t).Raw("SELECT count(*) FROM pg_namespace WHERE nspname = ?", name).Scan(&n)
	return n > 0
}

var searchPathRE = regexp.MustCompile(`search_path=(t_[0-9a-f]{12})`)

// Lỗi cũ: Cleanup chỉ đăng ký SAU khi mở kết nối thứ hai, nên t.Skip/t.Fatalf ở bước đó bỏ lại schema
// và kết nối admin. Giờ drop + đóng admin phải xảy ra ở mọi nhánh.
func TestIsolatedSchema_SecondConnectionSkips_DropsSchemaAndClosesAdmin(t *testing.T) {
	ft := &fakeTB{}
	var admin *gorm.DB
	var schema string
	calls := 0
	opener := func(_ tb, extra string) *gorm.DB {
		calls++
		if calls == 1 {
			admin = open(t, extra)
			return admin
		}
		schema = searchPathRE.FindStringSubmatch(extra)[1]
		ft.Skip("giả lập: không mở được kết nối thứ hai")
		return nil
	}

	ft.run(func() { isolatedSchema(ft, opener, nil) })

	if !ft.skipped || schema == "" {
		t.Fatalf("nhánh Skip không được chạy (skipped=%v schema=%q)", ft.skipped, schema)
	}
	if schemaExists(t, schema) {
		t.Fatalf("schema %s bị bỏ lại sau khi kết nối thứ hai Skip", schema)
	}
	sqlDB, _ := admin.DB()
	if err := sqlDB.Ping(); err == nil {
		t.Fatal("kết nối admin chưa được đóng")
	}
	if len(ft.errs) != 0 {
		t.Fatalf("không được báo lỗi: %v", ft.errs)
	}
}

func TestIsolatedSchema_MigrateFails_DropsSchema(t *testing.T) {
	ft := &fakeTB{}
	var schema string
	opener := func(_ tb, extra string) *gorm.DB {
		if m := searchPathRE.FindStringSubmatch(extra); m != nil {
			schema = m[1]
		}
		return open(t, extra)
	}

	ft.run(func() {
		isolatedSchema(ft, opener, func(*gorm.DB) error { return errors.New("migrate hỏng") })
	})

	if !strings.Contains(ft.fatal, "migrate") || schema == "" {
		t.Fatalf("muốn Fatalf do migrate, được %q (schema=%q)", ft.fatal, schema)
	}
	if schemaExists(t, schema) {
		t.Fatalf("schema %s bị bỏ lại sau khi migrate lỗi", schema)
	}
}

func TestIsolatedSchema_StampsCreationTimeAndAppName(t *testing.T) {
	db := IsolatedSchema(t, nil)
	var schema, comment, app string
	db.Raw("SELECT current_schema()").Scan(&schema)
	db.Raw("SELECT obj_description(?::regnamespace, 'pg_namespace')", schema).Scan(&comment)
	db.Raw("SELECT current_setting('application_name')").Scan(&app)
	created, ok := createdAt(comment)
	if !ok || time.Since(created) > time.Minute {
		t.Fatalf("comment schema %q không ghi thời điểm tạo hợp lệ", comment)
	}
	if app != appNamePrefix+schema {
		t.Fatalf("application_name = %q, muốn %q", app, appNamePrefix+schema)
	}
}

func TestDropWithRetry(t *testing.T) {
	boom := errors.New("boom")
	cases := []struct {
		name    string
		fails   int
		wantErr bool
		calls   int
	}{
		{"thành công ngay: không retry", 0, false, 1},
		{"lỗi một lần rồi qua: retry đúng một lần", 1, false, 2},
		{"lỗi cả hai lần: báo lỗi, không retry thêm", 5, true, 2},
	}
	for _, c := range cases {
		calls := 0
		err := dropWithRetry(func() error {
			calls++
			if calls <= c.fails {
				return boom
			}
			return nil
		}, func(time.Duration) {})
		if (err != nil) != c.wantErr || calls != c.calls {
			t.Errorf("%s: err=%v calls=%d, muốn lỗi=%v calls=%d", c.name, err, calls, c.wantErr, c.calls)
		}
		if c.wantErr && !errors.Is(err, boom) {
			t.Errorf("%s: lỗi trả về mất nguyên nhân gốc: %v", c.name, err)
		}
	}
}

func TestDropSchema_RefusesNamesOutsideTempPattern(t *testing.T) {
	for _, name := range []string{"public", "t_short", "t_0123456789abc", "t_0123456789AB", "x; DROP SCHEMA public"} {
		if err := dropSchema(nil, name); err == nil {
			t.Errorf("dropSchema(%q) phải từ chối", name)
		}
	}
}

func TestIsOrphan(t *testing.T) {
	now := time.Unix(1_800_000_000, 0)
	at := func(age time.Duration) string { return fmt.Sprintf("%s%d", commentPrefix, now.Add(-age).Unix()) }
	cases := []struct {
		comment string
		want    bool
	}{
		{at(3 * time.Hour), true},
		{at(2*time.Hour + time.Second), true},
		{at(2 * time.Hour), false},
		{at(10 * time.Minute), false},
		{"", false},                   // schema của bản cũ: không biết tuổi -> không drop
		{"pgtest:created=abc", false}, // sai dạng
		{"pgtest:created=", false},
		{"khác: " + at(5*time.Hour), false},
	}
	for _, c := range cases {
		if got := isOrphan(c.comment, now, 2*time.Hour); got != c.want {
			t.Errorf("isOrphan(%q) = %v, muốn %v", c.comment, got, c.want)
		}
	}
}

// Quét mồ côi: chỉ drop schema khớp mẫu, đủ già, không còn phiên dùng; mọi trường hợp còn lại giữ nguyên.
func TestSweepOrphans(t *testing.T) {
	admin := Open(t)
	now := time.Now()
	stamp := func(age time.Duration) string { return fmt.Sprintf("%s%d", commentPrefix, now.Add(-age).Unix()) }
	mk := func(name, comment string) {
		t.Helper()
		if err := admin.Exec("CREATE SCHEMA " + name).Error; err != nil {
			t.Fatalf("tạo %s: %v", name, err)
		}
		t.Cleanup(func() { admin.Exec("DROP SCHEMA IF EXISTS " + name + " CASCADE") })
		if comment != "" {
			if err := admin.Exec(fmt.Sprintf("COMMENT ON SCHEMA %s IS '%s'", name, comment)).Error; err != nil {
				t.Fatalf("comment %s: %v", name, err)
			}
		}
	}
	suffix := fmt.Sprintf("%012x", now.UnixNano()&0xffffffffffff)
	hex := func(p string) string { return "t_" + p + suffix[len(p):] } // luôn đúng 12 ký tự hex
	stale, fresh, inUse, noComment := hex("a"), hex("b"), hex("c"), hex("d")
	mk(stale, stamp(3*time.Hour))
	mk(fresh, stamp(10*time.Minute))
	mk(inUse, stamp(3*time.Hour))
	mk(noComment, "")
	const offPattern = "t_pgtest_sweep_probe" // quá dài/không hex: không thuộc mẫu dù comment đã già
	mk(offPattern, stamp(3*time.Hour))

	// Một lần chạy "đang sống": kết nối mang application_name của schema inUse.
	live := open(t, " application_name="+appNamePrefix+inUse)
	t.Cleanup(func() { closeDB(live) })
	if err := live.Exec("SELECT 1").Error; err != nil {
		t.Fatal(err)
	}

	sweepOrphans(t, admin, now, 2*time.Hour)

	want := map[string]bool{stale: false, fresh: true, inUse: true, noComment: true, offPattern: true}
	for name, exists := range want {
		if got := schemaExists(t, name); got != exists {
			t.Errorf("schema %s: tồn tại=%v, muốn %v", name, got, exists)
		}
	}
	if !schemaExists(t, "public") {
		t.Fatal("schema public biến mất")
	}
}
