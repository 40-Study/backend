package utils

import (
	"strings"
	"unicode"
)

// NormalizeCertificateNumber làm sạch mã chứng chỉ người dùng dán vào trước khi tra cứu (Q10-PG-05).
// Mã chứng chỉ chỉ gồm ASCII, nên mọi ký tự định dạng vô hình (Unicode Cf: U+200B-U+200D, U+2060, U+FEFF, U+200E/F,
// U+00AD...) và NBSP đều là rác do app chat/trình duyệt chèn khi sao chép; bỏ chúng ở BẤT KỲ vị trí nào, rồi bỏ
// khoảng trắng hai đầu. Không đổi kiểu chữ: việc tra không phân biệt hoa/thường nằm ở repository.
func NormalizeCertificateNumber(raw string) string {
	cleaned := strings.Map(func(r rune) rune {
		if r == '\u00a0' || unicode.Is(unicode.Cf, r) {
			return -1
		}
		return r
	}, raw)
	return strings.TrimSpace(cleaned)
}
