package database

import (
	"fmt"
	"strings"
	"testing"

	"study.com/v1/internal/model"
	"study.com/v1/internal/testutil/pgtest"
)

// TestBuildStatusCheckConstraintSQL_ListsEverySSOTValue — SQL sinh ra phải chứa đúng các giá trị
// của model.PayoutStatuses và kiểm số dấu nháy = 2*len (để phát hiện constraint cũ THỪA giá trị).
func TestBuildStatusCheckConstraintSQL_ListsEverySSOTValue(t *testing.T) {
	sql := buildStatusCheckConstraintSQL("instructor_payouts", "chk_instructor_payouts_status", "status", model.PayoutStatuses)
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

// TestBuildStatusCheckConstraintSQL_Postgres — chạy thật trên Postgres, trong 1 transaction
// ROLLBACK, trên bảng tạm: constraint cũ (thừa 'processing'/'failed', thiếu 'approved') phải bị
// thay bằng constraint mới; chạy lần 2 là no-op; giá trị cũ bị từ chối, giá trị mới được nhận.
// Không có Postgres: skip ở local, FAIL khi CI=true (pgtest.Open).
func TestBuildStatusCheckConstraintSQL_Postgres(t *testing.T) {
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

	build := buildStatusCheckConstraintSQL("qa_payouts", "chk_qa_payouts_status", "status", model.PayoutStatuses)
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
	must(buildStatusCheckConstraintSQL("qa_payouts2", "chk_qa_payouts2_status", "status", model.PayoutStatuses))
	tx.Raw("SELECT pg_get_constraintdef(oid) FROM pg_constraint WHERE conname = 'chk_qa_payouts2_status'").Scan(&def)
	if strings.Contains(def, "processing") {
		t.Fatalf("constraint tập cha không được thay: %s", def)
	}
}
