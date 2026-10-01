package database

// Cột thời điểm buổi học timestamp -> timestamptz (Postgres thật, schema tạm pgtest.IsolatedSchema, DROP khi xong).
//
// Trước bản sửa: model khai `type:timestamp` nên Migrate tạo `timestamp without time zone`, ghi một thời điểm
// có múi giờ rồi đọc lại ra instant lệch 7 giờ (TestInstantColumns_MigrateTaoTimestamptzVaGiuInstant ĐỎ).
// Bỏ migrateInstantColumnsUp hoặc bỏ `AT TIME ZONE` thì TestInstantColumns_ChuyenDuLieuCuTheoGioVN ĐỎ (phiên
// UTC cho 20:00Z thay vì 13:00Z). Bỏ migrateInstantColumnsDown/đổi chiều thì TestInstantColumns_HoanTac ĐỎ.

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"study.com/v1/internal/model"
	"study.com/v1/internal/testutil/pgtest"
)

func instantColumnTypes(t *testing.T, db *gorm.DB) map[string]string {
	t.Helper()
	type col struct{ TableName, ColumnName, DataType string }
	var cols []col
	if err := db.Raw(`SELECT table_name, column_name, data_type FROM information_schema.columns
		WHERE table_schema = current_schema()
		  AND table_name IN ('class_lesson_contents','livestream_sessions')
		  AND column_name IN ('open_date','due_date','scheduled_at','end_at','started_at','ended_at')`).Scan(&cols).Error; err != nil {
		t.Fatal(err)
	}
	got := make(map[string]string, len(cols))
	for _, c := range cols {
		got[c.TableName+"."+c.ColumnName] = c.DataType
	}
	return got
}

func TestInstantColumns_MigrateTaoTimestamptzVaGiuInstant(t *testing.T) {
	db := pgtest.IsolatedSchema(t, Migrate)

	got := instantColumnTypes(t, db)
	if len(got) != len(instantColumns) {
		t.Fatalf("thấy %d cột, muốn %d: %v", len(got), len(instantColumns), got)
	}
	for _, c := range instantColumns {
		if dt := got[c.table+"."+c.column]; dt != typeTimestamptz {
			t.Errorf("%s.%s = %q, muốn %q", c.table, c.column, dt, typeTimestamptz)
		}
	}

	// Khởi động lại: bước chuyển + AutoMigrate trên cột đã đúng kiểu không lỗi, kiểu giữ nguyên.
	if err := migrateInstantColumnsUp(db); err != nil {
		t.Fatalf("migrateInstantColumnsUp lần 2: %v", err)
	}

	// Round-trip qua model: 20:00 giờ VN phải đọc lại đúng là 13:00Z (cùng instant), không phải 20:00Z.
	vn := time.FixedZone("ICT", 7*3600)
	want := time.Date(2026, 10, 8, 20, 0, 0, 0, vn)
	class := model.Class{Name: "L2 instant", Status: "active"}
	if err := db.Create(&class).Error; err != nil {
		t.Fatal(err)
	}
	ls := model.LivestreamSession{Title: "L2", HostID: uuid.New(), ClassID: class.ID, RoomName: "l2-" + uuid.NewString(), ScheduledAt: &want}
	if err := db.Create(&ls).Error; err != nil {
		t.Fatal(err)
	}
	var back model.LivestreamSession
	if err := db.First(&back, "id = ?", ls.ID).Error; err != nil {
		t.Fatal(err)
	}
	if back.ScheduledAt == nil || !back.ScheduledAt.Equal(want) {
		t.Fatalf("scheduled_at đọc lại = %v, muốn cùng instant với %v", back.ScheduledAt, want)
	}
}

const legacyInstantTables = `
	CREATE TABLE class_lesson_contents (id int PRIMARY KEY, open_date timestamp, due_date timestamp, scheduled_at timestamp, end_at timestamp);
	CREATE TABLE livestream_sessions (id int PRIMARY KEY, scheduled_at timestamp, started_at timestamp, ended_at timestamp);
	INSERT INTO class_lesson_contents VALUES (1, NULL, '2026-10-30 23:59:00', '2026-10-08 20:00:00', '2026-10-08 21:30:00');
	INSERT INTO livestream_sessions VALUES (1, '2026-10-08 20:00:00', NULL, NULL);
`

// DB cũ: cột timestamp chứa giờ đồng hồ VN (đúng như seed). Sau chuyển phải là đúng instant (20:00 VN =
// 13:00Z) kể cả khi kết nối chạy migration ở múi giờ UTC; NULL giữ NULL; chạy lại không đổi gì.
func TestInstantColumns_ChuyenDuLieuCuTheoGioVN(t *testing.T) {
	db := pgtest.IsolatedSchema(t, func(*gorm.DB) error { return nil })
	if err := db.Exec(legacyInstantTables).Error; err != nil {
		t.Fatal(err)
	}

	err := db.Connection(func(conn *gorm.DB) error {
		if err := conn.Exec("SET TIME ZONE 'UTC'").Error; err != nil {
			return err
		}
		for run := 1; run <= 2; run++ {
			if err := migrateInstantColumnsUp(conn); err != nil {
				t.Fatalf("lần %d: %v", run, err)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	for k, dt := range instantColumnTypes(t, db) {
		if dt != typeTimestamptz {
			t.Errorf("%s = %q, muốn %q", k, dt, typeTimestamptz)
		}
	}
	utc := func(q string) string {
		var v *string
		if err := db.Raw(q).Scan(&v).Error; err != nil {
			t.Fatalf("%s: %v", q, err)
		}
		if v == nil {
			return "NULL"
		}
		return *v
	}
	want := map[string]string{
		`SELECT to_char(scheduled_at AT TIME ZONE 'UTC', 'YYYY-MM-DD HH24:MI') FROM class_lesson_contents WHERE id = 1`: "2026-10-08 13:00",
		`SELECT to_char(end_at AT TIME ZONE 'UTC', 'YYYY-MM-DD HH24:MI') FROM class_lesson_contents WHERE id = 1`:       "2026-10-08 14:30",
		`SELECT to_char(due_date AT TIME ZONE 'UTC', 'YYYY-MM-DD HH24:MI') FROM class_lesson_contents WHERE id = 1`:     "2026-10-30 16:59",
		`SELECT to_char(open_date AT TIME ZONE 'UTC', 'YYYY-MM-DD HH24:MI') FROM class_lesson_contents WHERE id = 1`:    "NULL",
		`SELECT to_char(scheduled_at AT TIME ZONE 'UTC', 'YYYY-MM-DD HH24:MI') FROM livestream_sessions WHERE id = 1`:   "2026-10-08 13:00",
		`SELECT to_char(started_at AT TIME ZONE 'UTC', 'YYYY-MM-DD HH24:MI') FROM livestream_sessions WHERE id = 1`:     "NULL",
	}
	for q, w := range want {
		if got := utc(q); got != w {
			t.Errorf("%s = %q, muốn %q", q, got, w)
		}
	}
}

// Down: timestamptz -> timestamp theo giờ VN, trả đúng giờ đồng hồ ban đầu; down rồi up không mất dữ liệu.
func TestInstantColumns_HoanTac(t *testing.T) {
	db := pgtest.IsolatedSchema(t, func(*gorm.DB) error { return nil })
	if err := db.Exec(legacyInstantTables).Error; err != nil {
		t.Fatal(err)
	}
	err := db.Connection(func(conn *gorm.DB) error {
		if err := conn.Exec("SET TIME ZONE 'UTC'").Error; err != nil {
			return err
		}
		if err := migrateInstantColumnsUp(conn); err != nil {
			return err
		}
		for run := 1; run <= 2; run++ { // idempotent
			if err := migrateInstantColumnsDown(conn); err != nil {
				t.Fatalf("down lần %d: %v", run, err)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	for k, dt := range instantColumnTypes(t, db) {
		if dt != typeTimestamp {
			t.Errorf("sau down %s = %q, muốn %q", k, dt, typeTimestamp)
		}
	}
	var wall string
	if err := db.Raw(`SELECT scheduled_at::text FROM class_lesson_contents WHERE id = 1`).Scan(&wall).Error; err != nil || wall != "2026-10-08 20:00:00" {
		t.Errorf("sau down scheduled_at = %q (%v), muốn giờ đồng hồ VN ban đầu 2026-10-08 20:00:00", wall, err)
	}
	if err := migrateInstantColumnsUp(db); err != nil {
		t.Fatal(err)
	}
	var inst string
	if err := db.Raw(`SELECT to_char(scheduled_at AT TIME ZONE 'UTC', 'YYYY-MM-DD HH24:MI') FROM class_lesson_contents WHERE id = 1`).Scan(&inst).Error; err != nil || inst != "2026-10-08 13:00" {
		t.Errorf("down rồi up: %q (%v), muốn 2026-10-08 13:00", inst, err)
	}
}
