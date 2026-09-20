package repository

// Test cho F2 (review 260917): khoa ghi tien do chi co tac dung neu cau SQL THAT su co FOR UPDATE,
// va nhanh tao moi chi het loi "duplicated key" neu co ON CONFLICT khop DUNG partial unique index
// idx_user_lesson (WHERE deleted_at IS NULL, migrations.go) — thieu predicate thi Postgres tu choi
// ca cau INSERT. DryRun + callback de doc cau SQL ma ham san xuat sinh ra.

import (
	"context"
	"strings"
	"testing"

	"github.com/google/uuid"
	"gorm.io/gorm"
	gormtests "gorm.io/gorm/utils/tests"
	"study.com/v1/internal/model"
)

func dryRunEnrollmentRepo(t *testing.T) (*EnrollmentRepository, *string) {
	t.Helper()
	db, err := gorm.Open(gormtests.DummyDialector{}, &gorm.Config{})
	if err != nil {
		t.Fatalf("gorm.Open: %v", err)
	}
	var captured string
	capture := func(d *gorm.DB) { captured = d.Statement.SQL.String() }
	if err := db.Callback().Create().After("gorm:create").Register("test:capture_create", capture); err != nil {
		t.Fatal(err)
	}
	if err := db.Callback().Query().After("gorm:query").Register("test:capture_query", capture); err != nil {
		t.Fatal(err)
	}
	// R3 (code-reviewer-260919-1557): .Scan(&dest) (dung boi GetLessonOrderInfoByCourseID) DI
	// QUA db.Rows() -> callbacks.Row(), KHONG qua callbacks.Query() nhu Find/Pluck/Count —
	// thieu dong nay thi *captured van la chuoi RONG cho moi truy van dung Scan(), va test se
	// bao "SQL thieu loc..." du cau SQL that su (in ra qua GORM logger luc DryRun) da dung.
	if err := db.Callback().Row().After("gorm:row").Register("test:capture_row", capture); err != nil {
		t.Fatal(err)
	}
	return &EnrollmentRepository{db: db.Session(&gorm.Session{DryRun: true})}, &captured
}

func TestInsertLessonProgressIfAbsent_OnConflictKhopPartialIndex(t *testing.T) {
	repo, sql := dryRunEnrollmentRepo(t)
	_, _ = repo.InsertLessonProgressIfAbsent(context.Background(), &model.LessonProgress{UserID: uuid.New(), LessonID: uuid.New()})
	for _, want := range []string{"ON CONFLICT", "user_id", "lesson_id", "WHERE deleted_at IS NULL", "DO NOTHING"} {
		if !strings.Contains(*sql, want) {
			t.Fatalf("SQL thieu %q:\n%s", want, *sql)
		}
	}
}

func TestGetLessonProgressForUpdate_CoForUpdate(t *testing.T) {
	repo, sql := dryRunEnrollmentRepo(t)
	_, _ = repo.getLessonProgressForUpdate(context.Background(), uuid.New(), uuid.New())
	if !strings.Contains(*sql, "FOR UPDATE") {
		t.Fatalf("SELECT phai co FOR UPDATE:\n%s", *sql)
	}
}

// R3 (code-reviewer-260919-1557): GetLessonIDsByCourseID/GetLessonOrderInfoByCourseID/
// CountTotalMandatory JOIN sections nhung GORM CHI tu them dieu kien soft-delete cho MODEL
// CHINH cua query (Lesson — von khong co cot deleted_at), khong tu lan sang bang JOIN. Thieu
// "sections.deleted_at IS NULL" thi bai cua mot chuong da xoa (soft-delete) van nam trong
// LessonOrder/CountTotalMandatory, khoa bai dau chuong ke tiep vinh vien. Ba test duoi day doc
// THANG cau SQL GORM sinh ra (DryRun) — mot mock repo tra ve gia du lieu se KHONG bao gio bat
// duoc loai loi nay, vi loi nam trong CHINH cau SQL, khong nam trong du lieu tra ve.
func TestGetLessonIDsByCourseID_LocSectionsDaXoaMem(t *testing.T) {
	repo, sql := dryRunEnrollmentRepo(t)
	_, _ = repo.GetLessonIDsByCourseID(context.Background(), uuid.New())
	if !strings.Contains(*sql, "sections.deleted_at IS NULL") {
		t.Fatalf("SQL thieu loc sections.deleted_at IS NULL (chuong da xoa mem van chan khoa tuan tu):\n%s", *sql)
	}
}

func TestGetLessonOrderInfoByCourseID_LocSectionsDaXoaMem(t *testing.T) {
	repo, sql := dryRunEnrollmentRepo(t)
	_, _ = repo.GetLessonOrderInfoByCourseID(context.Background(), uuid.New())
	if !strings.Contains(*sql, "sections.deleted_at IS NULL") {
		t.Fatalf("SQL thieu loc sections.deleted_at IS NULL:\n%s", *sql)
	}
}

func TestCountTotalMandatory_LocSectionsDaXoaMem(t *testing.T) {
	repo, sql := dryRunEnrollmentRepo(t)
	_, _ = repo.CountTotalMandatory(context.Background(), uuid.New())
	if !strings.Contains(*sql, "sections.deleted_at IS NULL") {
		t.Fatalf("SQL thieu loc sections.deleted_at IS NULL (bai cua chuong da xoa lam progress_percentage khong bao gio dat 100%%):\n%s", *sql)
	}
}
