package database

// Lane L7 (Postgres thật, schema tạm): chuyển nốt các cột `timestamp` còn lại sang timestamptz bằng CÙNG cơ chế
// của L2 (migrateInstantColumnsUp), và chuyển an toàn khi nhiều tiến trình migrate chạy đồng thời.
//
//   - TestInstantColumns_MoiCotTheoMuiGioNguon: dựng MỌI cột trong instantColumns ở kiểu cũ, chuyển dưới phiên UTC,
//     kiểm instant của từng cột theo múi giờ nguồn đã khai. Đổi zone của một cột (hoặc bỏ AT TIME ZONE theo cột)
//     thì ĐỎ; thêm cột vào danh sách mà quên khai múi giờ hợp lệ cũng ĐỎ.
//   - TestInstantColumns_ChayDongThoiKhongLechGio: nhiều kết nối phiên UTC chạy migration cùng lúc. Trước khi có khoá
//     advisory + giao dịch, kết nối đến sau ALTER tiếp trên cột đã là timestamptz và lệch 7 giờ (ĐỎ).

import (
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"gorm.io/gorm"
	"study.com/v1/internal/testutil/pgtest"
)

// zoneOffsetSeconds: độ lệch so với UTC của các múi giờ nguồn đang dùng. Thêm múi giờ mới thì phải khai ở đây
// (cố ý: test không tự đoán múi giờ lạ).
var zoneOffsetSeconds = map[string]int{zoneVN: 7 * 3600, zoneUTC: 0}

// legacyValue: giờ đồng hồ ghi trong cột kiểu cũ.
const legacyValue = "2026-10-08 20:00:00"

func createLegacyInstantTables(t *testing.T, db *gorm.DB) {
	t.Helper()
	byTable := map[string][]string{}
	var order []string
	for _, c := range instantColumns {
		if _, ok := byTable[c.table]; !ok {
			order = append(order, c.table)
		}
		byTable[c.table] = append(byTable[c.table], c.column)
	}
	for _, table := range order {
		cols := byTable[table]
		defs, names, vals := []string{"id int PRIMARY KEY"}, []string{"id"}, []string{"1"}
		for _, col := range cols {
			defs = append(defs, col+" timestamp")
			names = append(names, col)
			vals = append(vals, "'"+legacyValue+"'")
		}
		sql := fmt.Sprintf("CREATE TABLE %s (%s); INSERT INTO %s (%s) VALUES (%s);",
			table, strings.Join(defs, ", "), table, strings.Join(names, ", "), strings.Join(vals, ", "))
		if err := db.Exec(sql).Error; err != nil {
			t.Fatalf("tạo bảng cũ %s: %v", table, err)
		}
	}
}

func instantUTC(t *testing.T, db *gorm.DB, table, column string) string {
	t.Helper()
	var v string
	q := fmt.Sprintf(`SELECT to_char(%s AT TIME ZONE 'UTC', 'YYYY-MM-DD HH24:MI') FROM %s WHERE id = 1`, column, table)
	if err := db.Raw(q).Scan(&v).Error; err != nil {
		t.Fatalf("%s: %v", q, err)
	}
	return v
}

func TestInstantColumns_MoiCotTheoMuiGioNguon(t *testing.T) {
	db := pgtest.IsolatedSchema(t, func(*gorm.DB) error { return nil })
	createLegacyInstantTables(t, db)

	// Kết nối chạy migration ở múi giờ UTC: kết quả không được phụ thuộc múi giờ phiên.
	err := db.Connection(func(conn *gorm.DB) error {
		if err := conn.Exec("SET TIME ZONE 'UTC'").Error; err != nil {
			return err
		}
		return migrateInstantColumnsUp(conn)
	})
	if err != nil {
		t.Fatal(err)
	}

	types := instantColumnTypes(t, db)
	if len(types) != len(instantColumns) {
		t.Fatalf("thấy %d cột, muốn %d: %v", len(types), len(instantColumns), types)
	}
	local, err := time.Parse("2006-01-02 15:04:05", legacyValue)
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range instantColumns {
		k := c.table + "." + c.column
		off, ok := zoneOffsetSeconds[c.zone]
		if !ok {
			t.Errorf("%s: múi giờ nguồn %q chưa khai trong zoneOffsetSeconds", k, c.zone)
			continue
		}
		if types[k] != typeTimestamptz {
			t.Errorf("%s = %q, muốn %q", k, types[k], typeTimestamptz)
		}
		want := local.Add(-time.Duration(off) * time.Second).Format("2006-01-02 15:04")
		if got := instantUTC(t, db, c.table, c.column); got != want {
			t.Errorf("%s (múi giờ nguồn %s): instant UTC = %s, muốn %s", k, c.zone, got, want)
		}
	}

	// Ghim các quyết định đã chốt theo nơi ghi (đổi một dòng ở đây = đổi quyết định, phải sửa cả bảng ở PR):
	// web gửi toISOString() nên start/end của bài tập là giờ UTC, còn mọi CURRENT_TIMESTAMP/time.Now() là giờ VN.
	pins := map[string]string{
		"assignments.start_time":   "2026-10-08 20:00",
		"assignments.end_time":     "2026-10-08 20:00",
		"assignments.published_at": "2026-10-08 13:00",
		"participants.joined_at":   "2026-10-08 13:00",
		"vouchers.start_date":      "2026-10-08 13:00",
		"user_vouchers.saved_at":   "2026-10-08 13:00",
	}
	for k, want := range pins {
		parts := strings.SplitN(k, ".", 2)
		if got := instantUTC(t, db, parts[0], parts[1]); got != want {
			t.Errorf("quyết định đã chốt %s: instant UTC = %s, muốn %s", k, got, want)
		}
	}
}

// Nhiều tiến trình migrate cùng DB (API khởi động song song, CI): mọi kết nối chạy ở múi giờ UTC, chờ nhau ở một
// rào chắn rồi cùng gọi migrate. Kết quả phải là ĐÚNG MỘT lần chuyển (20:00 VN = 13:00Z), không phải chuyển lần hai
// trên cột đã là timestamptz (13:00Z -> 20:00Z). Lặp vài vòng vì lỗi cũ phụ thuộc thứ tự chạy.
func TestInstantColumns_ChayDongThoiKhongLechGio(t *testing.T) {
	db := pgtest.IsolatedSchema(t, func(*gorm.DB) error { return nil })
	const rounds, workers = 5, 6

	for round := 1; round <= rounds; round++ {
		if err := db.Exec("DROP TABLE IF EXISTS class_lesson_contents, livestream_sessions").Error; err != nil {
			t.Fatal(err)
		}
		if err := db.Exec(legacyInstantTables).Error; err != nil {
			t.Fatal(err)
		}

		start := make(chan struct{})
		errs := make(chan error, workers)
		var ready, done sync.WaitGroup
		for i := 0; i < workers; i++ {
			ready.Add(1)
			done.Add(1)
			go func() {
				defer done.Done()
				errs <- db.Connection(func(conn *gorm.DB) error {
					if err := conn.Exec("SET TIME ZONE 'UTC'").Error; err != nil {
						ready.Done()
						return err
					}
					ready.Done()
					<-start
					return migrateInstantColumnsUp(conn)
				})
			}()
		}
		ready.Wait()
		close(start)
		done.Wait()
		close(errs)
		for err := range errs {
			if err != nil {
				t.Fatalf("vòng %d: %v", round, err)
			}
		}

		if got := instantUTC(t, db, "class_lesson_contents", "scheduled_at"); got != "2026-10-08 13:00" {
			t.Fatalf("vòng %d: scheduled_at = %s UTC, muốn 2026-10-08 13:00 (lệch giờ do chuyển nhiều lần)", round, got)
		}
		if got := instantUTC(t, db, "livestream_sessions", "scheduled_at"); got != "2026-10-08 13:00" {
			t.Fatalf("vòng %d: livestream_sessions.scheduled_at = %s UTC, muốn 2026-10-08 13:00", round, got)
		}
	}
}
