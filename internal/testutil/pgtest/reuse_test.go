package pgtest

import (
	"fmt"
	"os"
	"testing"

	"gorm.io/gorm"
)

// TestMain dọn mọi schema dùng lại mà các test của gói này đã dựng.
func TestMain(m *testing.M) {
	code := m.Run()
	CloseReusable()
	os.Exit(code)
}

// miniMigrate: schema nhỏ có đủ thứ cần kiểm: khoá ngoại, chỉ mục, sequence và một dòng baseline (như data_migrations
// của migration thật).
func miniMigrate(db *gorm.DB) error {
	return db.Exec(`
		CREATE TABLE accounts (id serial PRIMARY KEY, name text NOT NULL UNIQUE);
		CREATE TABLE orders (id bigserial PRIMARY KEY, account_id int NOT NULL REFERENCES accounts(id), total int NOT NULL DEFAULT 0);
		CREATE INDEX orders_total_idx ON orders (total);
		CREATE TABLE settings (k text PRIMARY KEY, v text NOT NULL);
		INSERT INTO settings VALUES ('fee', '8');`).Error
}

func currentSchema(t *testing.T, db *gorm.DB) string {
	t.Helper()
	var s string
	if err := db.Raw("SELECT current_schema()").Scan(&s).Error; err != nil {
		t.Fatalf("current_schema: %v", err)
	}
	return s
}

func mustExec(t *testing.T, db *gorm.DB, sql string, args ...any) {
	t.Helper()
	if err := db.Exec(sql, args...).Error; err != nil {
		t.Fatalf("exec %q: %v", sql, err)
	}
}

func mustCount(t *testing.T, db *gorm.DB, table string) int64 {
	t.Helper()
	var n int64
	if err := db.Raw("SELECT count(*) FROM " + table).Scan(&n).Error; err != nil {
		t.Fatalf("count %s: %v", table, err)
	}
	return n
}

// Lượt mượn thứ hai nhận ĐÚNG schema của lượt đầu (không có thế thì test không chứng minh gì về việc dùng lại) và
// sạch hoàn toàn: dữ liệu, dòng baseline bị sửa/xoá, sequence, khoá ngoại.
func TestReusableSchema_ResetsBetweenLeases(t *testing.T) {
	const key = "reuse-test-reset"
	var first string
	t.Run("dirty", func(t *testing.T) {
		db := ReusableSchema(t, key, miniMigrate)
		first = currentSchema(t, db)
		mustExec(t, db, "INSERT INTO accounts (name) VALUES ('a'), ('b')")
		mustExec(t, db, "INSERT INTO orders (account_id, total) SELECT id, 5 FROM accounts")
		mustExec(t, db, "UPDATE settings SET v = '99'") // sửa dòng baseline mà không đổi số dòng
	})
	t.Run("deleted-baseline", func(t *testing.T) {
		db := ReusableSchema(t, key, miniMigrate)
		if got := currentSchema(t, db); got != first {
			t.Fatalf("schema = %s, muốn dùng lại %s", got, first)
		}
		mustExec(t, db, "DELETE FROM settings") // xoá hẳn dòng baseline
	})
	t.Run("clean", func(t *testing.T) {
		db := ReusableSchema(t, key, miniMigrate)
		if got := currentSchema(t, db); got != first {
			t.Fatalf("schema = %s, muốn dùng lại %s", got, first)
		}
		if n := mustCount(t, db, "accounts") + mustCount(t, db, "orders"); n != 0 {
			t.Fatalf("còn %d dòng của test trước", n)
		}
		var v string
		if err := db.Raw("SELECT v FROM settings WHERE k = 'fee'").Scan(&v).Error; err != nil || v != "8" {
			t.Fatalf("dòng baseline settings.fee = %q (err=%v), muốn 8 như ngay sau migrate", v, err)
		}
		if n := mustCount(t, db, "settings"); n != 1 {
			t.Fatalf("settings có %d dòng, muốn đúng 1", n)
		}
		mustExec(t, db, "INSERT INTO accounts (name) VALUES ('c')")
		var id int
		db.Raw("SELECT id FROM accounts WHERE name = 'c'").Scan(&id)
		if id != 1 {
			t.Fatalf("id đầu tiên sau reset = %d, muốn 1 (sequence phải được đặt lại như schema mới)", id)
		}
		if err := db.Exec("INSERT INTO orders (account_id) VALUES (424242)").Error; err == nil {
			t.Fatal("khoá ngoại orders.account_id không còn hiệu lực")
		}
	})
}

// Nơi gọi thứ hai khi schema đang bận (test cha chưa xong) nhận một schema RIÊNG, không bao giờ cùng schema.
func TestReusableSchema_BusyLeaseFallsBackToPrivateSchema(t *testing.T) {
	const key = "reuse-test-busy"
	db1 := ReusableSchema(t, key, miniMigrate)
	db2 := ReusableSchema(t, key, miniMigrate)
	if a, b := currentSchema(t, db1), currentSchema(t, db2); a == b {
		t.Fatalf("hai lượt mượn đồng thời cùng schema %s", a)
	}
	mustExec(t, db1, "INSERT INTO accounts (name) VALUES ('only-in-1')")
	if n := mustCount(t, db2, "accounts"); n != 0 {
		t.Fatalf("dữ liệu của lượt 1 hiện ở lượt 2 (%d dòng)", n)
	}
}

// Test chạy DDL làm cấu trúc lệch dấu vân tay: lượt sau phải nhận schema DỰNG LẠI, không phải schema đã bị sửa.
func TestReusableSchema_DDLTriggersRebuild(t *testing.T) {
	const key = "reuse-test-ddl"
	var first string
	t.Run("ddl", func(t *testing.T) {
		db := ReusableSchema(t, key, miniMigrate)
		first = currentSchema(t, db)
		mustExec(t, db, "ALTER TABLE accounts ADD COLUMN extra text")
	})
	t.Run("rebuilt", func(t *testing.T) {
		db := ReusableSchema(t, key, miniMigrate)
		if got := currentSchema(t, db); got == first {
			t.Fatalf("vẫn nhận schema %s đã bị đổi cấu trúc", got)
		}
		if db.Migrator().HasColumn("accounts", "extra") {
			t.Fatal("cột extra do DDL của test trước còn lại")
		}
	})
	t.Run("old-schema-dropped", func(t *testing.T) {
		db := ReusableSchema(t, key, miniMigrate)
		var n int64
		db.Raw("SELECT count(*) FROM pg_namespace WHERE nspname = ?", first).Scan(&n)
		if n != 0 {
			t.Fatalf("schema cũ %s chưa bị xoá khi dựng lại", first)
		}
	})
}

// Phiên sót lại của lượt trước (goroutine giữ kết nối riêng) bị ngắt khi lượt sau mượn.
func TestReusableSchema_StraySessionIsTerminated(t *testing.T) {
	const key = "reuse-test-stray"
	var stray *gorm.DB
	var strayPID int64
	t.Run("leak", func(t *testing.T) {
		db := ReusableSchema(t, key, miniMigrate)
		schema := currentSchema(t, db)
		// Kết nối riêng mang cùng application_name nhưng KHÔNG đăng ký đóng: mô phỏng goroutine sót lại.
		stray = open(t, fmt.Sprintf(" search_path=%s application_name=%s%s", schema, appNamePrefix, schema))
		sqlDB, _ := stray.DB()
		sqlDB.SetMaxOpenConns(1)
		if err := stray.Raw("SELECT pg_backend_pid()").Scan(&strayPID).Error; err != nil || strayPID == 0 {
			t.Fatalf("lấy pid phiên sót: pid=%d err=%v", strayPID, err)
		}
	})
	defer func() {
		if stray != nil {
			closeDB(stray)
		}
	}()
	t.Run("next", func(t *testing.T) {
		db := ReusableSchema(t, key, miniMigrate)
		var alive int64
		if err := db.Raw("SELECT count(*) FROM pg_stat_activity WHERE pid = ?", strayPID).Scan(&alive).Error; err != nil {
			t.Fatalf("kiểm phiên sót: %v", err)
		}
		if alive != 0 {
			t.Fatalf("phiên sót pid=%d vẫn sống sau khi mượn lại schema", strayPID)
		}
	})
}

// Dấu vân tay bỏ tên schema (hai schema cùng cấu trúc ra cùng mã) và đổi khi cấu trúc đổi.
func TestSchemaFingerprint_SameStructureSameFingerprint(t *testing.T) {
	a := IsolatedSchema(t, miniMigrate)
	b := IsolatedSchema(t, miniMigrate)
	fa, err := SchemaFingerprint(a, currentSchema(t, a))
	if err != nil {
		t.Fatal(err)
	}
	fb, err := SchemaFingerprint(b, currentSchema(t, b))
	if err != nil {
		t.Fatal(err)
	}
	if fa != fb {
		t.Fatalf("hai schema cùng cấu trúc ra hai dấu vân tay khác nhau: %s vs %s", fa, fb)
	}
	for name, ddl := range map[string]string{
		"thêm cột":     "ALTER TABLE accounts ADD COLUMN extra text",
		"thêm chỉ mục": "CREATE INDEX accounts_name_idx2 ON accounts (name)",
		"bỏ ràng buộc": "ALTER TABLE orders DROP CONSTRAINT orders_account_id_fkey",
		"thêm bảng":    "CREATE TABLE extra_tbl (id int)",
		"đổi mặc định": "ALTER TABLE orders ALTER COLUMN total SET DEFAULT 7",
		// Cùng tên, cùng số lượng, khác định nghĩa: chỉ băm định nghĩa mới thấy.
		"đổi định nghĩa chỉ mục": "DROP INDEX orders_total_idx; CREATE INDEX orders_total_idx ON orders (account_id)",
		"đổi hành vi khoá ngoại": "ALTER TABLE orders DROP CONSTRAINT orders_account_id_fkey, ADD CONSTRAINT orders_account_id_fkey FOREIGN KEY (account_id) REFERENCES accounts(id) ON DELETE CASCADE",
	} {
		c := IsolatedSchema(t, miniMigrate)
		mustExec(t, c, ddl)
		fc, err := SchemaFingerprint(c, currentSchema(t, c))
		if err != nil {
			t.Fatal(err)
		}
		if fc == fa {
			t.Errorf("%s: dấu vân tay không đổi", name)
		}
	}
}

// PGTEST_REUSE=0: mỗi lượt một schema mới như IsolatedSchema.
func TestReusableSchema_DisabledByEnvUsesFreshSchemas(t *testing.T) {
	t.Setenv(reuseEnvVar, "0")
	var first string
	t.Run("one", func(t *testing.T) { first = currentSchema(t, ReusableSchema(t, "reuse-test-off", miniMigrate)) })
	t.Run("two", func(t *testing.T) {
		if got := currentSchema(t, ReusableSchema(t, "reuse-test-off", miniMigrate)); got == first {
			t.Fatalf("PGTEST_REUSE=0 mà vẫn dùng lại schema %s", got)
		}
	})
}

// CloseReusable xoá cả schema chính lẫn schema baseline.
func TestCloseReusable_DropsBothSchemas(t *testing.T) {
	const key = "reuse-test-close"
	var main string
	t.Run("lease", func(t *testing.T) { main = currentSchema(t, ReusableSchema(t, key, miniMigrate)) })
	reuseMu.Lock()
	base := reuseEntries[key].baseSchema
	reuseMu.Unlock()
	if base == "" || base == main {
		t.Fatalf("baseSchema = %q, main = %q", base, main)
	}
	CloseReusable()
	check := Open(t)
	for _, s := range []string{main, base} {
		var n int64
		check.Raw("SELECT count(*) FROM pg_namespace WHERE nspname = ?", s).Scan(&n)
		if n != 0 {
			t.Errorf("schema %s còn sau CloseReusable", s)
		}
	}
}

// ----- đường DELETE (schema không có sequence/trigger, không vòng khoá ngoại) -----

func miniMigrateNoSeq(db *gorm.DB) error {
	return db.Exec(`
		CREATE TABLE parents (id int PRIMARY KEY, name text NOT NULL);
		CREATE TABLE children (id int PRIMARY KEY, parent_id int NOT NULL REFERENCES parents(id));
		CREATE TABLE grandchildren (id int PRIMARY KEY, child_id int NOT NULL REFERENCES children(id) ON DELETE RESTRICT);
		CREATE TABLE tree (id int PRIMARY KEY, up int REFERENCES tree(id));
		CREATE TABLE meta (k text PRIMARY KEY, v text NOT NULL);
		INSERT INTO meta VALUES ('version', '1');`).Error
}

func reuseEntry(key string) *reusable {
	reuseMu.Lock()
	defer reuseMu.Unlock()
	return reuseEntries[key]
}

func TestReusableSchema_DeletePathEmptiesTablesInForeignKeyOrder(t *testing.T) {
	const key = "reuse-test-delete"
	var first string
	t.Run("dirty", func(t *testing.T) {
		db := ReusableSchema(t, key, miniMigrateNoSeq)
		first = currentSchema(t, db)
		if reuseEntry(key).deleteOrder == nil {
			t.Fatal("schema không có sequence/trigger/vòng khoá ngoại mà không dùng đường DELETE")
		}
		mustExec(t, db, "INSERT INTO parents VALUES (1, 'p')")
		mustExec(t, db, "INSERT INTO children VALUES (1, 1)")
		mustExec(t, db, "INSERT INTO grandchildren VALUES (1, 1)")
		mustExec(t, db, "INSERT INTO tree VALUES (1, NULL), (2, 1), (3, 2)") // khoá ngoại tự trỏ
		mustExec(t, db, "UPDATE meta SET v = '2'")
	})
	t.Run("clean", func(t *testing.T) {
		db := ReusableSchema(t, key, miniMigrateNoSeq)
		if got := currentSchema(t, db); got != first {
			t.Fatalf("schema = %s, muốn dùng lại %s (reset DELETE lỗi nên bị dựng lại?)", got, first)
		}
		for _, tbl := range []string{"parents", "children", "grandchildren", "tree"} {
			if n := mustCount(t, db, tbl); n != 0 {
				t.Fatalf("%s còn %d dòng", tbl, n)
			}
		}
		var v string
		db.Raw("SELECT v FROM meta WHERE k = 'version'").Scan(&v)
		if v != "1" {
			t.Fatalf("meta.version = %q, muốn 1", v)
		}
	})
}

// Schema có sequence cần RESTART IDENTITY nên reset dùng TRUNCATE (deleteOrder = nil).
func TestReusableSchema_SchemaWithSequencesFallsBackToTruncate(t *testing.T) {
	const key = "reuse-test-truncate"
	_ = ReusableSchema(t, key, miniMigrate)
	if reuseEntry(key).deleteOrder != nil {
		t.Fatal("schema có sequence mà vẫn dùng đường DELETE (sequence sẽ không được đặt lại)")
	}
}

func TestDeleteOrderFromEdges(t *testing.T) {
	tables := []string{"a_parent", "b_child", "c_grand", "d_loner", "e_self"}
	edges := func(pairs ...[2]string) func(func(child, parent string)) {
		return func(yield func(child, parent string)) {
			for _, p := range pairs {
				yield(p[0], p[1])
			}
		}
	}
	pos := func(order []string) map[string]int {
		m := map[string]int{}
		for i, tbl := range order {
			m[tbl] = i
		}
		return m
	}

	order := deleteOrderFromEdges(tables, edges(
		[2]string{"b_child", "a_parent"}, [2]string{"c_grand", "b_child"},
		[2]string{"e_self", "e_self"},        // tự trỏ: không phải vòng
		[2]string{"b_child", "outside_tbl"})) // bảng ngoài danh sách: bỏ qua
	if len(order) != len(tables) {
		t.Fatalf("order = %v, muốn đủ %d bảng", order, len(tables))
	}
	p := pos(order)
	if !(p["c_grand"] < p["b_child"] && p["b_child"] < p["a_parent"]) {
		t.Fatalf("order = %v, muốn con trước cha: c_grand < b_child < a_parent", order)
	}

	if got := deleteOrderFromEdges(tables, edges([2]string{"a_parent", "b_child"}, [2]string{"b_child", "a_parent"})); got != nil {
		t.Fatalf("có vòng khoá ngoại mà order = %v, muốn nil (phải dùng TRUNCATE)", got)
	}
}

// verifyClean là lưới an toàn cuối của reset: bảng thường còn dòng, hoặc bảng baseline sai số dòng, đều bị bắt (khi đó
// schema bị dựng lại thay vì trao cho test sau).
func TestReusableSchema_VerifyCleanDetectsLeftovers(t *testing.T) {
	const key = "reuse-test-verify"
	db := ReusableSchema(t, key, miniMigrate)
	e := reuseEntry(key)
	if err := e.verifyClean(); err != nil {
		t.Fatalf("schema vừa mượn phải sạch: %v", err)
	}
	mustExec(t, db, "INSERT INTO accounts (name) VALUES ('leftover')")
	if err := e.verifyClean(); err == nil {
		t.Fatal("bảng thường còn dòng mà verifyClean không báo")
	}
	mustExec(t, db, "DELETE FROM accounts")
	mustExec(t, db, "DELETE FROM settings")
	if err := e.verifyClean(); err == nil {
		t.Fatal("bảng baseline mất dòng mà verifyClean không báo")
	}
	mustExec(t, db, "INSERT INTO settings VALUES ('fee', '8'), ('extra', '1')")
	if err := e.verifyClean(); err == nil {
		t.Fatal("bảng baseline thừa dòng mà verifyClean không báo")
	}
}
