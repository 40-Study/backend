package database

import (
	"fmt"
	"time"

	"gorm.io/gorm"
)

// l2UTCInstantFixName: tên bản ghi đánh dấu trong data_migrations. Có bản ghi này nghĩa là bước sửa một lần đã chạy.
const l2UTCInstantFixName = "l7_l2_utc_instant_columns_plus_7h"

// l2UTCInstantFixCutoff: mốc merge của L2 (#108, 2026-10-02T11:23:47+07:00). Chỉ dòng tạo TRƯỚC mốc này mới có thể
// mang giá trị bị L2 đổi sai; dòng tạo sau đó được ghi khi cột đã là timestamptz nên đúng sẵn.
var l2UTCInstantFixCutoff = time.Date(2026, 10, 2, 4, 23, 47, 0, time.UTC)

// l2WronglyConvertedColumns: các cột L2 đã đổi sang timestamptz theo giờ VN trong khi đường ghi thật ở production
// là UTC (API nhận RFC3339, web gửi toISOString(), time.Parse giữ UTC nên pgx ghi giờ đồng hồ UTC vào cột timestamp).
// Giá trị đã đổi sai nằm sớm hơn đúng 7 giờ.
var l2WronglyConvertedColumns = []timestampColumn{
	{"class_lesson_contents", "open_date", zoneUTC},
	{"class_lesson_contents", "due_date", zoneUTC},
	{"class_lesson_contents", "scheduled_at", zoneUTC},
	{"class_lesson_contents", "end_at", zoneUTC},
	{"livestream_sessions", "scheduled_at", zoneUTC},
}

// fixL2WronglyConvertedInstants cộng lại 7 giờ cho dữ liệu L2 đã đổi sai. MỘT LẦN DUY NHẤT trên mỗi DB:
//   - Chạy trong giao dịch giữ cùng khoá advisory với convertTimestampColumns; kiểm tra bản ghi đánh dấu SAU khi
//     giữ khoá nên hai tiến trình song song không thể cùng cộng.
//   - Chỉ động vào cột ĐANG là timestamptz (L2 đã đổi) và dòng tạo trước mốc merge L2. Cột còn là timestamp sẽ được
//     convertTimestampColumns đổi ĐÚNG theo UTC ngay sau đó nên không cần sửa; DB mới (chưa có bảng) không có gì để sửa.
//   - Ghi bản ghi đánh dấu trong cùng giao dịch: lỗi giữa chừng thì rollback cả hai, chạy lại được.
//
// Giới hạn đã biết: dòng tạo sau mốc merge nhưng trước khi DB đó chạy L2 vẫn có thể lệch 7 giờ (cửa sổ vài giờ);
// theo quyết định của chủ dự án không suy luận từng dòng.
func fixL2WronglyConvertedInstants(db *gorm.DB) error {
	return db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec(fmt.Sprintf("SET LOCAL TIME ZONE '%s'", zoneVN)).Error; err != nil {
			return fmt.Errorf("đặt múi giờ phiên cho bước sửa L2: %w", err)
		}
		if err := tx.Exec("SELECT pg_advisory_xact_lock(?)", timestampConversionLockKey).Error; err != nil {
			return fmt.Errorf("lấy khoá bước sửa L2: %w", err)
		}
		if err := tx.Exec(`CREATE TABLE IF NOT EXISTS data_migrations (
			name text PRIMARY KEY,
			applied_at timestamptz NOT NULL DEFAULT now())`).Error; err != nil {
			return fmt.Errorf("tạo bảng data_migrations: %w", err)
		}
		var done int64
		if err := tx.Raw("SELECT count(*) FROM data_migrations WHERE name = ?", l2UTCInstantFixName).Scan(&done).Error; err != nil {
			return fmt.Errorf("đọc data_migrations: %w", err)
		}
		if done > 0 {
			return nil
		}
		var found []struct{ Tbl, Col string }
		tables := []string{"class_lesson_contents", "livestream_sessions"}
		if err := tx.Raw(`
			SELECT c.relname AS tbl, a.attname AS col
			FROM pg_attribute a
			JOIN pg_class c ON c.oid = a.attrelid
			JOIN pg_namespace n ON n.oid = c.relnamespace
			WHERE n.nspname = current_schema() AND c.relkind = 'r' AND c.relname IN ?
			  AND a.attnum > 0 AND NOT a.attisdropped AND a.atttypid = ?::regtype`, tables, regTimestamptz).Scan(&found).Error; err != nil {
			return fmt.Errorf("tìm cột timestamptz cần sửa: %w", err)
		}
		has := make(map[[2]string]bool, len(found))
		for _, f := range found {
			has[[2]string{f.Tbl, f.Col}] = true
		}
		for _, c := range l2WronglyConvertedColumns {
			if !has[[2]string{c.table, c.column}] {
				continue
			}
			stmt := fmt.Sprintf(`UPDATE %q SET %q = %q + interval '7 hours' WHERE %q IS NOT NULL AND created_at < ?`,
				c.table, c.column, c.column, c.column)
			if err := tx.Exec(stmt, l2UTCInstantFixCutoff).Error; err != nil {
				return fmt.Errorf("sửa %s.%s: %w", c.table, c.column, err)
			}
		}
		if err := tx.Exec("INSERT INTO data_migrations (name) VALUES (?)", l2UTCInstantFixName).Error; err != nil {
			return fmt.Errorf("ghi dấu bước sửa L2: %w", err)
		}
		return nil
	})
}
