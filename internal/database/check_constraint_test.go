package database

import (
	"fmt"
	"strings"
	"testing"

	"study.com/v1/internal/model"
	"study.com/v1/internal/testutil/pgtest"
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

// TestBuildCheckConstraintSQL_PayoutStatusListsEverySSOTValue — SQL sinh ra phải chứa đúng các giá trị
// của model.PayoutStatuses và kiểm số dấu nháy = 2*len (để phát hiện constraint cũ THỪA giá trị).
func TestBuildCheckConstraintSQL_PayoutStatusListsEverySSOTValue(t *testing.T) {
	sql := buildCheckConstraintSQL("instructor_payouts", "chk_instructor_payouts_status", "status", model.PayoutStatuses)
	for _, s := range model.PayoutStatuses {
		if !strings.Contains(sql, "'"+s+"'") {
			t.Fatalf("SQL thiếu giá trị %q:\n%s", s, sql)
		}
	}
	if !strings.Contains(sql, fmt.Sprintf("= %d", 2*len(model.PayoutStatuses))) {
		t.Fatalf("SQL không kiểm số lượng giá trị:\n%s", sql)
	}
	if strings.Contains(sql, "'processing'") || strings.Contains(sql, "'failed'") {
		t.Fatal("SQL còn giá trị cũ processing/failed")
	}
}

// TestBuildCheckConstraintSQL_Postgres — chạy thật trên Postgres, trong 1 transaction
// ROLLBACK, trên bảng tạm: constraint cũ (thừa 'processing'/'failed', thiếu 'approved') phải bị
// thay bằng constraint mới; chạy lần 2 là no-op; giá trị cũ bị từ chối, giá trị mới được nhận.
// Không có Postgres: skip ở local, FAIL khi CI=true (pgtest.Open).
func TestBuildCheckConstraintSQL_Postgres(t *testing.T) {
	db := pgtest.Open(t)
	tx := db.Begin()
	defer tx.Rollback()

	exec := func(sql string) error { return tx.Exec(sql).Error }
	must := func(sql string) {
		t.Helper()
		if err := exec(sql); err != nil {
			t.Fatalf("%v\n%s", err, sql)
		}
	}
	must(`CREATE TEMP TABLE qa_payouts (status varchar(20),
		CONSTRAINT chk_qa_payouts_status CHECK (status IN ('pending','processing','completed','failed')))`)

	build := buildCheckConstraintSQL("qa_payouts", "chk_qa_payouts_status", "status", model.PayoutStatuses)
	must(build)

	var def string
	tx.Raw("SELECT pg_get_constraintdef(oid) FROM pg_constraint WHERE conname = 'chk_qa_payouts_status'").Scan(&def)
	if strings.Contains(def, "processing") || !strings.Contains(def, "approved") || !strings.Contains(def, "rejected") {
		t.Fatalf("constraint chưa được thay: %s", def)
	}
	must(build) // idempotent

	must("SAVEPOINT sp")
	if err := exec("INSERT INTO qa_payouts VALUES ('processing')"); err == nil {
		t.Fatal("constraint mới vẫn nhận 'processing'")
	}
	must("ROLLBACK TO SAVEPOINT sp")
	for _, s := range model.PayoutStatuses {
		must("INSERT INTO qa_payouts VALUES ('" + s + "')")
	}

	// Constraint cũ là TẬP CHA (đủ 4 giá trị mới + thừa 'processing'): kiểm LIKE từng giá trị đều
	// khớp, chỉ phép đếm dấu nháy phát hiện được giá trị thừa -> vẫn phải thay.
	must(`CREATE TEMP TABLE qa_payouts2 (status varchar(20),
		CONSTRAINT chk_qa_payouts2_status CHECK (status IN ('pending','approved','rejected','completed','processing')))`)
	must(buildCheckConstraintSQL("qa_payouts2", "chk_qa_payouts2_status", "status", model.PayoutStatuses))
	tx.Raw("SELECT pg_get_constraintdef(oid) FROM pg_constraint WHERE conname = 'chk_qa_payouts2_status'").Scan(&def)
	if strings.Contains(def, "processing") {
		t.Fatalf("constraint tập cha không được thay: %s", def)
	}
}
