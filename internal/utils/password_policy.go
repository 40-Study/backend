package utils

import "unicode"

// PasswordMinLength là độ dài tối thiểu của mật khẩu MỚI (đăng ký, đặt lại, đổi mật khẩu).
const PasswordMinLength = 8

// PasswordPolicyMessage là thông báo duy nhất cho mọi form đặt mật khẩu mới. Web hiển thị
// đúng câu này (và dùng cùng 4 luật cho checklist) nên đổi luật phải đổi cả hai nơi.
const PasswordPolicyMessage = "Mật khẩu phải có ít nhất 8 ký tự, gồm ít nhất 1 chữ in hoa, 1 chữ số và 1 ký tự đặc biệt"

// IsStrongPassword là SSOT của chính sách mật khẩu mới: >= 8 ký tự, có >= 1 chữ in hoa,
// >= 1 chữ số, >= 1 ký tự đặc biệt (không phải chữ, không phải số, không phải khoảng trắng).
// Chỉ áp cho mật khẩu mới; đăng nhập KHÔNG dùng hàm này để mật khẩu yếu cũ vẫn vào được.
func IsStrongPassword(p string) bool {
	runes := []rune(p)
	if len(runes) < PasswordMinLength {
		return false
	}
	var upper, digit, special bool
	for _, r := range runes {
		switch {
		case unicode.IsUpper(r):
			upper = true
		case unicode.IsDigit(r):
			digit = true
		case unicode.IsLetter(r), unicode.IsSpace(r):
			// chữ thường / khoảng trắng: không tính vào bất kỳ yêu cầu nào
		default:
			special = true
		}
	}
	return upper && digit && special
}
