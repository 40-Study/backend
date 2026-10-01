package database

import (
	"fmt"

	"gorm.io/gorm"
	"study.com/v1/internal/model"
)

// timeOfDaySourceZone là múi giờ dữ liệu cũ được ghi: backend và seed ghi giờ học theo giờ Việt Nam
// (DSN `TimeZone=Asia/Ho_Chi_Minh`, seed dùng giờ máy chủ +07). Đã kiểm trên DB dev 01/10/2026:
// 6 lịch + 36 buổi đều lưu `... 19:00:00+07`, `... 19:30:00+07`, `... 08:30:00+07`.
const timeOfDaySourceZone = "Asia/Ho_Chi_Minh"

// timeOfDayColumns: các cột giờ-trong-ngày từng bị AutoMigrate tạo thành TIMESTAMPTZ (xem
// model.TimeOfDayColumnType).
var timeOfDayColumns = []struct{ table, column string }{
	{"class_schedules", "start_time"},
	{"class_schedules", "end_time"},
	{"class_sessions", "start_time"},
	{"class_sessions", "end_time"},
	{"session_attendances", "expected_time"},
	{"attendances", "expected_time"},
}

// buildTimeOfDayColumnSQL chuyển 1 cột TIMESTAMPTZ sang `time without time zone`, giữ giờ địa phương
// (19:00+07 -> 19:00, không phải 12:00 UTC). Idempotent: chỉ ALTER khi cột đang là TIMESTAMPTZ; bảng
// hoặc cột chưa có (DB mới) thì không làm gì. Kiểu khác (vd text) không đụng tới: AT TIME ZONE trên
// kiểu đó mang nghĩa khác, để AutoMigrate xử lý.
func buildTimeOfDayColumnSQL(table, column string) string {
	return fmt.Sprintf(`
		DO $$
		BEGIN
			IF EXISTS (SELECT 1 FROM information_schema.columns
			           WHERE table_schema = current_schema() AND table_name = '%[1]s'
			             AND column_name = '%[2]s' AND data_type = 'timestamp with time zone') THEN
				ALTER TABLE %[1]s ALTER COLUMN %[2]s TYPE %[3]s
					USING (%[2]s AT TIME ZONE '%[4]s')::%[3]s;
			END IF;
		END $$;
	`, table, column, model.TimeOfDayColumnType, timeOfDaySourceZone)
}

// migrateTimeOfDayColumns phải chạy TRƯỚC AutoMigrate, không đặt trong RunPostMigrations: khi model
// đã khai `time without time zone`, AutoMigrate thấy cột TIMESTAMPTZ khác kiểu và tự chạy
// `ALTER ... TYPE time USING col::time` (driver/postgres migrator.go modifyColumn). Phép ép đó lấy
// giờ theo TimeZone của phiên kết nối — đúng chỉ khi DSN đặt Asia/Ho_Chi_Minh, sai lệch 7 giờ với kết
// nối UTC. Chạy trước để chuyển đổi tường minh theo timeOfDaySourceZone; sau đó AutoMigrate thấy
// cùng kiểu và bỏ qua.
func migrateTimeOfDayColumns(db *gorm.DB) error {
	for _, c := range timeOfDayColumns {
		if err := db.Exec(buildTimeOfDayColumnSQL(c.table, c.column)).Error; err != nil {
			return fmt.Errorf("chuyển %s.%s sang %s: %w", c.table, c.column, model.TimeOfDayColumnType, err)
		}
	}
	return nil
}
