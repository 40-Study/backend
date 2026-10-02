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
// zone từng cột = múi giờ của nơi GHI giá trị cũ (xem timestampColumn.zone). Quy tắc mặc định là giờ VN (quyết
// định của chủ dự án 02/10/2026: backend, seed và CURRENT_TIMESTAMP đều ghi giờ VN); chỉ cột mà code ghi từ chuỗi
// UTC của web mới là UTC. Bảng quyết định đủ 20 cột (đợt L2 + L7) nằm trong mô tả PR của L7.
//
// KHÔNG gồm start_time/end_time của class_schedules, class_sessions: đó là giờ-trong-ngày "HH:MM" của lịch
// lặp lại (model.TimeOfDay, #104) đi cùng cột `date`, đã đúng kiểu `time`. Ranh giới nửa đêm giờ VN của điểm
// danh dựa vào cột `date` (requireCheckInOpen), không dựa vào các cột này nên không đổi.
var instantColumns = []timestampColumn{
	// L2: thời điểm buổi học (lịch live, nội dung học theo lớp). Seed và time.Now() ghi giờ VN.
	{"class_lesson_contents", "open_date", zoneVN},
	{"class_lesson_contents", "due_date", zoneVN},
	{"class_lesson_contents", "scheduled_at", zoneVN},
	{"class_lesson_contents", "end_at", zoneVN},
	{"livestream_sessions", "scheduled_at", zoneVN},
	{"livestream_sessions", "started_at", zoneVN},
	{"livestream_sessions", "ended_at", zoneVN},

	// L7. Bài tập: start_time/end_time do web gửi bằng toISOString() ("...Z"), service time.Parse(RFC3339) giữ
	// UTC nên pgx ghi giờ đồng hồ UTC (và API đọc lại gắn nhãn UTC: hiện đang ĐÚNG). Chuyển theo UTC để giữ
	// nguyên instant. published_at ghi bằng CURRENT_TIMESTAMP của phiên VN nên là giờ VN.
	{"assignments", "published_at", zoneVN},
	{"assignments", "start_time", zoneUTC},
	{"assignments", "end_time", zoneUTC},
	// Người tham gia livestream: joined_at mặc định CURRENT_TIMESTAMP, left_at/kicked_at ghi bằng
	// gorm.Expr("CURRENT_TIMESTAMP"): giờ VN.
	{"participants", "joined_at", zoneVN},
	{"participants", "left_at", zoneVN},
	{"participants", "kicked_at", zoneVN},
	// Khoá idempotency: expires_at = time.Now().Add(TTL) (order_service), so với time.Now() khi dọn: giờ VN.
	{"idempotency_keys", "expires_at", zoneVN},
	// Voucher: start_date/end_date đến từ DTO; web không có form ghi các trường này, dữ liệu thật là seed và
	// time.Now().Add(...) (giờ VN, đã đối chiếu với created_at trên DB dev).
	{"vouchers", "start_date", zoneVN},
	{"vouchers", "end_date", zoneVN},
	{"user_vouchers", "saved_at", zoneVN},
	// Yêu cầu gia hạn: không có nơi nào trong code ghi hai cột này; mặc định giờ VN.
	{"extension_requests", "requested_until", zoneVN},
	{"extension_requests", "approved_until", zoneVN},
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
	return convertTimestampColumns(db, instantColumns, regTimestamp, regTimestamptz)
}

// migrateInstantColumnsDown: hoàn tác — timestamptz -> timestamp theo múi giờ nguồn của từng cột. Chỉ dùng khi
// quay về bản code cũ; không được gọi lúc khởi động.
func migrateInstantColumnsDown(db *gorm.DB) error {
	return convertTimestampColumns(db, instantColumns, regTimestamptz, regTimestamp)
}
