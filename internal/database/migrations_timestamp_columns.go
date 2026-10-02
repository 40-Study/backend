package database

import (
	"fmt"

	"gorm.io/gorm"
)

// timestampColumn: một cột thời điểm cần đổi kiểu timestamp <-> timestamptz.
type timestampColumn struct {
	table, column string
	// zone: múi giờ coi là của giá trị CŨ khi cột chưa có múi giờ. Mỗi cột tự khai vì cùng một kiểu `timestamp`
	// mà nơi ghi khác nhau ghi giờ đồng hồ khác nhau: time.Now()/CURRENT_TIMESTAMP của phiên Asia/Ho_Chi_Minh ghi
	// giờ VN, còn chuỗi RFC3339 "...Z" do web gửi (toISOString) ghi giờ UTC. Chọn sai múi thì giá trị lệch 7 giờ.
	zone string
}

const (
	zoneVN  = "Asia/Ho_Chi_Minh"
	zoneUTC = "UTC"
)

// timestampConversionLockKey: khoá advisory (toàn DB) tuần tự hoá mọi lần đổi kiểu cột thời điểm. Hằng cố định,
// không suy từ tên schema: hai tiến trình migrate cùng DB phải tranh CÙNG một khoá.
const timestampConversionLockKey int64 = 4020261002

// convertTimestampColumns đổi các cột trong cols đang có kiểu `from` sang `to`, diễn giải/xuất giá trị ở múi giờ
// của từng cột (`x AT TIME ZONE zone` đổi hai chiều: timestamp -> timestamptz và ngược lại).
// Idempotent: chỉ ALTER cột thật sự đang kiểu `from`; bảng/cột chưa có (DB mới) hoặc đã đúng kiểu `to` thì không
// làm gì. Dùng MỘT truy vấn pg_catalog cho cả danh sách (không phải một truy vấn information_schema cho mỗi cột):
// bước này chạy ở mọi lần Migrate, kể cả mỗi fixture test, và information_schema chậm đáng kể.
//
// An toàn khi nhiều tiến trình migrate cùng lúc: toàn bộ (dò kiểu cột + ALTER) chạy trong MỘT giao dịch giữ
// pg_advisory_xact_lock, và việc dò kiểu cột nằm SAU khi giữ khoá. Trước đây dò xong mới ALTER không có khoá: tiến
// trình thứ hai đã thấy cột là `timestamp`, chờ khoá bảng của tiến trình thứ nhất rồi ALTER tiếp trên cột đã là
// `timestamptz`, tức là `ts AT TIME ZONE zone` (ra timestamp) bị ép ngược về timestamptz theo múi giờ của PHIÊN:
// lệch 7 giờ nếu phiên là UTC. Giờ tiến trình đến sau thấy cột đã đổi và không làm gì.
//
// `SET LOCAL TIME ZONE` ghim múi giờ phiên trong giao dịch này để mọi phép ép kiểu ngầm (nếu có) không phụ thuộc
// múi giờ kết nối của nơi gọi (DSN, pgtest, công cụ khác); hết giao dịch múi giờ phiên trở lại như cũ.
func convertTimestampColumns(db *gorm.DB, cols []timestampColumn, from, to string) error {
	tables := make([]string, 0, len(cols))
	seen := map[string]bool{}
	for _, c := range cols {
		if !seen[c.table] {
			seen[c.table] = true
			tables = append(tables, c.table)
		}
	}
	return db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec(fmt.Sprintf("SET LOCAL TIME ZONE '%s'", zoneVN)).Error; err != nil {
			return fmt.Errorf("đặt múi giờ phiên cho migration: %w", err)
		}
		if err := tx.Exec("SELECT pg_advisory_xact_lock(?)", timestampConversionLockKey).Error; err != nil {
			return fmt.Errorf("lấy khoá migration cột thời điểm: %w", err)
		}
		var found []struct{ Tbl, Col string }
		err := tx.Raw(`
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
		for _, c := range cols {
			if !has[[2]string{c.table, c.column}] {
				continue
			}
			stmt := fmt.Sprintf(`ALTER TABLE %q ALTER COLUMN %q TYPE %s USING (%q AT TIME ZONE '%s')`,
				c.table, c.column, to, c.column, c.zone)
			if err := tx.Exec(stmt).Error; err != nil {
				return fmt.Errorf("đổi %s.%s từ %s sang %s: %w", c.table, c.column, from, to, err)
			}
		}
		return nil
	})
}
