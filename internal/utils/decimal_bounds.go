package utils

import "github.com/shopspring/decimal"

// QA7 review M1: shopspring/decimal rescales a value to a common exponent before Compare/GreaterThan/Round/IsInteger,
// materialising 10^|exp| as a big.Int. A 20-byte JSON number such as `1e20000000` therefore costs ~2 s of CPU in a plain
// `GreaterThan(1e9)`, and a larger exponent grows superlinearly: a cheap request-driven DoS on every money field.
// Bounding the exponent BEFORE any comparison keeps every later operation O(1) in the exponent.
const (
	// DecimalExponentMax: số mũ dương tối đa. 10^9 là giá trị lớn nhất mọi trường tiền của hệ thống chấp nhận (VND).
	DecimalExponentMax = 9
	// DecimalScaleMax: số chữ số thập phân tối đa còn được chấp nhận ("1000.000000" vẫn là số nguyên 1000), đủ rộng cho
	// mọi cách client viết số nguyên mà vẫn chặn được 10^N khổng lồ.
	DecimalScaleMax = 30
)

// DecimalExponentWithinBounds báo exponent của v có nằm trong [-DecimalScaleMax, DecimalExponentMax] không, đọc trực tiếp
// từ biểu diễn (không so sánh, không rescale). Phải gọi TRƯỚC IsInteger/GreaterThan/Round trên số lấy từ request.
func DecimalExponentWithinBounds(v decimal.Decimal) bool {
	exp := v.Exponent()
	return exp <= DecimalExponentMax && exp >= -DecimalScaleMax
}
