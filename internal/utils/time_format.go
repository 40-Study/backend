package utils

import "time"

// FormatTimestamp là helper DUY NHẤT để đổi time.Time thành chuỗi thời gian trong response API.
//
// Vì sao cần (QA vòng 2, G1 — N10/N-12): trước đây ~45 chỗ trong service tự format bằng layout
// "2006-01-02T15:04:05Z". Trong layout Go, chữ `Z` đứng sau giây KHÔNG phải chỉ thị múi giờ mà
// là ký tự literal, nên giờ local lấy từ DB (DSN `TimeZone=Asia/Ho_Chi_Minh`, vd 10:00 +07:00)
// bị in thành "…T10:00:00Z" — client hiểu là 10:00 UTC = 17:00 giờ Việt Nam, lệch +7h.
//
// time.RFC3339 ("…Z07:00") in đúng offset THẬT của giá trị: giờ đọc từ DB ra "+07:00", giờ UTC
// ra "Z". Không đổi múi giờ (không gọi .UTC()) để khớp các field time.Time mà encoding/json tự
// marshal ở chỗ khác (cũng giữ offset gốc) — mọi response cùng một kiểu chuỗi.
func FormatTimestamp(t time.Time) string {
	return t.Format(time.RFC3339)
}

// FormatTimestampPtr như FormatTimestamp cho field có thể rỗng: nil vào thì nil ra (JSON bỏ qua
// hoặc null), không bao giờ sinh chuỗi "0001-01-01…" cho giá trị chưa có.
func FormatTimestampPtr(t *time.Time) *string {
	if t == nil {
		return nil
	}
	s := FormatTimestamp(*t)
	return &s
}
