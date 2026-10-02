package database

// Cột thời điểm buổi học timestamp -> timestamptz (Postgres thật, schema tạm pgtest.IsolatedSchema, DROP khi xong).
//
// Trước bản sửa: model khai `type:timestamp` nên Migrate tạo `timestamp without time zone`, ghi một thời điểm
// có múi giờ rồi đọc lại ra instant lệch 7 giờ (assertInstantColumnsAfterMigrate, gọi từ
// TestTimeOfDayColumns_MigrateTaoKieuTime, ĐỎ).
// Bỏ migrateInstantColumnsUp hoặc bỏ `AT TIME ZONE` thì TestInstantColumns_ChuyenDuLieuCuTheoMuiGioNguon ĐỎ. Bỏ migrateInstantColumnsDown/đổi chiều thì TestInstantColumns_HoanTac ĐỎ.

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"study.com/v1/internal/model"
	"study.com/v1/internal/testutil/pgtest"
)

// Tên kiểu theo information_schema.columns.data_type.
const (
	typeTimestamp   = "timestamp without time zone"
	typeTimestamptz = "timestamp with time zone"
)

// instantColumnTypes: kiểu hiện tại của các cột trong instantColumns CÓ trong schema hiện tại (bảng chưa tạo thì
// không xuất hiện), khoá "bảng.cột".
func instantColumnTypes(t *testing.T, db *gorm.DB) map[string]string {
	t.Helper()
	tables := make([]string, 0, len(instantColumns))
	want := make(map[string]bool, len(instantColumns))
	for _, c := range instantColumns {
		if !want[c.table+"."+c.column] {
			tables = append(tables, c.table)
		}
		want[c.table+"."+c.column] = true
	}
	type col struct{ TableName, ColumnName, DataType string }
	var cols []col
	if err := db.Raw(`SELECT table_name, column_name, data_type FROM information_schema.columns
		WHERE table_schema = current_schema() AND table_name IN ?`, tables).Scan(&cols).Error; err != nil {
		t.Fatal(err)
	}
	got := make(map[string]string, len(cols))
	for _, c := range cols {
		if k := c.TableName + "." + c.ColumnName; want[k] {
			got[k] = c.DataType
		}
	}
	return got
}

// assertInstantColumnsAfterMigrate kiểm một schema đã Migrate đầy đủ: cột thời điểm là timestamptz và đọc lại
// đúng instant. Được gọi từ TestTimeOfDayColumns_MigrateTaoKieuTime để dùng chung MỘT lần Migrate đầy đủ (mỗi
// lần vài chục giây trên CI; package này và package service đã sát giới hạn thời gian của `go test`).
func assertInstantColumnsAfterMigrate(t *testing.T, db *gorm.DB) {
	t.Helper()
	got := instantColumnTypes(t, db)
	if len(got) != len(instantColumns) {
		t.Fatalf("thấy %d cột thời điểm, muốn %d: %v", len(got), len(instantColumns), got)
	}
	for _, c := range instantColumns {
		if dt := got[c.table+"."+c.column]; dt != typeTimestamptz {
			t.Errorf("%s.%s = %q, muốn %q", c.table, c.column, dt, typeTimestamptz)
		}
	}

	// Khởi động lại: bước chuyển trên cột đã đúng kiểu không lỗi, kiểu giữ nguyên.
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

// DB cũ: cột timestamp chứa giờ đồng hồ UTC (API nhận RFC3339, web gửi toISOString). Sau chuyển phải
// giữ đúng instant (20:00 đồng hồ UTC = 20:00Z, KHÔNG phải 13:00Z) kể cả khi kết nối chạy migration ở múi giờ UTC; NULL giữ NULL; chạy lại không đổi gì.
func TestInstantColumns_ChuyenDuLieuCuTheoMuiGioNguon(t *testing.T) {
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
		`SELECT to_char(scheduled_at AT TIME ZONE 'UTC', 'YYYY-MM-DD HH24:MI') FROM class_lesson_contents WHERE id = 1`: "2026-10-08 20:00",
		`SELECT to_char(end_at AT TIME ZONE 'UTC', 'YYYY-MM-DD HH24:MI') FROM class_lesson_contents WHERE id = 1`:       "2026-10-08 21:30",
		`SELECT to_char(due_date AT TIME ZONE 'UTC', 'YYYY-MM-DD HH24:MI') FROM class_lesson_contents WHERE id = 1`:     "2026-10-30 23:59",
		`SELECT to_char(open_date AT TIME ZONE 'UTC', 'YYYY-MM-DD HH24:MI') FROM class_lesson_contents WHERE id = 1`:    "NULL",
		`SELECT to_char(scheduled_at AT TIME ZONE 'UTC', 'YYYY-MM-DD HH24:MI') FROM livestream_sessions WHERE id = 1`:   "2026-10-08 20:00",
		`SELECT to_char(started_at AT TIME ZONE 'UTC', 'YYYY-MM-DD HH24:MI') FROM livestream_sessions WHERE id = 1`:     "NULL",
	}
	for q, w := range want {
		if got := utc(q); got != w {
			t.Errorf("%s = %q, muốn %q", q, got, w)
		}
	}
}

// Down: timestamptz -> timestamp theo múi giờ nguồn của cột, trả đúng giờ đồng hồ ban đầu; down rồi up không mất dữ liệu.
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
		t.Errorf("sau down scheduled_at = %q (%v), muốn giờ đồng hồ ban đầu 2026-10-08 20:00:00", wall, err)
	}
	if err := migrateInstantColumnsUp(db); err != nil {
		t.Fatal(err)
	}
	var inst string
	if err := db.Raw(`SELECT to_char(scheduled_at AT TIME ZONE 'UTC', 'YYYY-MM-DD HH24:MI') FROM class_lesson_contents WHERE id = 1`).Scan(&inst).Error; err != nil || inst != "2026-10-08 20:00" {
		t.Errorf("down rồi up: %q (%v), muốn 2026-10-08 20:00", inst, err)
	}
}

// assertClassOrganizationColumn: classes.organization_id cho phép NULL, có index, FK tới organizations
// (tổ chức không tồn tại bị từ chối; xoá tổ chức thì lớp về lớp cá nhân chứ không bị xoá), và lớp tạo không kèm
// tổ chức giữ NULL (không backfill/suy luận).
func assertClassOrganizationColumn(t *testing.T, db *gorm.DB) {
	t.Helper()
	// Chỉ truy vấn catalog của đúng bảng classes (to_regclass theo search_path của schema tạm): information_schema
	// và pg_indexes quét MỌI schema, mà test của package khác chạy song song đang tạo/xoá schema của chúng
	// ("could not open relation with OID" trên CI).
	var notNull bool
	if err := db.Raw(`SELECT attnotnull FROM pg_attribute WHERE attrelid = to_regclass('classes') AND attname = 'organization_id'`).
		Scan(&notNull).Error; err != nil {
		t.Fatal(err)
	}
	if notNull {
		t.Fatal("classes.organization_id phải cho phép NULL")
	}
	var idx int64
	if err := db.Raw(`SELECT count(*) FROM pg_index i JOIN pg_attribute a ON a.attrelid = i.indrelid AND a.attnum = ANY(i.indkey)
		WHERE i.indrelid = to_regclass('classes') AND a.attname = 'organization_id'`).Scan(&idx).Error; err != nil || idx == 0 {
		t.Fatalf("classes.organization_id thiếu index (n=%d err=%v)", idx, err)
	}
	org := model.Organization{Name: "L2 org " + uuid.NewString()[:6]}
	if err := db.Create(&org).Error; err != nil {
		t.Fatal(err)
	}
	personal := model.Class{Name: "L2 personal", Status: "active"}
	inOrg := model.Class{Name: "L2 in org", Status: "active", OrganizationID: &org.ID}
	if err := db.Create(&personal).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&inOrg).Error; err != nil {
		t.Fatal(err)
	}
	var back model.Class
	if err := db.First(&back, "id = ?", personal.ID).Error; err != nil || back.OrganizationID != nil {
		t.Fatalf("lớp cá nhân phải giữ organization_id NULL: %v err=%v", back.OrganizationID, err)
	}
	ghost := uuid.New()
	if err := db.Create(&model.Class{Name: "L2 ghost org", Status: "active", OrganizationID: &ghost}).Error; err == nil {
		t.Fatal("organization_id trỏ tới tổ chức không tồn tại phải bị FK từ chối")
	}
	if err := db.Unscoped().Delete(&org).Error; err != nil {
		t.Fatal(err)
	}
	var after model.Class
	if err := db.First(&after, "id = ?", inOrg.ID).Error; err != nil || after.OrganizationID != nil {
		t.Fatalf("xoá tổ chức: lớp phải còn và về NULL, thấy %v err=%v", after.OrganizationID, err)
	}
}
