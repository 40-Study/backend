package utils

// username.go — sinh user_name an toàn cho tài khoản OAuth không có username riêng (Google). Không bao
// giờ dùng email: user_name hiển thị công khai (leaderboard, contest, thành viên nhóm), nên phần trước '@'
// làm lộ một phần email (issue #105). Dùng chung cho đăng ký OAuth (service) và migration đổi tên cũ
// (database), nên đặt ở utils để hai nơi không lệch nhau.

import (
	"strings"
	"unicode"

	gonanoid "github.com/matoous/go-nanoid/v2"
	"golang.org/x/text/runes"
	"golang.org/x/text/transform"
	"golang.org/x/text/unicode/norm"
)

const (
	safeUserNameBaseMax   = 20
	safeUserNameSuffixLen = 6
	safeUserNameDefault   = "hocvien"
	safeUserNameAlphabet  = "abcdefghijklmnopqrstuvwxyz0123456789"
)

// SafeUserNameBase đổi họ tên thành chuỗi ASCII an toàn (chữ thường, số, không dấu, không khoảng trắng,
// tối đa 20 ký tự): "Nguyễn Văn Đạt" -> "nguyenvandat". Tên rỗng/không còn ký tự hợp lệ -> "hocvien".
func SafeUserNameBase(fullName *string) string {
	if fullName == nil {
		return safeUserNameDefault
	}
	// "đ" không phân rã thành chữ + dấu nên phải đổi tay trước khi bỏ dấu.
	s := strings.NewReplacer("đ", "d", "Đ", "d").Replace(*fullName)
	stripped, _, err := transform.String(transform.Chain(norm.NFD, runes.Remove(runes.In(unicode.Mn)), norm.NFC), s)
	if err != nil {
		stripped = s
	}
	var b strings.Builder
	for _, r := range strings.ToLower(stripped) {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
			if b.Len() >= safeUserNameBaseMax {
				break
			}
		}
	}
	if b.Len() == 0 {
		return safeUserNameDefault
	}
	return b.String()
}

// NewSafeUserName ghép <tên không dấu>_<hậu tố ngẫu nhiên 6 ký tự a-z0-9>. Hậu tố là phần lấy ngẫu nhiên,
// KHÔNG suy ra từ email. Không đảm bảo duy nhất: người gọi phải kiểm tra tồn tại (user_name không có
// unique index vì dữ liệu cũ đã trùng tên).
func NewSafeUserName(fullName *string) (string, error) {
	suffix, err := gonanoid.Generate(safeUserNameAlphabet, safeUserNameSuffixLen)
	if err != nil {
		return "", err
	}
	return SafeUserNameBase(fullName) + "_" + suffix, nil
}
