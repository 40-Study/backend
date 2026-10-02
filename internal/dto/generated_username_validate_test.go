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
	// Đối chứng: chính validator vẫn từ chối '_' (không nới validator).
	bad := "abc_def"
	if errs := utils.ValidateStruct(UpdateMeRequestDto{Username: &bad}); len(errs) == 0 {
		t.Fatal("validator đã bị nới: 'abc_def' phải bị từ chối")
	}
}
