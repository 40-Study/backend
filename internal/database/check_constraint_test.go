package database

import (
	"bytes"
	"fmt"
	"log"
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
	// Dữ liệu sạch: ADD ... NOT VALID rồi VALIDATE phải để lại constraint VALIDATED đầy đủ.
	var validated bool
	tx.Raw("SELECT convalidated FROM pg_constraint WHERE conname = 'chk_qa_payouts_status'").Scan(&validated)
	if !validated {
		t.Fatal("dữ liệu sạch nhưng constraint còn NOT VALID — thiếu bước VALIDATE")
	}

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

// TestBuildCheckConstraintSQL_PostgresLegacyRowsDoNotBlockBoot (review PR #79, rollback): DB đã có
// dòng mang giá trị mà danh sách của bản đang boot KHÔNG còn (vd. rollback về bản chưa có
// 'cancelled'). Post-migration KHÔNG được lỗi (lỗi = backend chết lúc khởi động), nhưng ghi mới giá
// trị đó vẫn phải bị chặn; dọn dữ liệu xong thì lần chạy sau validate đầy đủ.
func TestBuildCheckConstraintSQL_PostgresLegacyRowsDoNotBlockBoot(t *testing.T) {
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
	validated := func() bool {
		var v bool
		tx.Raw("SELECT convalidated FROM pg_constraint WHERE conname = 'chk_qa_rb_status'").Scan(&v)
		return v
	}
	must(`CREATE TEMP TABLE qa_rb (status varchar(20),
		CONSTRAINT chk_qa_rb_status CHECK (status IN ('pending','cancelled')))`)
	must("INSERT INTO qa_rb VALUES ('pending'), ('cancelled')")

	oldList := []string{"pending"} // bản "cũ" không biết 'cancelled'
	build := buildCheckConstraintSQL("qa_rb", "chk_qa_rb_status", "status", oldList)
	must(build) // trước bản vá: "violates check constraint" -> backend không boot được
	if validated() {
		t.Fatal("còn dòng 'cancelled' mà constraint lại báo validated")
	}
	if names, err := notValidCheckConstraints(tx); err != nil || !containsStr(names, "qa_rb.chk_qa_rb_status") {
		t.Fatalf("notValidCheckConstraints = %v (err=%v), muốn có qa_rb.chk_qa_rb_status", names, err)
	}
	must("SAVEPOINT sp")
	if err := exec("INSERT INTO qa_rb VALUES ('cancelled')"); err == nil {
		t.Fatal("constraint NOT VALID vẫn phải chặn ghi MỚI giá trị ngoài danh sách")
	}
	must("ROLLBACK TO SAVEPOINT sp")

	must("DELETE FROM qa_rb WHERE status = 'cancelled'")
	must(build)
	if !validated() {
		t.Fatal("dữ liệu đã sạch nhưng lần chạy sau không validate lại constraint")
	}
	if names, _ := notValidCheckConstraints(tx); containsStr(names, "qa_rb.chk_qa_rb_status") {
		t.Fatalf("đã validate nhưng vẫn báo NOT VALID: %v", names)
	}
}

// TestRunPostMigrations_LogsNotValidConstraint (re-review vòng 2 PR #79): RAISE WARNING chỉ vào log
// Postgres, nên RunPostMigrations phải tự ghi vào log BACKEND mỗi CHECK constraint còn NOT VALID.
// Chạy trong transaction ROLLBACK trên DB đã migrate.
func TestRunPostMigrations_LogsNotValidConstraint(t *testing.T) {
	db := pgtest.IsolatedSchema(t, Migrate) // schema tạm đã Migrate: không phụ thuộc DB dùng chung đã AutoMigrate cột mới
	tx := db.Begin()
	defer tx.Rollback()
	for _, sql := range []string{
		"CREATE TEMP TABLE qa_nv (status varchar(20))",
		"INSERT INTO qa_nv VALUES ('cu')",
		"ALTER TABLE qa_nv ADD CONSTRAINT chk_qa_nv_status CHECK (status IN ('moi')) NOT VALID",
	} {
		if err := tx.Exec(sql).Error; err != nil {
			t.Fatalf("%v\n%s", err, sql)
		}
	}
	var buf bytes.Buffer
	prev := log.Writer()
	log.SetOutput(&buf)
	defer log.SetOutput(prev)
	if err := RunPostMigrations(tx); err != nil {
		t.Fatalf("RunPostMigrations: %v", err)
	}
	if !strings.Contains(buf.String(), "qa_nv.chk_qa_nv_status") || !strings.Contains(buf.String(), "NOT VALID") {
		t.Fatalf("log backend không có cảnh báo NOT VALID cho qa_nv.chk_qa_nv_status:\n%s", buf.String())
	}
}

func containsStr(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}