package database

// Cột giờ trong ngày của lịch học (Postgres thật, schema tạm pgtest.IsolatedSchema, DROP khi xong).
//
// Trước bản sửa: model khai `type:time` nên AutoMigrate tạo TIMESTAMPTZ (xem model.TimeOfDayColumnType)
// — TestTimeOfDayColumns_MigrateTaoKieuTime ĐỎ. Bỏ migrateTimeOfDayColumns hoặc bỏ `AT TIME ZONE` thì
// TestTimeOfDayColumns_ChuyenCotCuGiuGioDiaPhuong ĐỎ (phiên UTC cho 12:00 thay vì 19:00).

import (
	"testing"

	"gorm.io/gorm"
	"study.com/v1/internal/model"
	"study.com/v1/internal/testutil/pgtest"
)

func timeOfDayColumnTypes(t *testing.T, db *gorm.DB) map[string]string {
	t.Helper()
	type col struct{ TableName, ColumnName, DataType string }
	var cols []col
	if err := db.Raw(`SELECT table_name, column_name, data_type FROM information_schema.columns
		WHERE table_schema = current_schema() AND column_name IN ('start_time','end_time','expected_time')
		  AND table_name IN ('class_schedules','class_sessions','session_attendances','attendances')`).
		Scan(&cols).Error; err != nil {
		t.Fatal(err)
	}
	got := make(map[string]string, len(cols))
	for _, c := range cols {
		got[c.TableName+"."+c.ColumnName] = c.DataType
	}
	return got
}

func TestTimeOfDayColumns_MigrateTaoKieuTime(t *testing.T) {
	db := pgtest.IsolatedSchema(t, Migrate)
	// Khởi động lại (bước giờ + AutoMigrate trên cột đã là time): không lỗi, không ALTER, kiểu giữ
	// nguyên. Chỉ AutoMigrate 4 model có cột giờ: Migrate() đầy đủ lần 2 truy vấn ColumnTypes cho mọi
	// bảng, quá 10 phút trên DB dev nhiều schema tạm.
	if err := migrateTimeOfDayColumns(db); err != nil {
		t.Fatalf("migrateTimeOfDayColumns lần 2: %v", err)
	}
	if err := db.AutoMigrate(&model.ClassSchedule{}, &model.ClassSession{}, &model.SessionAttendance{}, &model.Attendance{}); err != nil {
		t.Fatalf("AutoMigrate lần 2: %v", err)
	}
	got := timeOfDayColumnTypes(t, db)
	if len(got) != len(timeOfDayColumns) {
		t.Fatalf("thấy %d cột giờ, muốn %d: %v", len(got), len(timeOfDayColumns), got)
	}
	for _, c := range timeOfDayColumns {
		if dt := got[c.table+"."+c.column]; dt != model.TimeOfDayColumnType {
			t.Errorf("%s.%s = %q, muốn %q", c.table, c.column, dt, model.TimeOfDayColumnType)
		}
	}
}

// DB cũ: cột TIMESTAMPTZ chứa giờ +07 (đúng như DB dev 01/10/2026). Sau chuyển đổi phải còn đúng giờ
// địa phương, kể cả khi kết nối chạy migration ở múi giờ UTC; chạy lại không đổi gì.
func TestTimeOfDayColumns_ChuyenCotCuGiuGioDiaPhuong(t *testing.T) {
	db := pgtest.IsolatedSchema(t, func(*gorm.DB) error { return nil })
	legacy := `
		CREATE TABLE class_schedules (id int PRIMARY KEY, start_time timestamptz NOT NULL, end_time timestamptz NOT NULL);
		CREATE TABLE class_sessions (id int PRIMARY KEY, start_time timestamptz NOT NULL, end_time timestamptz NOT NULL);
		CREATE TABLE session_attendances (id int PRIMARY KEY, expected_time timestamptz);
		INSERT INTO class_schedules VALUES (1, '2026-09-02 19:00:00+07', '2026-09-02 21:00:00+07'),
		                                   (2, '2026-08-26 08:30:00+07', '2026-08-26 11:30:00+07');
		INSERT INTO class_sessions VALUES (1, '2026-10-13 19:30:00+07', '2026-10-13 21:30:00+07');
		INSERT INTO session_attendances VALUES (1, '2026-09-02 19:00:00+07'), (2, NULL);
	`
	if err := db.Exec(legacy).Error; err != nil {
		t.Fatal(err)
	}

	err := db.Connection(func(conn *gorm.DB) error {
		if err := conn.Exec("SET TIME ZONE 'UTC'").Error; err != nil {
			return err
		}
		for run := 1; run <= 2; run++ {
			if err := migrateTimeOfDayColumns(conn); err != nil {
				t.Fatalf("lần %d: %v", run, err)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	got := timeOfDayColumnTypes(t, db)
	for _, k := range []string{"class_schedules.start_time", "class_schedules.end_time", "class_sessions.start_time",
		"class_sessions.end_time", "session_attendances.expected_time"} {
		if got[k] != model.TimeOfDayColumnType {
			t.Errorf("%s = %q, muốn %q", k, got[k], model.TimeOfDayColumnType)
		}
	}

	want := map[string]string{
		"SELECT start_time::text FROM class_schedules WHERE id = 1":        "19:00:00",
		"SELECT end_time::text FROM class_schedules WHERE id = 1":          "21:00:00",
		"SELECT start_time::text FROM class_schedules WHERE id = 2":        "08:30:00",
		"SELECT start_time::text FROM class_sessions WHERE id = 1":         "19:30:00",
		"SELECT expected_time::text FROM session_attendances WHERE id = 1": "19:00:00",
	}
	for q, w := range want {
		var v string
		if err := db.Raw(q).Scan(&v).Error; err != nil || v != w {
			t.Errorf("%s = %q (%v), muốn %q", q, v, err, w)
		}
	}
	var nulls int64
	db.Raw("SELECT count(*) FROM session_attendances WHERE expected_time IS NULL").Scan(&nulls)
	if nulls != 1 {
		t.Errorf("expected_time NULL phải giữ NULL, thấy %d dòng NULL", nulls)
	}

	// Đọc qua model: Scan trả dạng "HH:MM".
	var sch model.ClassSchedule
	if err := db.Raw("SELECT start_time, end_time FROM class_schedules WHERE id = 1").Scan(&sch).Error; err != nil ||
		sch.StartTime != "19:00" || sch.EndTime != "21:00" {
		t.Errorf("đọc model: %q-%q (%v), muốn 19:00-21:00", sch.StartTime, sch.EndTime, err)
	}
}

// Backend lane khác còn model cũ (`type:time` -> TIMESTAMPTZ) khởi động trên DB dùng chung đã chuyển:
// AutoMigrate của nó KHÔNG được đổi cột về TIMESTAMPTZ (GORM so tiền tố "timestamptz" với "time").
func TestTimeOfDayColumns_ModelCuKhongDoiNguocKieuCot(t *testing.T) {
	db := pgtest.IsolatedSchema(t, func(*gorm.DB) error { return nil })
	if err := db.Exec(`CREATE TABLE class_schedules (id bigserial PRIMARY KEY, start_time time NOT NULL, end_time time NOT NULL);
		INSERT INTO class_schedules (start_time, end_time) VALUES ('19:00', '21:00')`).Error; err != nil {
		t.Fatal(err)
	}
	type oldClassSchedule struct {
		ID        uint
		StartTime string `gorm:"type:time;not null"`
		EndTime   string `gorm:"type:time;not null"`
	}
	if err := db.Table("class_schedules").AutoMigrate(&oldClassSchedule{}); err != nil {
		t.Fatalf("AutoMigrate model cũ: %v", err)
	}
	if got := timeOfDayColumnTypes(t, db)["class_schedules.start_time"]; got != model.TimeOfDayColumnType {
		t.Fatalf("model cũ đổi cột thành %q", got)
	}
}
