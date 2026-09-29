package dto

// Lane S3, lỗi 2: /api/teachers và /api/teachers/:id không cần đăng nhập, danh sách giảng viên của
// lớp thì học viên nào trong lớp cũng đọc. Email, số điện thoại và ngày sinh giảng viên chỉ thuộc về
// chính chủ và admin, nên các DTO này không được có khoá JSON nào trong số đó. Thêm lại một trường
// thì test ĐỎ. Dò theo khoá JSON (thứ thực sự lộ ra ngoài), kể cả lồng sâu.

import (
	"reflect"
	"strings"
	"testing"
)

var s3PersonalKeyFragments = []string{"email", "phone", "date_of_birth", "dateofbirth", "dob", "birthday"}

func jsonPersonalKeys(typ reflect.Type, path string, seen map[reflect.Type]bool) []string {
	for typ.Kind() == reflect.Ptr || typ.Kind() == reflect.Slice || typ.Kind() == reflect.Array {
		typ = typ.Elem()
	}
	if typ.Kind() != reflect.Struct || seen[typ] {
		return nil
	}
	seen[typ] = true
	var found []string
	for i := 0; i < typ.NumField(); i++ {
		f := typ.Field(i)
		if !f.IsExported() {
			continue
		}
		name := strings.Split(f.Tag.Get("json"), ",")[0]
		if name == "-" {
			continue
		}
		if name == "" {
			name = f.Name
		}
		lower := strings.ToLower(name)
		for _, frag := range s3PersonalKeyFragments {
			if strings.Contains(lower, frag) {
				found = append(found, path+"."+name)
				break
			}
		}
		found = append(found, jsonPersonalKeys(f.Type, path+"."+name, seen)...)
	}
	return found
}

func TestS3_TeacherDTOsKhongCoDuLieuCaNhan(t *testing.T) {
	for name, typ := range map[string]reflect.Type{
		"TeacherResponseDTO (GET /teachers/:id, công khai)":        reflect.TypeOf(TeacherResponseDTO{}),
		"TeacherListResponseDTO (GET /teachers, công khai)":        reflect.TypeOf(TeacherListResponseDTO{}),
		"TeacherClassListResponseDTO (giảng viên của lớp)":         reflect.TypeOf(TeacherClassListResponseDTO{}),
		"TeacherClassResponseDTO (giảng viên của lớp, một dòng)": reflect.TypeOf(TeacherClassResponseDTO{}),
	} {
		if keys := jsonPersonalKeys(typ, "", map[reflect.Type]bool{}); len(keys) > 0 {
			t.Errorf("%s có khoá JSON dữ liệu cá nhân: %v", name, keys)
		}
	}
}

// Kiểm chứng chính hàm dò: nếu nó mù thì test trên xanh vì dò hỏng chứ không vì DTO sạch.
func TestS3_JsonPersonalKeys_PhatHienPhoneVaNgaySinhLongSau(t *testing.T) {
	type inner struct {
		Phone *string `json:"phone,omitempty"`
		DOB   string  `json:"date_of_birth"`
		Mail  string  `json:"email"`
		Name  string  `json:"user_name"`
	}
	type outer struct {
		Items []struct{ U *inner } `json:"items"`
	}
	if keys := jsonPersonalKeys(reflect.TypeOf(outer{}), "", map[reflect.Type]bool{}); len(keys) != 3 {
		t.Fatalf("phải phát hiện đúng 3 khoá (phone, date_of_birth, email), nhận %v", keys)
	}
}
