package service

import "testing"

// Phase 3 (quyết định #4): đăng ký làm giáo viên giờ vào hàng chờ duyệt — người dùng chỉ TỰ nhận
// được TEACHER_APPLICANT; TEACHER chỉ có sau khi admin duyệt. Nếu ai đó thêm lại "TEACHER" vào
// allowlist tự-cấp, luồng duyệt bị vô hiệu hoàn toàn (tự chọn role TEACHER lúc đăng nhập lần đầu).
func TestSelfServiceSystemRoles_TeacherRequiresApproval(t *testing.T) {
	if isSelfServiceSystemRole("TEACHER") {
		t.Fatal("TEACHER khong duoc tu cap — phai qua duyet ho so")
	}
	for _, r := range []string{"STUDENT", "PARENT", "TEACHER_APPLICANT"} {
		if !isSelfServiceSystemRole(r) {
			t.Errorf("%s phai tu cap duoc", r)
		}
	}
	for _, r := range []string{"SYSTEM_ADMIN", "ORG_OWNER"} {
		if isSelfServiceSystemRole(r) {
			t.Errorf("%s khong duoc tu cap", r)
		}
	}
}
