package dto

// MAJOR review L3: user_name sinh cho tài khoản Google phải qua được validator của UpdateMeRequestDto
// (`omitempty,alphanum,min=3,max=30`). Web luôn gửi lại username cũ mỗi lần lưu hồ sơ, nên tên có '_' làm 100%
// tài khoản Google không lưu được hồ sơ (400). Đổi NewSafeUserName sang có '_' hoặc quá 30 ký tự thì test ĐỎ.

import (
	"strings"
	"testing"

	"study.com/v1/internal/utils"
)

func TestGeneratedUserName_QuaValidatorUpdateMe(t *testing.T) {
	s := func(v string) *string { return &v }
	names := []*string{nil, s(""), s("Nguyễn Văn Đạt"), s("A"), s("張偉"), s(strings.Repeat("Nguyen ", 20)), s("a.b_c-d")}
	for _, full := range names {
		for i := 0; i < 20; i++ {
			name, err := utils.NewSafeUserName(full)
			if err != nil {
				t.Fatal(err)
			}
			req := UpdateMeRequestDto{Username: &name}
			if errs := utils.ValidateStruct(req); len(errs) != 0 {
				t.Fatalf("user_name sinh ra %q không qua UpdateMeRequestDto: %+v", name, errs)
			}
		}
	}
}

// QA hồi quy 03/10 (A-09, B-03): một luật user_name duy nhất cho đăng ký và sửa hồ sơ. '_' được phép (web cho
// phép từ trước), '@' và dạng email bị từ chối ở CẢ HAI đường để email không bao giờ thành user_name công khai.
func TestUserName_MotLuatChoDangKyVaSuaHoSo(t *testing.T) {
	cases := []struct {
		name string
		ok   bool
	}{
		{"student1_qa", true},
		{"student123", true},
		{"_ab", true},
		{"ab", false},
		{"a23456789012345678901234567890", true},   // 30 ký tự
		{"a234567890123456789012345678901", false}, // 31 ký tự
		{"tvanle.dev@gmail.com", false},
		{"abc@def", false},
		{"abc.def", false},
		{"abc def", false},
		{"nguyễnvăn", false},
		{"a-b-c", false},
	}
	for _, c := range cases {
		reg := utils.ValidateStruct(RegisterRequestDto{
			Email: "a@b.co", Password: "SecurePass123!", ConfirmPassword: "SecurePass123!", UserName: c.name,
		})
		upd := utils.ValidateStruct(UpdateMeRequestDto{Username: &c.name})
		if (len(reg) == 0) != c.ok {
			t.Errorf("đăng ký user_name %q: ok=%v, muốn %v (%+v)", c.name, len(reg) == 0, c.ok, reg)
		}
		if (len(upd) == 0) != c.ok {
			t.Errorf("sửa hồ sơ user_name %q: ok=%v, muốn %v (%+v)", c.name, len(upd) == 0, c.ok, upd)
		}
	}
}
