package database

// Test Postgres THẬT cho backfill quizzes.created_by (quiz_created_by_backfill.go). Gọi qua
// RunPostMigrations — đúng đường chạy lúc API khởi động — nên bỏ lời gọi backfill khỏi
// RunPostMigrations thì test ĐỎ. Schema tạm riêng (pgtest.IsolatedSchema), DROP khi xong.

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"gorm.io/gorm"
	"study.com/v1/internal/model"
	"study.com/v1/internal/testutil/pgtest"
)

func backfillUser(t *testing.T, db *gorm.DB, kind string) uuid.UUID {
	t.Helper()
	s := uuid.NewString()
	u := model.User{Email: "qa-backfill-" + kind + "-" + s + "@40study.test", PasswordHash: "x", UserName: "QA-bf-" + kind + s[:8]}
	if err := db.Create(&u).Error; err != nil {
		t.Fatalf("tạo user: %v", err)
	}
	return u.ID
}

func backfillQuiz(t *testing.T, db *gorm.DB, q model.Quiz) uuid.UUID {
	t.Helper()
	q.Title, q.TriggerType = "QA backfill "+uuid.NewString()[:8], "manual"
	if err := db.Create(&q).Error; err != nil {
		t.Fatalf("tạo quiz: %v", err)
	}
	return q.ID
}

func createdByOf(t *testing.T, db *gorm.DB, quizID uuid.UUID) *uuid.UUID {
	t.Helper()
	var q model.Quiz
	if err := db.First(&q, "id = ?", quizID).Error; err != nil {
		t.Fatalf("đọc quiz: %v", err)
	}
	return q.CreatedBy
}

func TestQuizCreatedByBackfill_GanChuTheoQuanHe_ChayLaiAnToan(t *testing.T) {
	db := pgtest.IsolatedSchema(t, Migrate)
	teacher, host, contestOwner, realOwner := backfillUser(t, db, "teacher"), backfillUser(t, db, "host"),
		backfillUser(t, db, "contest"), backfillUser(t, db, "owner")

	course := model.Course{InstructorID: teacher, Title: "Khoá QA", Slug: "qa-bf-" + uuid.NewString(), Price: decimal.NewFromInt(1)}
	if err := db.Create(&course).Error; err != nil {
		t.Fatalf("tạo khoá: %v", err)
	}
	section := model.Section{CourseID: course.ID, Title: "Chương 1", DisplayOrder: 1}
	if err := db.Create(&section).Error; err != nil {
		t.Fatalf("tạo chương: %v", err)
	}
	lesson := model.Lesson{SectionID: section.ID, Title: "Bài 1", DisplayOrder: 1}
	if err := db.Create(&lesson).Error; err != nil {
		t.Fatalf("tạo bài: %v", err)
	}
	class := model.Class{Name: "Lớp QA"}
	if err := db.Create(&class).Error; err != nil {
		t.Fatalf("tạo lớp: %v", err)
	}
	session := model.LivestreamSession{Title: "Live QA", HostID: host, ClassID: class.ID, RoomName: "qa-bf-" + uuid.NewString()}
	if err := db.Create(&session).Error; err != nil {
		t.Fatalf("tạo buổi live: %v", err)
	}

	lessonQuiz := backfillQuiz(t, db, model.Quiz{LessonID: &lesson.ID})
	courseQuiz := backfillQuiz(t, db, model.Quiz{CourseID: &course.ID})
	sessionQuiz := backfillQuiz(t, db, model.Quiz{SessionID: &session.ID})
	contestQuiz := backfillQuiz(t, db, model.Quiz{})
	orphanQuiz := backfillQuiz(t, db, model.Quiz{})
	ownedQuiz := backfillQuiz(t, db, model.Quiz{LessonID: &lesson.ID, CreatedBy: &realOwner})

	// contests.quiz_id là cột của lane B1; trước khi B1 merge thì thêm tạm trong schema test.
	if err := db.Exec("ALTER TABLE contests ADD COLUMN IF NOT EXISTS quiz_id uuid").Error; err != nil {
		t.Fatalf("thêm contests.quiz_id: %v", err)
	}
	contest := model.Contest{Title: "Cuộc thi QA", Slug: "qa-bf-" + uuid.NewString(), Type: "QUIZ",
		StartTime: time.Now(), EndTime: time.Now().Add(time.Hour), CreatedBy: contestOwner}
	if err := db.Create(&contest).Error; err != nil {
		t.Fatalf("tạo cuộc thi: %v", err)
	}
	if err := db.Exec("UPDATE contests SET quiz_id = ? WHERE id = ?", contestQuiz, contest.ID).Error; err != nil {
		t.Fatalf("gắn quiz vào cuộc thi: %v", err)
	}

	want := map[string]struct {
		id    uuid.UUID
		owner *uuid.UUID
	}{
		"quiz bài học -> giảng viên khoá":  {lessonQuiz, &teacher},
		"quiz khoá học -> giảng viên khoá": {courseQuiz, &teacher},
		"quiz live -> host buổi live":      {sessionQuiz, &host},
		"quiz cuộc thi -> người tạo":       {contestQuiz, &contestOwner},
		"quiz không suy được chủ -> NULL":  {orphanQuiz, nil},
		"quiz đã có chủ -> giữ nguyên":     {ownedQuiz, &realOwner},
	}
	// Chạy 2 lần: lần 2 phải không đổi gì (idempotent, không ghi đè chủ đã có).
	for run := 1; run <= 2; run++ {
		if err := RunPostMigrations(db); err != nil {
			t.Fatalf("lần %d RunPostMigrations: %v", run, err)
		}
		for name, w := range want {
			got := createdByOf(t, db, w.id)
			switch {
			case w.owner == nil && got != nil:
				t.Errorf("lần %d, %s: muốn NULL, nhận %s", run, name, *got)
			case w.owner != nil && (got == nil || *got != *w.owner):
				t.Errorf("lần %d, %s: muốn %s, nhận %v", run, name, *w.owner, got)
			}
		}
	}
}
