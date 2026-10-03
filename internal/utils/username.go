package utils

// username.go — sinh user_name an toàn cho tài khoản OAuth không có username riêng (Google). Không bao
// giờ dùng email: user_name hiển thị công khai (leaderboard, contest, thành viên nhóm), nên phần trước '@'
// làm lộ một phần email (issue #105). Dùng chung cho đăng ký OAuth (service) và migration đổi tên cũ
// (database), nên đặt ở utils để hai nơi không lệch nhau.

import (
	"regexp"
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

// UserNamePattern là luật user_name DUY NHẤT (SSOT) cho cả đăng ký lẫn sửa hồ sơ: chữ không dấu, số và dấu gạch
// dưới, 3-30 ký tự. Cố ý KHÔNG cho '@' và '.', nên một địa chỉ email không bao giờ là user_name hợp lệ (user_name
// hiện công khai ở bảng xếp hạng và hồ sơ công khai, QA hồi quy 03/10 B-03). Web phản chiếu đúng luật này ở
// web/src/lib/validations/auth.ts; đổi một bên phải đổi bên kia.
const UserNamePattern = `^[A-Za-z0-9_]{3,30}$`

var userNameRegexp = regexp.MustCompile(UserNamePattern)

// IsValidUserName kiểm một user_name nhập tay theo UserNamePattern.
func IsValidUserName(s string) bool { return userNameRegexp.MatchString(s) }

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

// NewSafeUserName ghép <tên không dấu><hậu tố ngẫu nhiên 6 ký tự a-z0-9>, KHÔNG có dấu gạch dưới: user_name
// phải qua được validator `user_name` (UserNamePattern) của UpdateMeRequestDto (PUT /users/me), nếu không tài khoản
// Google không lưu được hồ sơ vì web luôn gửi lại username cũ. Dài tối đa 20+6=26 ký tự. Hậu tố là phần lấy ngẫu nhiên,
// KHÔNG suy ra từ email. Không đảm bảo duy nhất: người gọi phải kiểm tra tồn tại (user_name không có
// unique index vì dữ liệu cũ đã trùng tên).
func NewSafeUserName(fullName *string) (string, error) {
	suffix, err := gonanoid.Generate(safeUserNameAlphabet, safeUserNameSuffixLen)
	if err != nil {
		return "", err
	}
	return SafeUserNameBase(fullName) + suffix, nil
}
