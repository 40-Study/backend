package utils

import (
	"regexp"
	"strings"
	"testing"
)

func TestSafeUserNameBase(t *testing.T) {
	str := func(s string) *string { return &s }
	cases := []struct {
		name string
		in   *string
		want string
	}{
		{"có dấu và đ", str("Nguyễn Văn Đạt"), "nguyenvandat"},
		{"khoảng trắng và ký tự lạ", str("  A.B_c-D!! 99 "), "abcd99"},
		{"nil", nil, "hocvien"},
		{"rỗng", str(""), "hocvien"},
		{"toàn ký tự ngoài ASCII", str("张伟 ✨"), "hocvien"},
		{"dài quá 20", str("Abcdefghijklmnopqrstuvwxyz"), "abcdefghijklmnopqrst"},
	}
	for _, c := range cases {
		if got := SafeUserNameBase(c.in); got != c.want {
			t.Errorf("%s: muốn %q, nhận %q", c.name, c.want, got)
		}
	}
}

func TestNewSafeUserName_DinhDangVaKhacNhau(t *testing.T) {
	re := regexp.MustCompile(`^[a-z0-9]{1,20}_[a-z0-9]{6}$`)
	full := "Lê Thị Hoa"
	seen := map[string]bool{}
	for i := 0; i < 50; i++ {
		n, err := NewSafeUserName(&full)
		if err != nil {
			t.Fatal(err)
		}
		if !re.MatchString(n) || !strings.HasPrefix(n, "lethihoa_") {
			t.Fatalf("định dạng sai: %q", n)
		}
		seen[n] = true
	}
	if len(seen) < 49 {
		t.Errorf("hậu tố phải ngẫu nhiên: 50 lần chỉ ra %d tên khác nhau", len(seen))
	}
}
