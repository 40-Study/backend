package database

// Test Postgres THẬT cho runExerciseSubmissionPendingCleanup (exercise_submission_pending_cleanup.go).
// Bỏ lời gọi trong RunPostMigrations, bỏ lọc verdict='pending', đổi UPDATE thành DELETE, hoặc chạy lại
// mỗi lần khởi động thì test ĐỎ.

import (
	"strings"
	"testing"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"study.com/v1/internal/model"
	"study.com/v1/internal/testutil/pgtest"
)

func pendingCleanupSubmission(t *testing.T, db *gorm.DB, exerciseID, userID uuid.UUID, verdict string) uuid.UUID {
	t.Helper()
	sub := model.ExerciseSubmission{ExerciseID: exerciseID, UserID: userID, Language: "python", Code: "print(1)", Verdict: verdict}
	if err := db.Create(&sub).Error; err != nil {
		t.Fatalf("tạo bài nộp %s: %v", verdict, err)
	}
	return sub.ID
}

func pendingCleanupRow(t *testing.T, db *gorm.DB, id uuid.UUID) model.ExerciseSubmission {
	t.Helper()
	var sub model.ExerciseSubmission
	if err := db.First(&sub, "id = ?", id).Error; err != nil {
		t.Fatalf("đọc bài nộp %s: %v", id, err)
	}
	return sub
}

func TestExerciseSubmissionPendingCleanup_MarksLegacyPendingOnceNeverDeletes(t *testing.T) {
	db := pgtest.IsolatedSchema(t, Migrate)

	// Wired: Migrate -> RunPostMigrations phải đã chạy bước này và ghi dấu.
	var marked int64
	if err := db.Raw("SELECT count(*) FROM data_migrations WHERE name = ?", exerciseSubmissionPendingCleanupName).Scan(&marked).Error; err != nil {
		t.Fatalf("đọc data_migrations: %v", err)
	}
	if marked != 1 {
		t.Fatalf("RunPostMigrations chưa gọi runExerciseSubmissionPendingCleanup (dấu = %d)", marked)
	}
	// Mô phỏng DB cũ chưa chạy bước này.
	if err := db.Exec("DELETE FROM data_migrations WHERE name = ?", exerciseSubmissionPendingCleanupName).Error; err != nil {
		t.Fatalf("xoá dấu: %v", err)
	}

	student := ghostUser(t, db, "exsub", "")
	ex := model.CourseExercise{Title: "QA-be2 ex", Description: "d", Language: []string{"python"}}
	if err := db.Create(&ex).Error; err != nil {
		t.Fatalf("tạo exercise: %v", err)
	}
	legacy := pendingCleanupSubmission(t, db, ex.ID, student.ID, "pending")
	accepted := pendingCleanupSubmission(t, db, ex.ID, student.ID, "accepted")
	wrong := pendingCleanupSubmission(t, db, ex.ID, student.ID, "wrong_answer")
	var total int64
	db.Model(&model.ExerciseSubmission{}).Count(&total)
	buf := captureLog(t)

	if err := runExerciseSubmissionPendingCleanup(db); err != nil {
		t.Fatalf("lần 1: %v", err)
	}
	got := pendingCleanupRow(t, db, legacy)
	if got.Verdict != "system_error" || got.ErrorMessage == nil || *got.ErrorMessage != exerciseSubmissionPendingCleanupReason {
		t.Fatalf("dòng pending cũ phải thành system_error kèm lý do, nhận verdict=%q msg=%v", got.Verdict, got.ErrorMessage)
	}
	if got.Code != "print(1)" {
		t.Fatalf("không được mất mã nguồn học viên, nhận %q", got.Code)
	}
	for id, want := range map[uuid.UUID]string{accepted: "accepted", wrong: "wrong_answer"} {
		if v := pendingCleanupRow(t, db, id).Verdict; v != want {
			t.Errorf("bài đã chấm %s bị đổi verdict: %q, muốn %q", id, v, want)
		}
	}
	var after int64
	db.Model(&model.ExerciseSubmission{}).Count(&after)
	if after != total {
		t.Fatalf("dọn không được xoá dòng nào: %d -> %d", total, after)
	}
	if out := buf.String(); !strings.Contains(out, legacy.String()) || !strings.Contains(out, "marked 1 ") ||
		strings.Contains(out, accepted.String()) || strings.Contains(out, wrong.String()) {
		t.Fatalf("log phải ghi đúng id đã đổi và số lượng 1; log = %q", out)
	}

	// Chạy lại (khởi động sau): đã có dấu nên không chạm dòng nào nữa.
	later := pendingCleanupSubmission(t, db, ex.ID, student.ID, "pending")
	if err := runExerciseSubmissionPendingCleanup(db); err != nil {
		t.Fatalf("lần 2: %v", err)
	}
	if v := pendingCleanupRow(t, db, later).Verdict; v != "pending" {
		t.Fatalf("bước dọn chỉ được chạy một lần mỗi DB; lần 2 đổi verdict thành %q", v)
	}
}
