package database

// Lane L7 đợt 2 (Postgres thật, schema tạm): bước SỬA MỘT LẦN dữ liệu mà L2 đã đổi sai (fixL2WronglyConvertedInstants)
// và bằng chứng về cách pgx/Postgres ghi thời điểm vào cột `timestamp`.
//
//   - TestL2Fix_CongLai7GioDungMotLan: dữ liệu L2 đã đổi sai (13:00Z, đúng ra 20:00Z) được cộng lại 7 giờ đúng một
//     lần; chạy 3 lần không lệch thêm; dòng tạo sau mốc merge L2, NULL và cột giờ VN không bị đụng. Bỏ UPDATE, bỏ
//     bản ghi đánh dấu hoặc bỏ điều kiện created_at thì ĐỎ.
//   - TestL2Fix_ChayDongThoiChiCongMotLan: 6 kết nối phiên UTC chạy song song, 5 vòng. Bỏ khoá advisory khỏi
//     fixL2WronglyConvertedInstants thì ĐỎ (lỗi tranh chấp bản ghi đánh dấu hoặc cộng nhiều lần).
//   - TestL2Fix_DaDanhDauThiKhongBaoGioCongLai: bản ghi đánh dấu chặn mọi lần cộng sau, kể cả khi sau đó có bảng
//     timestamptz mới (DB mới tạo bằng AutoMigrate).
//   - TestPgxTimestampWriteClock_*: ghim cách ghi (giờ đồng hồ theo múi của time.Time; CURRENT_TIMESTAMP theo
//     múi giờ phiên) làm căn cứ cho bảng quyết định múi giờ nguồn.

import (
	"fmt"
	"sync"
	"testing"
	"time"

	"gorm.io/gorm"
	"study.com/v1/internal/testutil/pgtest"
)

// l2ConvertedTables: hai bảng ở đúng trạng thái sau khi L2 đã đổi sang timestamptz theo giờ VN (sai): người dùng
// nhập 20:00Z, L2 coi là 20:00 VN nên lưu 13:00Z. Dòng 1 tạo trước mốc merge L2, dòng 2 tạo sau (đã đúng sẵn, 20:00Z).
const l2ConvertedTables = `
	CREATE TABLE class_lesson_contents (id int PRIMARY KEY, created_at timestamptz,
		open_date timestamptz, due_date timestamptz, scheduled_at timestamptz, end_at timestamptz);
	CREATE TABLE livestream_sessions (id int PRIMARY KEY, created_at timestamptz,
		scheduled_at timestamptz, started_at timestamptz, ended_at timestamptz);
	INSERT INTO class_lesson_contents VALUES
		(1, '2026-10-01 00:00:00+00', NULL, '2026-10-30 16:59:00+00', '2026-10-08 13:00:00+00', '2026-10-08 14:30:00+00'),
		(2, '2026-10-03 00:00:00+00', NULL, '2026-10-30 23:59:00+00', '2026-10-08 20:00:00+00', '2026-10-08 21:30:00+00');
	INSERT INTO livestream_sessions VALUES
		(1, '2026-10-01 00:00:00+00', '2026-10-08 13:00:00+00', '2026-10-08 13:05:00+00', NULL),
		(2, '2026-10-03 00:00:00+00', '2026-10-08 20:00:00+00', NULL, NULL);
`

func instantAt(t *testing.T, db *gorm.DB, table, column string, id int) string {
	t.Helper()
	var v *string
	q := fmt.Sprintf(`SELECT to_char(%s AT TIME ZONE 'UTC', 'YYYY-MM-DD HH24:MI') FROM %s WHERE id = %d`, column, table, id)
	if err := db.Raw(q).Scan(&v).Error; err != nil {
		t.Fatalf("%s: %v", q, err)
	}
	if v == nil {
		return "NULL"
	}
	return *v
}

// assertL2FixedOnce: trạng thái đúng sau MỘT lần sửa trên l2ConvertedTables.
func assertL2FixedOnce(t *testing.T, db *gorm.DB, label string) {
	t.Helper()
	want := []struct {
		table, col string
		id         int
		val        string
	}{
		{"class_lesson_contents", "scheduled_at", 1, "2026-10-08 20:00"}, // 13:00Z + 7h
		{"class_lesson_contents", "end_at", 1, "2026-10-08 21:30"},
		{"class_lesson_contents", "due_date", 1, "2026-10-30 23:59"}, // đúng cái admin chọn 23:59
		{"class_lesson_contents", "open_date", 1, "NULL"},
		{"livestream_sessions", "scheduled_at", 1, "2026-10-08 20:00"},
		{"livestream_sessions", "started_at", 1, "2026-10-08 13:05"},     // giờ VN, L2 đổi đúng: KHÔNG cộng
		{"class_lesson_contents", "scheduled_at", 2, "2026-10-08 20:00"}, // tạo sau mốc merge: giữ nguyên
		{"class_lesson_contents", "due_date", 2, "2026-10-30 23:59"},
		{"livestream_sessions", "scheduled_at", 2, "2026-10-08 20:00"},
	}
	for _, w := range want {
		if got := instantAt(t, db, w.table, w.col, w.id); got != w.val {
			t.Errorf("%s: %s.%s id=%d = %s, muốn %s", label, w.table, w.col, w.id, got, w.val)
		}
	}
}

func markerCount(t *testing.T, db *gorm.DB) int64 {
	t.Helper()
	var n int64
	if err := db.Raw("SELECT count(*) FROM data_migrations WHERE name = ?", l2UTCInstantFixName).Scan(&n).Error; err != nil {
		t.Fatal(err)
	}
	return n
}

func TestL2Fix_CongLai7GioDungMotLan(t *testing.T) {
	db := pgtest.IsolatedSchema(t, func(*gorm.DB) error { return nil })
	if err := db.Exec(l2ConvertedTables).Error; err != nil {
		t.Fatal(err)
	}
	err := db.Connection(func(conn *gorm.DB) error {
		if err := conn.Exec("SET TIME ZONE 'UTC'").Error; err != nil {
			return err
		}
		for run := 1; run <= 3; run++ {
			if err := migrateInstantColumnsUp(conn); err != nil {
				return fmt.Errorf("lần %d: %w", run, err)
			}
			assertL2FixedOnce(t, conn, fmt.Sprintf("sau lần %d", run))
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if n := markerCount(t, db); n != 1 {
		t.Fatalf("data_migrations có %d bản ghi đánh dấu, muốn đúng 1", n)
	}
}

func TestL2Fix_ChayDongThoiChiCongMotLan(t *testing.T) {
	db := pgtest.IsolatedSchema(t, func(*gorm.DB) error { return nil })
	const rounds, workers = 5, 6
	for round := 1; round <= rounds; round++ {
		if err := db.Exec("DROP TABLE IF EXISTS class_lesson_contents, livestream_sessions, data_migrations").Error; err != nil {
			t.Fatal(err)
		}
		if err := db.Exec(l2ConvertedTables).Error; err != nil {
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
					err := conn.Exec("SET TIME ZONE 'UTC'").Error
					ready.Done()
					if err != nil {
						return err
					}
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
		assertL2FixedOnce(t, db, fmt.Sprintf("vòng %d", round))
		if n := markerCount(t, db); n != 1 {
			t.Fatalf("vòng %d: %d bản ghi đánh dấu, muốn 1", round, n)
		}
	}
}

// DB mới (chưa có bảng nào): bước sửa chỉ ghi dấu. Bảng timestamptz xuất hiện SAU đó (AutoMigrate tạo mới, dữ liệu
// ghi bằng code mới nên đã đúng) không bao giờ bị cộng.
func TestL2Fix_DaDanhDauThiKhongBaoGioCongLai(t *testing.T) {
	db := pgtest.IsolatedSchema(t, func(*gorm.DB) error { return nil })
	if err := migrateInstantColumnsUp(db); err != nil {
		t.Fatal(err)
	}
	if n := markerCount(t, db); n != 1 {
		t.Fatalf("DB mới: %d bản ghi đánh dấu, muốn 1", n)
	}
	if err := db.Exec(l2ConvertedTables).Error; err != nil {
		t.Fatal(err)
	}
	if err := migrateInstantColumnsUp(db); err != nil {
		t.Fatal(err)
	}
	if got := instantAt(t, db, "class_lesson_contents", "scheduled_at", 1); got != "2026-10-08 13:00" {
		t.Fatalf("đã đánh dấu mà vẫn cộng: scheduled_at = %s, muốn giữ 2026-10-08 13:00", got)
	}
}

// Cột còn là `timestamp` (DB chưa từng chạy L2): được đổi ĐÚNG theo UTC bởi convertTimestampColumns, không được cộng
// thêm 7 giờ (cộng thêm = lệch hai lần).
func TestL2Fix_CotConTimestampKhongBiCong(t *testing.T) {
	db := pgtest.IsolatedSchema(t, func(*gorm.DB) error { return nil })
	if err := db.Exec(`
		CREATE TABLE class_lesson_contents (id int PRIMARY KEY, created_at timestamptz, scheduled_at timestamp);
		INSERT INTO class_lesson_contents VALUES (1, '2026-10-01 00:00:00+00', '2026-10-08 20:00:00');`).Error; err != nil {
		t.Fatal(err)
	}
	err := db.Connection(func(conn *gorm.DB) error {
		if err := conn.Exec("SET TIME ZONE 'UTC'").Error; err != nil {
			return err
		}
		return migrateInstantColumnsUp(conn)
	})
	if err != nil {
		t.Fatal(err)
	}
	if got := instantAt(t, db, "class_lesson_contents", "scheduled_at", 1); got != "2026-10-08 20:00" {
		t.Fatalf("scheduled_at = %s, muốn 2026-10-08 20:00 (UTC, không cộng 7 giờ)", got)
	}
}

// Ca của review: admin chọn 23:59 giờ VN, web gửi toISOString() = 16:59Z; pgx ghi giờ đồng hồ UTC "16:59" vào cột
// `timestamp`. Sau migration phải vẫn là 16:59Z = 23:59 giờ VN, không phải 09:59Z = 16:59 VN.
func TestInstantColumns_VoucherAdminChon2359VNVanLa2359VN(t *testing.T) {
	db := pgtest.IsolatedSchema(t, func(*gorm.DB) error { return nil })
	if err := db.Exec(`CREATE TABLE vouchers (id int PRIMARY KEY, start_date timestamp, end_date timestamp)`).Error; err != nil {
		t.Fatal(err)
	}
	vn := time.FixedZone("ICT", 7*3600)
	picked := time.Date(2026, 10, 31, 23, 59, 0, 0, vn) // admin chọn
	sent, err := time.Parse(time.RFC3339, picked.UTC().Format(time.RFC3339))
	if err != nil {
		t.Fatal(err)
	}
	if sent.Location() != time.UTC {
		t.Fatalf("time.Parse của chuỗi Z phải giữ UTC, được %v", sent.Location())
	}
	if err := db.Exec(`INSERT INTO vouchers VALUES (1, ?, ?)`, sent, sent).Error; err != nil {
		t.Fatal(err)
	}
	err = db.Connection(func(conn *gorm.DB) error {
		if err := conn.Exec("SET TIME ZONE 'UTC'").Error; err != nil {
			return err
		}
		return migrateInstantColumnsUp(conn)
	})
	if err != nil {
		t.Fatal(err)
	}
	var back time.Time
	if err := db.Raw(`SELECT end_date FROM vouchers WHERE id = 1`).Scan(&back).Error; err != nil {
		t.Fatal(err)
	}
	if !back.Equal(picked) {
		t.Fatalf("end_date sau migration = %s, muốn cùng instant với %s (23:59 giờ VN)", back.UTC(), picked)
	}
	if got := back.In(vn).Format("15:04"); got != "23:59" {
		t.Fatalf("hiển thị giờ VN = %s, muốn 23:59", got)
	}
}

// Căn cứ cho bảng quyết định: pgx ghi vào cột `timestamp` GIỜ ĐỒNG HỒ theo múi của chính time.Time truyền vào
// (không đổi về UTC, không theo múi giờ phiên). Nên chuỗi "...Z" (UTC) ghi giờ UTC, time.Now() ghi giờ đồng hồ theo
// time.Local của TIẾN TRÌNH (container production `FROM scratch` không có TZ nên là UTC; máy dev Windows là VN).
func TestPgxTimestampWriteClock_TheoMuiCuaTimeTime(t *testing.T) {
	db := pgtest.IsolatedSchema(t, func(*gorm.DB) error { return nil })
	if err := db.Exec(`CREATE TABLE wc (id int PRIMARY KEY, ts timestamp)`).Error; err != nil {
		t.Fatal(err)
	}
	vn := time.FixedZone("ICT", 7*3600)
	instant := time.Date(2026, 10, 8, 13, 0, 0, 0, time.UTC) // 20:00 giờ VN
	cases := []struct {
		id   int
		val  time.Time
		want string
	}{
		{1, instant.In(time.UTC), "2026-10-08 13:00:00"},
		{2, instant.In(vn), "2026-10-08 20:00:00"},
	}
	for _, c := range cases {
		err := db.Connection(func(conn *gorm.DB) error {
			if err := conn.Exec("SET TIME ZONE 'America/New_York'").Error; err != nil { // phiên không được ảnh hưởng
				return err
			}
			return conn.Exec(`INSERT INTO wc VALUES (?, ?)`, c.id, c.val).Error
		})
		if err != nil {
			t.Fatal(err)
		}
		var got string
		if err := db.Raw(`SELECT ts::text FROM wc WHERE id = ?`, c.id).Scan(&got).Error; err != nil || got != c.want {
			t.Errorf("ghi %s: cột timestamp = %q (%v), muốn %q", c.val, got, err, c.want)
		}
	}
}

// CURRENT_TIMESTAMP vào cột `timestamp` ghi giờ đồng hồ theo múi giờ PHIÊN. DSN của backend đặt
// TimeZone=Asia/Ho_Chi_Minh nên ra giờ VN (started_at, participants.*, whiteboard_snapshots.saved_at, assignments.published_at).
func TestPgxTimestampWriteClock_CurrentTimestampTheoMuiGioPhien(t *testing.T) {
	db := pgtest.IsolatedSchema(t, func(*gorm.DB) error { return nil })
	if err := db.Exec(`CREATE TABLE wc (id int PRIMARY KEY, ts timestamp)`).Error; err != nil {
		t.Fatal(err)
	}
	for id, zone := range map[int]string{1: zoneVN, 2: zoneUTC} {
		err := db.Connection(func(conn *gorm.DB) error {
			if err := conn.Exec(fmt.Sprintf("SET TIME ZONE '%s'", zone)).Error; err != nil {
				return err
			}
			return conn.Exec(`INSERT INTO wc VALUES (?, CURRENT_TIMESTAMP)`, id).Error
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	var diff float64
	if err := db.Raw(`SELECT extract(epoch FROM (SELECT ts FROM wc WHERE id = 1) - (SELECT ts FROM wc WHERE id = 2))`).Scan(&diff).Error; err != nil {
		t.Fatal(err)
	}
	if diff < 7*3600-60 || diff > 7*3600+60 {
		t.Fatalf("giờ đồng hồ CURRENT_TIMESTAMP phiên VN hơn phiên UTC %.0f giây, muốn ~25200 (7 giờ)", diff)
	}
}
