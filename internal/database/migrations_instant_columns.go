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

// Tên kiểu theo pg_type (regtype), dùng cho cả điều kiện tìm cột lẫn câu ALTER.
const (
	regTimestamp   = "timestamp"
	regTimestamptz = "timestamptz"
)

// convertInstantColumns đổi các cột trong instantColumns đang có kiểu `from` sang `to`, diễn giải/xuất giá trị
// ở instantSourceZone (`x AT TIME ZONE zone` đổi hai chiều: timestamp -> timestamptz và ngược lại).
// Idempotent: chỉ ALTER cột thật sự đang kiểu `from`; bảng/cột chưa có (DB mới) hoặc đã đúng kiểu `to` thì
// không làm gì. Dùng MỘT truy vấn pg_catalog cho cả danh sách (không phải một truy vấn information_schema cho
// mỗi cột): bước này chạy ở mọi lần Migrate, kể cả mỗi fixture test, và information_schema chậm đáng kể.
func convertInstantColumns(db *gorm.DB, from, to string) error {
	tables := make([]string, 0, len(instantColumns))
	seen := map[string]bool{}
	for _, c := range instantColumns {
		if !seen[c.table] {
			seen[c.table] = true
			tables = append(tables, c.table)
		}
	}
	var found []struct{ Tbl, Col string }
	err := db.Raw(`
		SELECT c.relname AS tbl, a.attname AS col
		FROM pg_attribute a
		JOIN pg_class c ON c.oid = a.attrelid
		JOIN pg_namespace n ON n.oid = c.relnamespace
		WHERE n.nspname = current_schema() AND c.relkind = 'r' AND c.relname IN ?
		  AND a.attnum > 0 AND NOT a.attisdropped AND a.atttypid = ?::regtype`, tables, from).Scan(&found).Error
	if err != nil {
		return fmt.Errorf("tìm cột thời điểm kiểu %s: %w", from, err)
	}
	has := make(map[[2]string]bool, len(found))
	for _, f := range found {
		has[[2]string{f.Tbl, f.Col}] = true
	}
	for _, c := range instantColumns {
		if !has[[2]string{c.table, c.column}] {
			continue
		}
		stmt := fmt.Sprintf(`ALTER TABLE %q ALTER COLUMN %q TYPE %s USING (%q AT TIME ZONE '%s')`,
			c.table, c.column, to, c.column, instantSourceZone)
		if err := db.Exec(stmt).Error; err != nil {
			return fmt.Errorf("đổi %s.%s từ %s sang %s: %w", c.table, c.column, from, to, err)
		}
	}
	return nil
}

// migrateInstantColumnsUp: timestamp -> timestamptz, giữ NULL. Phải chạy TRƯỚC AutoMigrate: khi model đã khai
// timestamptz, AutoMigrate thấy cột khác kiểu và tự ALTER ... USING col::timestamptz, lấy múi giờ của PHIÊN kết
// nối (đúng chỉ khi DSN là Asia/Ho_Chi_Minh, lệch 7 giờ với kết nối UTC). Chạy trước để chuyển đổi tường minh.
func migrateInstantColumnsUp(db *gorm.DB) error {
	return convertInstantColumns(db, regTimestamp, regTimestamptz)
}

// migrateInstantColumnsDown: hoàn tác — timestamptz -> timestamp theo giờ VN. Chỉ dùng khi quay về bản code
// cũ; không được gọi lúc khởi động.
func migrateInstantColumnsDown(db *gorm.DB) error {
	return convertInstantColumns(db, regTimestamptz, regTimestamp)
}
