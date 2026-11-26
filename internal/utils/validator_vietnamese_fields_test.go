package utils

// QA8 PG-07: full_name nhập quá dài/ngắn/trống phải ra câu tiếng Việt nêu giới hạn (form đăng ký hiện thẳng câu này), các trường
// khác giữ câu tiếng Anh cũ. Đỏ khi bỏ vietnameseFieldMessage khỏi getErrorMessage.

import (
	"strings"
	"testing"
)

type fullNameProbe struct {
	FullName string `json:"full_name" validate:"required,min=2,max=255"`
	Other    string `json:"other" validate:"required,max=3"`
}

func TestValidateStruct_FullName_VietnameseMessages(t *testing.T) {
	for _, tc := range []struct {
		name, value, want string
	}{
		{"quá dài", strings.Repeat("a", 256), "Họ và tên tối đa 255 ký tự."},
		{"quá ngắn", "a", "Họ và tên tối thiểu 2 ký tự."},
		{"trống", "", "Họ và tên không được để trống."},
	} {
		t.Run(tc.name, func(t *testing.T) {
			errs := ValidateStruct(fullNameProbe{FullName: tc.value, Other: "ok"})
			if len(errs) != 1 || errs[0].Field != "full_name" || errs[0].Message != tc.want {
				t.Fatalf("errs=%+v, muốn 1 lỗi full_name %q", errs, tc.want)
			}
		})
	}
	errs := ValidateStruct(fullNameProbe{FullName: "Nguyen Van A", Other: "toolong"})
	if len(errs) != 1 || errs[0].Message != "other must be at most 3 characters" {
		t.Fatalf("trường khác phải giữ câu cũ: %+v", errs)
	}
}
