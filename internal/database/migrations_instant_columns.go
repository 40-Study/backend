package database

import (
	"fmt"

	"gorm.io/gorm"
)

// instantColumns: các cột THỜI ĐIỂM của buổi học (lịch live, nội dung học theo lớp) từng khai `type:timestamp`
// (timestamp without time zone). Kiểu đó không giữ múi giờ nên cùng một giá trị đọc ra sai 7 giờ tuỳ nơi ghi:
// pgx ghi giờ đồng hồ theo múi của time.Time truyền vào (seed, CURRENT_TIMESTAMP của phiên Asia/Ho_Chi_Minh
// ghi giờ VN), nhưng đọc lại luôn gắn nhãn UTC ("20:00" của giờ VN thành 20:00Z = 03:00 sáng hôm sau giờ VN).
// Hậu quả thật: mốc "vào sớm" của livestream so với time.Now() và so trùng lịch live lệch 7 giờ.
//
// Chuyển sang timestamptz: lưu instant tuyệt đối, đọc ra đúng ở mọi múi giờ phiên.
//
// KHÔNG gồm start_time/end_time của class_schedules, class_sessions: đó là giờ-trong-ngày "HH:MM" của lịch
// lặp lại (model.TimeOfDay, #104) đi cùng cột `date`, đã đúng kiểu `time`. Ranh giới nửa đêm giờ VN của điểm
// danh dựa vào cột `date` (requireCheckInOpen), không dựa vào các cột này nên không đổi.
var instantColumns = []struct{ table, column string }{
	{"class_lesson_contents", "open_date"},
	{"class_lesson_contents", "due_date"},
	{"class_lesson_contents", "scheduled_at"},
	{"class_lesson_contents", "end_at"},
	{"livestream_sessions", "scheduled_at"},
	{"livestream_sessions", "started_at"},
	{"livestream_sessions", "ended_at"},
}

// instantSourceZone: múi giờ coi là của dữ liệu cũ khi chưa có múi giờ. Seed, `CURRENT_TIMESTAMP` của phiên
// (DSN TimeZone=Asia/Ho_Chi_Minh) và các nơi ghi time.Now() đều là giờ VN. Giả định này là quyết định của chủ
// dự án (02/10/2026); dòng do web gửi chuỗi UTC ("...Z") đã ghi giờ UTC nên các dòng đó (nếu có) lệch 7 giờ
// MỘT LẦN khi chuyển.
const instantSourceZone = "Asia/Ho_Chi_Minh"

const (
	typeTimestamp   = "timestamp without time zone"
	typeTimestamptz = "timestamp with time zone"
)

// buildInstantColumnSQL đổi kiểu một cột từ `from` sang `to`, diễn giải/xuất giá trị ở instantSourceZone
// (`x AT TIME ZONE zone` đổi hai chiều: timestamp -> timestamptz và ngược lại). Idempotent: chỉ ALTER khi
// cột đang đúng kiểu `from`; bảng/cột chưa có (DB mới) hoặc đã đúng kiểu `to` thì không làm gì.
func buildInstantColumnSQL(table, column, from, to string) string {
	return fmt.Sprintf(`
		DO $$
		BEGIN
			IF EXISTS (SELECT 1 FROM information_schema.columns
			           WHERE table_schema = current_schema() AND table_name = '%[1]s'
			             AND column_name = '%[2]s' AND data_type = '%[3]s') THEN
				ALTER TABLE %[1]s ALTER COLUMN %[2]s TYPE %[4]s
					USING (%[2]s AT TIME ZONE '%[5]s');
			END IF;
		END $$;
	`, table, column, from, to, instantSourceZone)
}

// migrateInstantColumnsUp: timestamp -> timestamptz, giữ NULL. Phải chạy TRƯỚC AutoMigrate: khi model đã khai
// timestamptz, AutoMigrate thấy cột khác kiểu và tự ALTER ... USING col::timestamptz, lấy múi giờ của PHIÊN kết
// nối (đúng chỉ khi DSN là Asia/Ho_Chi_Minh, lệch 7 giờ với kết nối UTC). Chạy trước để chuyển đổi tường minh.
func migrateInstantColumnsUp(db *gorm.DB) error {
	for _, c := range instantColumns {
		if err := db.Exec(buildInstantColumnSQL(c.table, c.column, typeTimestamp, "timestamptz")).Error; err != nil {
			return fmt.Errorf("chuyển %s.%s sang timestamptz: %w", c.table, c.column, err)
		}
	}
	return nil
}

// migrateInstantColumnsDown: hoàn tác — timestamptz -> timestamp theo giờ VN. Chỉ dùng khi quay về bản code
// cũ; không được gọi lúc khởi động.
func migrateInstantColumnsDown(db *gorm.DB) error {
	for _, c := range instantColumns {
		if err := db.Exec(buildInstantColumnSQL(c.table, c.column, typeTimestamptz, "timestamp")).Error; err != nil {
			return fmt.Errorf("hoàn tác %s.%s về timestamp: %w", c.table, c.column, err)
		}
	}
	return nil
}
