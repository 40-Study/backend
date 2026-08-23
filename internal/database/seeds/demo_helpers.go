package seeds

import (
	"database/sql"
	"regexp"
	"strings"
	"time"

	"github.com/shopspring/decimal"
)

var nonSlugChars = regexp.MustCompile(`[^a-z0-9]+`)

// slugify chuyển tên thành slug URL-safe (chỉ ASCII thường, phân cách bằng "-").
// Đủ dùng cho tên tag demo vốn đã là ASCII; tên tiếng Việt phải truyền slug thủ công.
func slugify(name string) string {
	s := nonSlugChars.ReplaceAllString(strings.ToLower(name), "-")
	return strings.Trim(s, "-")
}

// DemoPassword là mật khẩu dùng chung cho toàn bộ tài khoản demo.
const DemoPassword = "Demo@123"

// ptr trả về con trỏ tới v. Dùng cho các trường optional (*string, *int...)
// mà model khai báo dưới dạng con trỏ.
func ptr[T any](v T) *T {
	return &v
}

// nullStr đóng gói chuỗi thành sql.NullString đã set Valid.
func nullStr(s string) sql.NullString {
	return sql.NullString{String: s, Valid: s != ""}
}

// money tạo decimal từ số nguyên VND.
func money(v int64) decimal.Decimal {
	return decimal.NewFromInt(v)
}

// rating tạo decimal 1 chữ số thập phân cho điểm đánh giá.
func rating(v float64) decimal.Decimal {
	return decimal.NewFromFloat(v)
}

// daysAgo trả về mốc thời gian cách hiện tại n ngày.
func daysAgo(n int) time.Time {
	return time.Now().AddDate(0, 0, -n)
}

// daysAhead trả về mốc thời gian sau hiện tại n ngày.
func daysAhead(n int) time.Time {
	return time.Now().AddDate(0, 0, n)
}
