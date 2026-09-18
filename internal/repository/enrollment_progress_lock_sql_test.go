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
