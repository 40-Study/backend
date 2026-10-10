package database

// Phase 8 (plan 261008), "wired, not just present": các bước của lane chỉ có tác dụng khi
// Migrate/RunPostMigrations THẬT gọi chúng. Test gọi chính hàm đó, không gọi từng bước riêng lẻ:
//   - bảng audit_logs được AutoMigrate tạo
//   - RunPostMigrations nới chk_lesson_contents_type trên DB cũ (article/quiz ghi được)
//   - RunPostMigrations chạy backfill quiz và dọn yêu cầu liên kết "ma" trên DB cũ

import (
	"strings"
	"testing"

	"github.com/google/uuid"
	"study.com/v1/internal/model"
	"study.com/v1/internal/testutil/pgtest"
)

func TestMigrate_CreatesAuditLogsTable(t *testing.T) {
	db := pgtest.IsolatedSchema(t, Migrate)
	if !db.Migrator().HasTable(&model.AuditLog{}) {
		t.Fatal("Migrate không tạo bảng audit_logs: thiếu &model.AuditLog{} trong AutoMigrate")
	}
}

func TestRunPostMigrations_WidensLessonContentCheckOnOldDB(t *testing.T) {
	db := pgtest.IsolatedSchema(t, Migrate)
	f := newLessonFixture(t, db)
	lesson := f.lesson("bài")
	insert := func(typ string) error {
		return db.Exec(`INSERT INTO lesson_contents (id, lesson_id, type) VALUES (gen_random_uuid(), ?, ?)`, lesson, typ).Error
	}

	if err := db.Exec(`ALTER TABLE lesson_contents DROP CONSTRAINT chk_lesson_contents_type;
		ALTER TABLE lesson_contents ADD CONSTRAINT chk_lesson_contents_type CHECK (type IN ('video','livestream','exercise'))`).Error; err != nil {
		t.Fatalf("giả lập DB cũ: %v", err)
	}
	if err := insert("quiz"); err == nil {
		t.Fatal("DB cũ phải từ chối 'quiz' trước RunPostMigrations (nếu không, test không chứng minh gì)")
	}

	if err := RunPostMigrations(db); err != nil {
		t.Fatalf("RunPostMigrations: %v", err)
	}
	for _, typ := range []string{"article", "quiz"} {
		if err := insert(typ); err != nil {
			t.Errorf("sau RunPostMigrations, type %q phải ghi được: %v", typ, err)
		}
	}
	if err := insert("bogus"); err == nil {
		t.Error("loại lạ vẫn phải bị CHK từ chối")
	}
}

func TestRunPostMigrations_RunsQuizBackfillAndGhostCleanupOnOldDB(t *testing.T) {
	db := pgtest.IsolatedSchema(t, Migrate)
	f := newLessonFixture(t, db)
	lesson := f.lesson("bài có quiz mồ côi")
	orphan := f.quiz(&lesson, nil, "Quiz mồ côi")

	parent := ghostUser(t, db, "parent", "PARENT")
	ghost := ghostRequest(t, db, parent.ID, "khong-ton-tai-"+strings.ToLower(uuid.NewString()[:8])+"@40study.test", "pending", nil)

	// DB cũ: backfill và dọn dòng ma chưa từng chạy (Migrate ở đầu test đã ghi dấu cả hai trên DB trống).
	if err := db.Exec("DELETE FROM data_migrations WHERE name IN (?, ?)", lessonContentQuizBackfillName, parentLinkGhostCleanupName).Error; err != nil {
		t.Fatalf("xoá dấu backfill/dọn dòng ma: %v", err)
	}
	if err := RunPostMigrations(db); err != nil {
		t.Fatalf("RunPostMigrations: %v", err)
	}

	rows := f.contentsOf(lesson)
	if len(rows) != 1 || rows[0].QuizID == nil || *rows[0].QuizID != orphan || rows[0].Type != "quiz" {
		t.Errorf("RunPostMigrations phải backfill đúng 1 dòng quiz cho quiz mồ côi, nhận %+v", rows)
	}
	if got := statusOf(t, db, ghost); got != "cancelled" {
		t.Errorf("RunPostMigrations phải huỷ yêu cầu liên kết ma, status = %q", got)
	}
}
