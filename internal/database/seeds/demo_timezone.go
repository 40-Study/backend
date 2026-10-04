package seeds

import "time"

// seedZone là múi giờ của MỌI giờ đồng hồ trong dữ liệu demo (hạn nộp bài 23:59, giờ phiên live, giờ vào/ra lớp).
// Cố định +07 thay vì múi giờ của máy chạy seed: máy dev Windows là VN còn container `FROM scratch` không có TZ nên là UTC,
// và cùng một lệnh seed phải ra cùng một khoảnh khắc. Dùng giờ máy từng làm hạn nộp "23:59" ghi
// thành 23:59 UTC rồi hiện là 06:59 hôm sau ở web (QA-reg A-22). Việt Nam không có giờ mùa hè nên độ lệch cố định là đúng,
// và FixedZone không cần tzdata (Windows không cài Go tzdata, LoadLocation thất bại).
var seedZone = time.FixedZone("Asia/Ho_Chi_Minh", 7*60*60)

// demoTodayAt trả về 00:00 của NGÀY HIỆN TẠI THEO GIỜ VIỆT NAM tại thời điểm now. Phải đổi sang seedZone trước khi
// lấy ngày: ở UTC từ 17:00 trở đi ngày theo UTC vẫn là hôm qua so với Việt Nam.
func demoTodayAt(now time.Time) time.Time {
	v := now.In(seedZone)
	return time.Date(v.Year(), v.Month(), v.Day(), 0, 0, 0, 0, seedZone)
}

// demoWallClock ghép (day + offsetDays) với giờ:phút theo giờ Việt Nam. day chỉ cần đúng ngày/tháng/năm.
func demoWallClock(day time.Time, offsetDays, hour, min int) time.Time {
	return time.Date(day.Year(), day.Month(), day.Day()+offsetDays, hour, min, 0, 0, seedZone)
}
