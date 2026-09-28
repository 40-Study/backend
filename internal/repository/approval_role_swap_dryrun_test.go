package repository

// Review PR #73 (MINOR): bất biến "duyệt hồ sơ GV phải GỠ TEACHER_APPLICANT" trước đây chỉ được
// approval_pg_test.go bắt — test đó tự SKIP khi không có Postgres/.env, nên xoá dòng gỡ role vẫn
// để gate xanh. Test này chạy DryRun (DummyDialector, không cần DB): ghi lại MỌI câu UPDATE/INSERT
// mà swapApplicantToTeacherTx sinh ra và đòi có đúng câu gỡ role ứng viên (status -> inactive).

import (
	"strings"
	"testing"

	"github.com/google/uuid"
	"gorm.io/gorm"
	gormtests "gorm.io/gorm/utils/tests"
)

func TestSwapApplicantToTeacher_DryRun_RevokesApplicantAfterGrant(t *testing.T) {
	db, err := gorm.Open(gormtests.DummyDialector{}, &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	var writes []capturedStatement
	capture := func(d *gorm.DB) {
		writes = append(writes, capturedStatement{sql: d.Statement.SQL.String(), vars: append([]interface{}(nil), d.Statement.Vars...)})
	}
	if err := db.Callback().Update().After("gorm:update").Register("test:swap_update", capture); err != nil {
		t.Fatal(err)
	}
	if err := db.Callback().Create().After("gorm:create").Register("test:swap_create", capture); err != nil {
		t.Fatal(err)
	}

	if err := swapApplicantToTeacherTx(db.Session(&gorm.Session{DryRun: true}), uuid.New(), uuid.New()); err != nil {
		t.Fatalf("swapApplicantToTeacherTx: %v", err)
	}

	grantIdx, revokeIdx := -1, -1
	for i, w := range writes {
		if !strings.Contains(w.sql, "user_system_roles") {
			continue
		}
		switch {
		case containsVar(w.vars, "active") && grantIdx == -1:
			grantIdx = i
		case containsVar(w.vars, "inactive"):
			revokeIdx = i
		}
	}
	if grantIdx == -1 {
		t.Fatalf("khong co cau gan role TEACHER (status active): %+v", writes)
	}
	if revokeIdx == -1 {
		t.Fatalf("duyet ho so KHONG go TEACHER_APPLICANT (thieu UPDATE status inactive): %+v", writes)
	}
	if revokeIdx < grantIdx {
		t.Fatal("phai GAN TEACHER truoc roi moi GO TEACHER_APPLICANT (tranh user 0 vai tro)")
	}
}
