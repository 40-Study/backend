package dto

// Lane S3, lỗi 2: /api/teachers và /api/teachers/:id không cần đăng nhập, danh sách giảng viên của
// lớp thì học viên nào trong lớp cũng đọc. Email giảng viên chỉ thuộc về chính chủ và admin, nên
// các DTO này không được có khoá JSON email. Thêm lại trường email thì test ĐỎ.
// Dùng lại jsonEmailKeys (s2_no_student_email_test.go): dò theo khoá JSON, kể cả lồng sâu.

import (
	"reflect"
	"testing"
)

func TestS3_TeacherDTOsKhongCoEmail(t *testing.T) {
	for name, typ := range map[string]reflect.Type{
		"TeacherResponseDTO (GET /teachers/:id, công khai)":        reflect.TypeOf(TeacherResponseDTO{}),
		"TeacherListResponseDTO (GET /teachers, công khai)":        reflect.TypeOf(TeacherListResponseDTO{}),
		"TeacherClassListResponseDTO (giảng viên của lớp)":         reflect.TypeOf(TeacherClassListResponseDTO{}),
		"TeacherClassResponseDTO (giảng viên của lớp, một dòng)": reflect.TypeOf(TeacherClassResponseDTO{}),
	} {
		if keys := jsonEmailKeys(typ, "", map[reflect.Type]bool{}); len(keys) > 0 {
			t.Errorf("%s có khoá JSON email: %v", name, keys)
		}
	}
}
