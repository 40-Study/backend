package pgtest

// reuse.go — schema tạm DÙNG LẠI giữa các test của một tiến trình (L8 mục 7).
//
// Vì sao: mỗi IsolatedSchema chạy database.Migrate trên schema trống (AutoMigrate ~123 bảng, ~430 chỉ mục, ~200
// khoá ngoại) rồi DROP SCHEMA ... CASCADE, và đó là ~95% thời gian dựng một fixture; gói internal/service dựng
// hàng trăm fixture nên mất 7 đến 24 phút trên CI. ReusableSchema migrate MỘT lần cho mỗi tiến trình test, rồi
// đầu mỗi test đưa schema về đúng trạng thái "vừa migrate" thay vì dựng lại.
//
// Cô lập không bị hy sinh, được bảo đảm bằng cơ chế chứ không bằng kỷ luật của từng test:
//   - mượn (lease): một schema chỉ cho MỘT test dùng tại một thời điểm. Nơi gọi thứ hai khi schema đang bận (test
//     song song, hoặc subtest dựng thêm fixture khi test cha còn giữ) nhận một IsolatedSchema riêng: chậm hơn
//     nhưng đúng;
//   - đầu mỗi lượt mượn: kiểm "dấu vân tay" cấu trúc (cột, chỉ mục, ràng buộc, trigger...) khớp lúc migrate xong,
//     lệch (test đã chạy DDL) thì dựng lại hẳn; ngắt mọi phiên còn sót của lượt trước; làm rỗng mọi bảng đang có dòng
//     (DELETE theo thứ tự khoá ngoại, hoặc TRUNCATE khi schema có sequence/trigger/vòng khoá ngoại) rồi chép lại dòng
//     baseline do migrate sinh ra; cuối cùng kiểm lại mọi bảng rỗng, trừ bảng baseline
//     đúng số dòng ban đầu, kiểm hỏng thì dựng lại hẳn (không bao giờ trao schema bẩn);
//   - mỗi lượt mượn có pool kết nối RIÊNG, đóng khi test xong: goroutine sót lại của test trước mất kết nối thay vì
//     ghi tiếp vào schema của test sau; mọi SET ở mức phiên chết cùng pool.
//
// Đặt PGTEST_REUSE=0 để tắt và quay về một schema mới cho mỗi test. Schema dùng lại phải được dọn bởi
// CloseReusable trong TestMain của gói; nếu tiến trình bị giết thì sweepOrphans dọn như mọi schema tạm khác.

import (
	"fmt"
	"log"
	"os"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"gorm.io/gorm"
)

const (
	// reuseEnvVar = "0" tắt việc dùng lại schema (mỗi test một IsolatedSchema như trước).
	reuseEnvVar = "PGTEST_REUSE"
	// resetLockTimeout: TRUNCATE chờ khoá tối đa chừng này rồi báo lỗi (khi đó schema được dựng lại).
	resetLockTimeout = "10s"
)

// reusable là một schema đã migrate + bản sao hàng baseline + dấu vân tay cấu trúc.
type reusable struct {
	schema      string
	baseSchema  string // schema phụ chứa bản sao các bảng có dòng ngay sau migrate (vd data_migrations)
	admin       *gorm.DB
	tables      []string
	baseCounts  map[string]int64 // bảng có dòng sau migrate -> số dòng
	fingerprint string
	leased      bool
	// deleteOrder: thứ tự xoá an toàn với khoá ngoại (bảng con trước bảng cha). nil nghĩa là không dùng DELETE được
	// (vòng khoá ngoại giữa các bảng, có sequence cần RESTART IDENTITY, hoặc có trigger mà TRUNCATE không kích hoạt)
	// nên reset dùng TRUNCATE ... CASCADE.
	deleteOrder []string
	// copyOrder: thứ tự chép lại bảng baseline (cha trước con), xác định — xem baselineCopyOrder.
	copyOrder []string
}

var (
	reuseMu      sync.Mutex
	reuseEntries = map[string]*reusable{}
)

// ReuseEnabled cho biết cơ chế dùng lại schema đang bật (PGTEST_REUSE khác "0"). Test chứng minh chính cơ chế dùng
// lại (lượt sau nhận ĐÚNG schema của lượt trước) phải t.Skip khi tắt, vì khi đó mỗi lượt một schema mới là hành vi
// đúng chứ không phải lỗi.
func ReuseEnabled() bool { return os.Getenv(reuseEnvVar) != "0" }

// ReusableSchema trả một kết nối tới schema tạm đã migrate và RỖNG (trừ các dòng baseline do migrate sinh ra),
// dùng lại giữa các test của tiến trình. key định danh hàm migrate: cùng key thì cùng một hàm migrate.
// Xem mô tả đầu file về cách bảo đảm cô lập.
func ReusableSchema(t *testing.T, key string, migrate func(*gorm.DB) error) *gorm.DB {
	t.Helper()
	if !ReuseEnabled() {
		return IsolatedSchema(t, migrate)
	}
	e, ok := leaseReusable(t, key, migrate)
	if !ok {
		// Đang bận: schema dùng lại chỉ cho một test tại một thời điểm.
		return IsolatedSchema(t, migrate)
	}
	// Đăng ký nhả mượn TRƯỚC khi đăng ký đóng pool: Cleanup chạy ngược thứ tự nên pool đóng xong mới nhả mượn.
	t.Cleanup(func() {
		reuseMu.Lock()
		e.leased = false
		reuseMu.Unlock()
	})
	db := open(t, fmt.Sprintf(" search_path=%s application_name=%s%s", e.schema, appNamePrefix, e.schema))
	t.Cleanup(func() { closeDB(db) })
	return db
}

// leaseReusable mượn schema của key (dựng lần đầu, hoặc đưa về trạng thái sạch). ok=false khi đang bận.
func leaseReusable(t *testing.T, key string, migrate func(*gorm.DB) error) (e *reusable, ok bool) {
	t.Helper()
	reuseMu.Lock()
	defer reuseMu.Unlock() // t.Skip/t.Fatalf trong khi dựng (runtime.Goexit) vẫn nhả khoá
	e = reuseEntries[key]
	if e != nil && e.leased {
		return nil, false
	}
	if e != nil {
		if err := e.reset(t); err != nil {
			t.Logf("pgtest: schema dùng lại %s không về được trạng thái sạch (%v), dựng lại", e.schema, err)
			e.drop()
			delete(reuseEntries, key)
			e = nil
		}
	}
	if e == nil {
		e = buildReusable(t, migrate)
		reuseEntries[key] = e
	}
	e.leased = true
	return e, true
}

func buildReusable(t *testing.T, migrate func(*gorm.DB) error) *reusable {
	t.Helper()
	admin := open(t, "")
	sweepOnce.Do(func() { sweepOrphans(t, admin, time.Now(), orphanMaxAge) })

	e := &reusable{admin: admin}
	built := false
	defer func() {
		if !built { // Fatalf/Skip giữa chừng: không rò schema và kết nối admin
			e.drop()
		}
	}()
	e.schema = createTempSchema(t, admin)
	e.baseSchema = createTempSchema(t, admin)

	pool := open(t, fmt.Sprintf(" search_path=%s application_name=%s%s", e.schema, appNamePrefix, e.schema))
	defer closeDB(pool)
	if migrate != nil {
		if err := migrate(pool); err != nil {
			t.Fatalf("migrate schema dùng lại %s: %v", e.schema, err)
		}
	}
	var err error
	if e.tables, err = listTables(admin, e.schema); err != nil {
		t.Fatalf("liệt kê bảng của %s: %v", e.schema, err)
	}
	e.baseCounts = map[string]int64{}
	withRows, err := tablesWithRows(admin, e.schema, e.tables)
	if err != nil {
		t.Fatalf("tìm bảng có dòng sau migrate: %v", err)
	}
	for _, tbl := range withRows {
		if err := admin.Exec(fmt.Sprintf("CREATE TABLE %s AS TABLE %s", qualified(e.baseSchema, tbl), qualified(e.schema, tbl))).Error; err != nil {
			t.Fatalf("sao baseline %s: %v", tbl, err)
		}
		n, err := countRows(admin, e.schema, tbl)
		if err != nil {
			t.Fatalf("đếm baseline %s: %v", tbl, err)
		}
		e.baseCounts[tbl] = n
	}
	if e.fingerprint, err = SchemaFingerprint(admin, e.schema); err != nil {
		t.Fatalf("dấu vân tay %s: %v", e.schema, err)
	}
	if e.deleteOrder, err = planDeleteOrder(admin, e.schema, e.tables); err != nil {
		t.Fatalf("lập thứ tự xoá của %s: %v", e.schema, err)
	}
	childrenFirst, err := fkChildrenFirst(admin, e.schema, e.tables)
	if err != nil {
		t.Fatalf("lập thứ tự khoá ngoại của %s: %v", e.schema, err)
	}
	e.copyOrder = baselineCopyOrder(e.baseCounts, childrenFirst)
	built = true
	return e
}

// planDeleteOrder trả thứ tự xoá bảng an toàn với khoá ngoại (bảng con trước bảng cha), hoặc nil khi phải dùng
// TRUNCATE. Lý do dùng DELETE khi được: TRUNCATE ... CASCADE một bảng gốc (users, courses...) kéo theo MỌI bảng
// có khoá ngoại trỏ tới nó (~100 bảng, mỗi bảng một lần đổi file dữ liệu và chỉ mục), tốn hàng giây cho vài chục
// dòng; DELETE chỉ chạm các dòng thật sự có.
func planDeleteOrder(admin *gorm.DB, schema string, tables []string) ([]string, error) {
	var seq, trg int64
	if err := admin.Raw(`SELECT count(*) FROM pg_class c JOIN pg_namespace n ON n.oid = c.relnamespace
		WHERE n.nspname = ? AND c.relkind = 'S'`, schema).Scan(&seq).Error; err != nil {
		return nil, err
	}
	if err := admin.Raw(`SELECT count(*) FROM pg_trigger t JOIN pg_class c ON c.oid = t.tgrelid JOIN pg_namespace n ON n.oid = c.relnamespace
		WHERE n.nspname = ? AND NOT t.tgisinternal`, schema).Scan(&trg).Error; err != nil {
		return nil, err
	}
	if seq > 0 || trg > 0 {
		return nil, nil
	}
	return fkChildrenFirst(admin, schema, tables)
}

// fkChildrenFirst sắp bảng sao cho bảng con (có khoá ngoại) đứng trước bảng cha, nil khi có vòng khoá ngoại.
// Tách khỏi planDeleteOrder vì thứ tự chép baseline (cha trước con) cần đúng thứ tự này kể cả khi reset dùng TRUNCATE.
func fkChildrenFirst(admin *gorm.DB, schema string, tables []string) ([]string, error) {
	var edges []struct{ Child, Parent string }
	if err := admin.Raw(`SELECT c.relname AS child, p.relname AS parent
		FROM pg_constraint k
		JOIN pg_class c ON c.oid = k.conrelid
		JOIN pg_class p ON p.oid = k.confrelid
		JOIN pg_namespace n ON n.oid = c.relnamespace
		WHERE n.nspname = ? AND k.contype = 'f' AND k.conrelid <> k.confrelid`, schema).Scan(&edges).Error; err != nil {
		return nil, err
	}
	return deleteOrderFromEdges(tables, func(yield func(child, parent string)) {
		for _, e := range edges {
			yield(e.Child, e.Parent)
		}
	}), nil
}

// baselineCopyOrder trả thứ tự CHÉP LẠI các bảng baseline sau khi làm rỗng: bảng cha trước bảng con (ngược với thứ
// tự xoá), để INSERT không vi phạm khoá ngoại giữa hai bảng baseline. Duyệt map thì thứ tự đổi theo từng lần chạy:
// reset lỗi khoá ngoại rồi cơ chế âm thầm dựng lại schema mỗi lượt (đúng nhưng chậm). Khi không có thứ tự khoá ngoại
// (childrenFirst nil: có vòng khoá ngoại) thì theo tên bảng, vẫn xác định.
func baselineCopyOrder(baseCounts map[string]int64, childrenFirst []string) []string {
	out := make([]string, 0, len(baseCounts))
	seen := make(map[string]bool, len(baseCounts))
	for i := len(childrenFirst) - 1; i >= 0; i-- {
		if tbl := childrenFirst[i]; !seen[tbl] {
			if _, isBase := baseCounts[tbl]; isBase {
				out = append(out, tbl)
				seen[tbl] = true
			}
		}
	}
	rest := make([]string, 0, len(baseCounts)-len(out))
	for tbl := range baseCounts {
		if !seen[tbl] {
			rest = append(rest, tbl)
		}
	}
	sort.Strings(rest)
	return append(out, rest...)
}

// deleteOrderFromEdges sắp bảng sao cho mọi bảng con đứng trước bảng cha của nó; nil khi có vòng giữa các bảng.
// Khoá ngoại tự trỏ (child = parent) không tạo vòng: một câu DELETE kiểm ở cuối câu lệnh.
func deleteOrderFromEdges(tables []string, edges func(yield func(child, parent string))) []string {
	pending := map[string]map[string]bool{} // bảng -> các bảng con CHƯA xoá trỏ tới nó
	for _, tbl := range tables {
		pending[tbl] = map[string]bool{}
	}
	edges(func(child, parent string) {
		if _, ok := pending[parent]; ok && child != parent {
			pending[parent][child] = true
		}
	})
	order := make([]string, 0, len(tables))
	done := map[string]bool{}
	for len(order) < len(tables) {
		progressed := false
		for _, tbl := range tables { // tables đã sắp theo tên nên kết quả ổn định
			if done[tbl] || len(pending[tbl]) > 0 {
				continue
			}
			done[tbl] = true
			order = append(order, tbl)
			for parent := range pending {
				delete(pending[parent], tbl)
			}
			progressed = true
		}
		if !progressed {
			return nil
		}
	}
	return order
}

// reset đưa schema về trạng thái ngay sau migrate, hoặc trả lỗi (nơi gọi dựng lại hẳn).
func (e *reusable) reset(t tb) error {
	fp, err := SchemaFingerprint(e.admin, e.schema)
	if err != nil {
		return fmt.Errorf("dấu vân tay: %w", err)
	}
	if fp != e.fingerprint {
		return fmt.Errorf("cấu trúc schema đã bị đổi (DDL trong test trước)")
	}
	if err := e.killStraySessions(t); err != nil {
		return err
	}
	dirty, err := tablesWithRows(e.admin, e.schema, e.tables)
	if err != nil {
		return err
	}
	targets := map[string]bool{}
	for _, tbl := range dirty {
		targets[tbl] = true
	}
	for tbl := range e.baseCounts {
		targets[tbl] = true // bảng baseline luôn được chép lại: test có thể đã SỬA dòng mà không đổi số dòng
	}
	if len(targets) > 0 {
		err = e.admin.Transaction(func(tx *gorm.DB) error {
			if err := tx.Exec("SET LOCAL lock_timeout = '" + resetLockTimeout + "'").Error; err != nil {
				return err
			}
			if err := e.emptyTables(tx, targets); err != nil {
				return err
			}
			for _, tbl := range e.copyOrder {
				if err := tx.Exec(fmt.Sprintf("INSERT INTO %s SELECT * FROM %s", qualified(e.schema, tbl), qualified(e.baseSchema, tbl))).Error; err != nil {
					return err
				}
			}
			return nil
		})
		if err != nil {
			return fmt.Errorf("làm rỗng bảng: %w", err)
		}
	}
	return e.verifyClean()
}

// emptyTables làm rỗng các bảng trong targets: DELETE theo thứ tự khoá ngoại khi được (xem planDeleteOrder), không
// thì TRUNCATE ... CASCADE (cũng đặt lại sequence).
func (e *reusable) emptyTables(tx *gorm.DB, targets map[string]bool) error {
	if e.deleteOrder != nil {
		for _, tbl := range e.deleteOrder {
			if !targets[tbl] {
				continue
			}
			if err := tx.Exec("DELETE FROM " + qualified(e.schema, tbl)).Error; err != nil {
				return fmt.Errorf("DELETE %s: %w", tbl, err)
			}
		}
		return nil
	}
	names := make([]string, 0, len(targets))
	for tbl := range targets {
		names = append(names, qualified(e.schema, tbl))
	}
	sort.Strings(names)
	if err := tx.Exec("TRUNCATE TABLE " + strings.Join(names, ", ") + " RESTART IDENTITY CASCADE").Error; err != nil {
		return fmt.Errorf("TRUNCATE: %w", err)
	}
	return nil
}

// verifyClean: mọi bảng rỗng, trừ bảng baseline phải đúng số dòng ban đầu.
func (e *reusable) verifyClean() error {
	left, err := tablesWithRows(e.admin, e.schema, e.tables)
	if err != nil {
		return err
	}
	seen := map[string]bool{}
	for _, tbl := range left {
		want, isBase := e.baseCounts[tbl]
		if !isBase {
			return fmt.Errorf("sau reset bảng %s vẫn còn dòng", tbl)
		}
		got, err := countRows(e.admin, e.schema, tbl)
		if err != nil {
			return err
		}
		if got != want {
			return fmt.Errorf("sau reset bảng baseline %s có %d dòng, muốn %d", tbl, got, want)
		}
		seen[tbl] = true
	}
	for tbl, want := range e.baseCounts {
		if !seen[tbl] && want > 0 {
			return fmt.Errorf("sau reset bảng baseline %s mất dòng", tbl)
		}
	}
	return nil
}

// killStraySessions ngắt phiên còn sót của lượt mượn trước (pool đã đóng nên thường không có). Phiên sót có thể
// là goroutine của test trước giữ kết nối riêng; ngắt để nó không ghi tiếp vào schema của test sau.
func (e *reusable) killStraySessions(t tb) error {
	app := appNamePrefix + e.schema
	var pids []int64
	if err := e.admin.Raw("SELECT pid FROM pg_stat_activity WHERE application_name = ? AND pid <> pg_backend_pid()", app).Scan(&pids).Error; err != nil {
		return fmt.Errorf("tìm phiên sót: %w", err)
	}
	if len(pids) == 0 {
		return nil
	}
	t.Logf("pgtest: ngắt %d phiên còn sót trên schema dùng lại %s", len(pids), e.schema)
	for _, pid := range pids {
		if err := e.admin.Exec("SELECT pg_terminate_backend(?)", pid).Error; err != nil {
			return fmt.Errorf("ngắt phiên %d: %w", pid, err)
		}
	}
	// pg_terminate_backend không chờ phiên thoát: chờ tới khi hết thật để TRUNCATE không vướng khoá của nó.
	for i := 0; i < 40; i++ {
		var n int64
		if err := e.admin.Raw("SELECT count(*) FROM pg_stat_activity WHERE application_name = ? AND pid <> pg_backend_pid()", app).Scan(&n).Error; err != nil {
			return err
		}
		if n == 0 {
			return nil
		}
		time.Sleep(50 * time.Millisecond)
	}
	return fmt.Errorf("phiên sót trên %s chưa thoát sau khi bị ngắt", e.schema)
}

// drop xoá hai schema và đóng kết nối admin. Lỗi chỉ log: dọn rác không được làm hỏng test.
func (e *reusable) drop() {
	if e.admin == nil {
		return
	}
	for _, s := range []string{e.schema, e.baseSchema} {
		if s == "" {
			continue
		}
		schema := s
		if err := dropWithRetry(func() error { return dropSchema(e.admin, schema) }, time.Sleep); err != nil {
			log.Printf("pgtest: không xoá được schema dùng lại %s: %v", schema, err)
		}
	}
	closeDB(e.admin)
	e.admin = nil
}

// CloseReusable xoá mọi schema dùng lại của tiến trình. Gọi từ TestMain của gói dùng ReusableSchema, sau m.Run().
func CloseReusable() {
	reuseMu.Lock()
	defer reuseMu.Unlock()
	for key, e := range reuseEntries {
		e.drop()
		delete(reuseEntries, key)
	}
}

// ----- truy vấn catalog -----

func quoteIdent(s string) string { return `"` + strings.ReplaceAll(s, `"`, `""`) + `"` }

func qualified(schema, table string) string { return quoteIdent(schema) + "." + quoteIdent(table) }

func listTables(admin *gorm.DB, schema string) ([]string, error) {
	var out []string
	err := admin.Raw(`SELECT c.relname FROM pg_class c JOIN pg_namespace n ON n.oid = c.relnamespace
		WHERE n.nspname = ? AND c.relkind = 'r' ORDER BY c.relname`, schema).Scan(&out).Error
	return out, err
}

// tablesWithRows trả các bảng trong tables hiện có ít nhất một dòng, bằng MỘT truy vấn.
func tablesWithRows(admin *gorm.DB, schema string, tables []string) ([]string, error) {
	if len(tables) == 0 {
		return nil, nil
	}
	parts := make([]string, 0, len(tables))
	for _, tbl := range tables {
		parts = append(parts, fmt.Sprintf("SELECT '%s' AS t WHERE EXISTS (SELECT 1 FROM %s)", strings.ReplaceAll(tbl, "'", "''"), qualified(schema, tbl)))
	}
	var out []string
	err := admin.Raw(strings.Join(parts, " UNION ALL ")).Scan(&out).Error
	return out, err
}

func countRows(admin *gorm.DB, schema, table string) (int64, error) {
	var n int64
	err := admin.Raw("SELECT count(*) FROM " + qualified(schema, table)).Scan(&n).Error
	return n, err
}

// SchemaFingerprint là mã băm cấu trúc của một schema: cột (kiểu, NOT NULL, mặc định), chỉ mục, ràng buộc, trigger,
// hàm và mọi quan hệ không phải bảng/chỉ mục (sequence, view...). Tên schema bị bỏ khỏi định nghĩa nên hai schema
// có cùng cấu trúc cho cùng dấu vân tay (so sánh schema dùng lại với schema vừa migrate sạch).
func SchemaFingerprint(db *gorm.DB, schema string) (string, error) {
	if !tempSchemaRE.MatchString(schema) {
		return "", fmt.Errorf("tên schema %q không khớp %s", schema, tempSchemaPattern)
	}
	q := strings.ReplaceAll(schemaFingerprintSQL, "@S", "'"+schema+"'")
	var fp string
	if err := db.Raw(q).Scan(&fp).Error; err != nil {
		return "", err
	}
	return fp, nil
}

const schemaFingerprintSQL = `
SELECT md5(coalesce(string_agg(replace(x, @S || '.', ''), E'\n' ORDER BY replace(x, @S || '.', '')), '')) FROM (
  SELECT 'col ' || c.relname || '.' || a.attname || ' ' || format_type(a.atttypid, a.atttypmod) || ' ' || a.attnotnull::text
         || ' ' || coalesce(pg_get_expr(d.adbin, d.adrelid), '') AS x
    FROM pg_class c
    JOIN pg_namespace n ON n.oid = c.relnamespace
    JOIN pg_attribute a ON a.attrelid = c.oid AND a.attnum > 0 AND NOT a.attisdropped
    LEFT JOIN pg_attrdef d ON d.adrelid = c.oid AND d.adnum = a.attnum
   WHERE n.nspname = @S AND c.relkind = 'r'
  UNION ALL
  SELECT 'idx ' || pg_get_indexdef(i.indexrelid)
    FROM pg_index i
    JOIN pg_class c ON c.oid = i.indrelid
    JOIN pg_namespace n ON n.oid = c.relnamespace
   WHERE n.nspname = @S
  UNION ALL
  SELECT 'con ' || c.relname || ' ' || k.conname || ' ' || pg_get_constraintdef(k.oid)
    FROM pg_constraint k
    JOIN pg_class c ON c.oid = k.conrelid
    JOIN pg_namespace n ON n.oid = c.relnamespace
   WHERE n.nspname = @S
  UNION ALL
  SELECT 'rel ' || c.relname || ' ' || c.relkind::text
    FROM pg_class c
    JOIN pg_namespace n ON n.oid = c.relnamespace
   WHERE n.nspname = @S AND c.relkind NOT IN ('r', 'i')
  UNION ALL
  SELECT 'trg ' || c.relname || ' ' || pg_get_triggerdef(t.oid)
    FROM pg_trigger t
    JOIN pg_class c ON c.oid = t.tgrelid
    JOIN pg_namespace n ON n.oid = c.relnamespace
   WHERE n.nspname = @S AND NOT t.tgisinternal
  UNION ALL
  SELECT 'fn ' || p.proname || ' ' || pg_get_function_identity_arguments(p.oid)
    FROM pg_proc p
    JOIN pg_namespace n ON n.oid = p.pronamespace
   WHERE n.nspname = @S
  UNION ALL
  SELECT 'typ ' || t.typname || ' ' || t.typtype::text
    FROM pg_type t
    JOIN pg_namespace n ON n.oid = t.typnamespace
   WHERE n.nspname = @S AND t.typrelid = 0 AND t.typelem = 0
) s`
