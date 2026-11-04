package utils

import "testing"

func TestIsStrongPassword(t *testing.T) {
	cases := map[string]bool{
		"Abcdef1!":     true,
		"abcdefgh":     false, // thiếu hoa, số, đặc biệt (ca QA P-04)
		"Abcdefg1":     false, // thiếu đặc biệt
		"abcdef1!":     false, // thiếu hoa
		"Abcdefg!":     false, // thiếu số
		"Ab1!":         false, // ngắn
		"Abcde 1 ":     false, // khoảng trắng không phải ký tự đặc biệt
		"Mật-khẩu1":    true,  // Unicode: hoa + số + '-'
		"ResilientP4_": true,
	}
	for pw, want := range cases {
		if got := IsStrongPassword(pw); got != want {
			t.Errorf("IsStrongPassword(%q) = %v, muốn %v", pw, got, want)
		}
	}
}

type passwordProbe struct {
	Password string `validate:"required,max=72,strong_password"`
}

func TestValidateStruct_StrongPasswordUsesVietnameseMessage(t *testing.T) {
	errs := ValidateStruct(passwordProbe{Password: "abcdefgh"})
	if len(errs) != 1 || errs[0].Tag != "strong_password" || errs[0].Message != PasswordPolicyMessage {
		t.Fatalf("muốn 1 lỗi strong_password với thông báo chuẩn, nhận %+v", errs)
	}
	if errs := ValidateStruct(passwordProbe{Password: "Abcdef1!"}); len(errs) != 0 {
		t.Fatalf("mật khẩu hợp lệ bị từ chối: %+v", errs)
	}
}
