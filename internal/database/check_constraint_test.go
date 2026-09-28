package database

import (
	"strings"
	"testing"

	"study.com/v1/internal/model"
)

// TestBuildCheckConstraintSQL_CourseStatusCoversEveryValue: SQL sinh cho chk_courses_status phải
// (1) liệt kê ĐỦ mọi giá trị model.CourseStatuses trong CHECK IN (...) và (2) kiểm ĐỦ mọi giá trị
// trong điều kiện "đã đúng sẵn" — nếu (2) thiếu 'rejected', DB cũ có constraint 4 giá trị sẽ bị
// coi là đúng và KHÔNG bao giờ được nới, luồng từ chối khoá học vỡ ở tầng DB.
func TestBuildCheckConstraintSQL_CourseStatusCoversEveryValue(t *testing.T) {
	sql := buildCheckConstraintSQL("courses", "chk_courses_status", "status", model.CourseStatuses)

	for _, s := range model.CourseStatuses {
		if !strings.Contains(sql, "'"+s+"'") {
			t.Errorf("CHECK IN thieu gia tri %q", s)
		}
		if !strings.Contains(sql, "LIKE '%''"+s+"''%'") {
			t.Errorf("dieu kien kiem noi dung constraint thieu %q", s)
		}
	}
	for _, want := range []string{
		"conname = 'chk_courses_status'",
		"ALTER TABLE courses DROP CONSTRAINT IF EXISTS chk_courses_status",
		"ALTER TABLE courses ADD CONSTRAINT chk_courses_status",
		"CHECK (status IN (",
	} {
		if !strings.Contains(sql, want) {
			t.Errorf("SQL thieu %q:\n%s", want, sql)
		}
	}
}

// TestBuildCheckConstraintSQL_QuotedValueMatchIsExact: điều kiện LIKE bọc giá trị trong dấu nháy
// ('%''draft''%') để 'published' không khớp nhầm khi constraint chỉ có một giá trị chứa chuỗi con
// giống nó — so khớp theo token có nháy, không theo substring trần.
func TestBuildCheckConstraintSQL_QuotedValueMatchIsExact(t *testing.T) {
	sql := buildCheckConstraintSQL("t", "c", "col", []string{"pending"})
	if strings.Contains(sql, "LIKE '%pending%'") {
		t.Fatal("LIKE phai boc gia tri trong dau nhay de khop chinh xac token")
	}
}

func TestRunPostMigrationsStatements_TeacherApprovalConstraintFromSSOT(t *testing.T) {
	sql := buildCheckConstraintSQL("teacher_profiles", "chk_teacher_profiles_approval_status",
		"approval_status", model.TeacherApprovalStatuses)
	for _, s := range model.TeacherApprovalStatuses {
		if !strings.Contains(sql, "'"+s+"'") {
			t.Errorf("thieu %q", s)
		}
	}
}
