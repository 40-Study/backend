package database

import (
	"gorm.io/gorm"
)

// instantColumns: các cột THỜI ĐIỂM từng khai `type:timestamp` (timestamp without time zone). Kiểu đó không giữ
// múi giờ nên cùng một giá trị đọc ra sai 7 giờ tuỳ nơi ghi: pgx ghi giờ đồng hồ theo múi của time.Time truyền
// vào (time.Now() và CURRENT_TIMESTAMP của phiên Asia/Ho_Chi_Minh ghi giờ VN; chuỗi RFC3339 "...Z" của web ghi
// giờ UTC), nhưng đọc lại luôn gắn nhãn UTC ("20:00" của giờ VN thành 20:00Z = 03:00 sáng hôm sau giờ VN).
// Hậu quả thật: mốc "vào sớm" của livestream so với time.Now() và so trùng lịch live lệch 7 giờ.
//
// Chuyển sang timestamptz: lưu instant tuyệt đối, đọc ra đúng ở mọi múi giờ phiên.
//
// zone từng cột = múi giờ của nơi GHI giá trị cũ (xem timestampColumn.zone). Nguyên tắc (chủ dự án 02/10/2026): múi giờ nguồn do ĐƯỜNG GHI THẬT ở production
// quyết định, không theo seed; dòng seed lệch là dữ liệu dev. Bảng quyết định đủ 20 cột (đợt L2 + L7) nằm trong mô tả PR của L7.
//
// KHÔNG gồm start_time/end_time của class_schedules, class_sessions: đó là giờ-trong-ngày "HH:MM" của lịch
// lặp lại (model.TimeOfDay, #104) đi cùng cột `date`, đã đúng kiểu `time`. Ranh giới nửa đêm giờ VN của điểm
// danh dựa vào cột `date` (requireCheckInOpen), không dựa vào các cột này nên không đổi.
var instantColumns = []timestampColumn{
	// L2: thời điểm buổi học. Đường ghi thật là API: DTO nhận chuỗi RFC3339, web gửi toISOString() ("...Z"),
	// time.Parse giữ UTC nên pgx ghi giờ đồng hồ UTC vào cột timestamp: nguồn là UTC. (L2 từng đổi 5 cột này
	// theo giờ VN, sai; dữ liệu đã đổi sai được sửa một lần bởi fixL2WronglyConvertedInstants.)
	{"class_lesson_contents", "open_date", zoneUTC},
	{"class_lesson_contents", "due_date", zoneUTC},
	{"class_lesson_contents", "scheduled_at", zoneUTC},
	{"class_lesson_contents", "end_at", zoneUTC},
	{"livestream_sessions", "scheduled_at", zoneUTC},
	// started_at/ended_at ghi bằng CURRENT_TIMESTAMP của phiên Asia/Ho_Chi_Minh (DSN): giờ VN.
	{"livestream_sessions", "started_at", zoneVN},
	{"livestream_sessions", "ended_at", zoneVN},

	// L7. Bài tập: start_time/end_time do web gửi bằng toISOString(): UTC. published_at ghi bằng
	// CURRENT_TIMESTAMP của phiên VN nên là giờ VN.
	{"assignments", "published_at", zoneVN},
	{"assignments", "start_time", zoneUTC},
	{"assignments", "end_time", zoneUTC},
	// Người tham gia livestream: joined_at mặc định CURRENT_TIMESTAMP, left_at/kicked_at ghi bằng
	// gorm.Expr("CURRENT_TIMESTAMP"): giờ VN.
	{"participants", "joined_at", zoneVN},
	{"participants", "left_at", zoneVN},
	{"participants", "kicked_at", zoneVN},
	// expires_at/saved_at ghi bằng time.Now(): giờ đồng hồ theo múi giờ TIẾN TRÌNH. Container production là
	// `FROM scratch`, không đặt TZ (Dockerfile.prod, repo infra không có TZ nào) nên time.Local = UTC: nguồn là UTC.
	{"idempotency_keys", "expires_at", zoneUTC},
	// Voucher: web admin gửi start_date/end_date bằng toISOString() (voucher-form-model.ts): UTC.
	{"vouchers", "start_date", zoneUTC},
	{"vouchers", "end_date", zoneUTC},
	{"user_vouchers", "saved_at", zoneUTC},
	// Yêu cầu gia hạn: không có nơi nào trong code ghi hai cột này (không có dữ liệu thật); theo kiểu thời điểm
	// của API là UTC.
	{"extension_requests", "requested_until", zoneUTC},
	{"extension_requests", "approved_until", zoneUTC},
	// Bảng trắng: saved_at ghi bằng CURRENT_TIMESTAMP của phiên VN.
	{"whiteboard_snapshots", "saved_at", zoneVN},
}

// Tên kiểu theo pg_type (regtype), dùng cho cả điều kiện tìm cột lẫn câu ALTER.
const (
	regTimestamp   = "timestamp"
	regTimestamptz = "timestamptz"
)

// migrateInstantColumnsUp: timestamp -> timestamptz, giữ NULL. Phải chạy TRƯỚC AutoMigrate: khi model đã khai
// timestamptz, AutoMigrate thấy cột khác kiểu và tự ALTER ... USING col::timestamptz, lấy múi giờ của PHIÊN kết
// nối (đúng chỉ khi DSN là Asia/Ho_Chi_Minh, lệch 7 giờ với kết nối UTC). Chạy trước để chuyển đổi tường minh.
func migrateInstantColumnsUp(db *gorm.DB) error {
	// Sửa dữ liệu L2 đã đổi sai TRƯỚC khi đổi các cột còn là timestamp (xem fixL2WronglyConvertedInstants).
	if err := fixL2WronglyConvertedInstants(db); err != nil {
		return err
	}
	return convertTimestampColumns(db, instantColumns, regTimestamp, regTimestamptz)
}

// migrateInstantColumnsDown: hoàn tác — timestamptz -> timestamp theo múi giờ nguồn của từng cột. Chỉ dùng khi
// quay về bản code cũ; không được gọi lúc khởi động.
func migrateInstantColumnsDown(db *gorm.DB) error {
	return convertTimestampColumns(db, instantColumns, regTimestamptz, regTimestamp)
}
