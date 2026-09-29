package dto

// Lane S2, lỗi 2: email học viên chỉ thuộc về chính chủ và admin. Các DTO dưới đây được người
// KHÁC (giảng viên, bạn cùng nhóm/lớp, người đọc review, phụ huynh) nhận qua API nên không được có
// trường email nào. Thêm lại trường email vào một trong các DTO này thì test ĐỎ.
//
// Cố ý dùng reflection trên khoá JSON (không dùng tên field Go) vì thứ lộ ra ngoài là khoá JSON.

import (
	"reflect"
	"strings"
	"testing"
)

func jsonEmailKeys(typ reflect.Type, path string, seen map[reflect.Type]bool) []string {
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
			name = f.Name // không có tag: encoding/json dùng tên field Go
		}
		if strings.Contains(strings.ToLower(name), "email") {
			found = append(found, path+"."+name)
		}
		found = append(found, jsonEmailKeys(f.Type, path+"."+name, seen)...)
	}
	return found
}

func TestS2_DTOGuiChoNguoiKhacKhongCoEmailHocVien(t *testing.T) {
	for name, typ := range map[string]reflect.Type{
		"CourseEnrollmentListDTO (giảng viên xem học viên của khoá)": reflect.TypeOf(CourseEnrollmentListDTO{}),
		"DebugEnrollmentDTO":                                          reflect.TypeOf(DebugEnrollmentDTO{}),
		"StudentClassListResponseDTO (học viên của lớp)":              reflect.TypeOf(StudentClassListResponseDTO{}),
		"TeacherStudentListResponseDTO (học viên của giảng viên)":     reflect.TypeOf(TeacherStudentListResponseDTO{}),
		"GroupMemberListResponse (thành viên nhóm)":                   reflect.TypeOf(GroupMemberListResponse{}),
		"JoinRequestListResponse (yêu cầu vào nhóm)":                  reflect.TypeOf(JoinRequestListResponse{}),
		"ParticipantResponse (người tham gia hội thoại)":              reflect.TypeOf(ParticipantResponse{}),
		"SubmissionListDTO (bài nộp, giảng viên xem)":                 reflect.TypeOf(SubmissionListDTO{}),
		"SubmissionResponseDTO":                                       reflect.TypeOf(SubmissionResponseDTO{}),
		"ChildOverviewDto (phụ huynh xem con)":                        reflect.TypeOf(ChildOverviewDto{}),
	} {
		if keys := jsonEmailKeys(typ, "", map[reflect.Type]bool{}); len(keys) > 0 {
			t.Errorf("%s có khoá JSON email: %v", name, keys)
		}
	}
}

// Kiểm chứng chính hàm dò: một DTO có email lồng sâu PHẢI bị phát hiện, nếu không test trên xanh
// vì dò hỏng chứ không vì DTO sạch.
func TestS2_JsonEmailKeys_PhatHienEmailLongSau(t *testing.T) {
	type inner struct {
		UserEmail string `json:"user_email"`
	}
	type outer struct {
		Items []struct{ U *inner } `json:"items"`
	}
	if keys := jsonEmailKeys(reflect.TypeOf(outer{}), "", map[reflect.Type]bool{}); len(keys) != 1 {
		t.Fatalf("phải phát hiện đúng 1 khoá email lồng sâu, nhận %v", keys)
	}
	type tagged struct {
		E string `json:"email,omitempty"`
		H string `json:"-"`
	}
	if keys := jsonEmailKeys(reflect.TypeOf(tagged{}), "", map[reflect.Type]bool{}); len(keys) != 1 {
		t.Fatalf("phải phát hiện email có omitempty, nhận %v", keys)
	}
}
