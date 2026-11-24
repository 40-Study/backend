package seeds

// QA5 A17 / review M1 trên Postgres THẬT (schema tạm): bước gắn đề cho cuộc thi QUIZ đã kết thúc chỉ chạm cuộc thi do
// CHÍNH seed tạo (theo slug), không đụng cuộc thi admin/QA tạo, và quiz theo slug nên trùng tiêu đề không va
// idx_contests_quiz_id. Bỏ giới hạn slug hoặc đặt tên quiz theo tiêu đề thì test này ĐỎ.

import (
	"testing"
	"time"

	"study.com/v1/internal/database"
	"study.com/v1/internal/model"
	"study.com/v1/internal/testutil/pgtest"
)

func TestSeedDemoContests_Postgres_AttachQuizOnlyTouchesSeedOwnContests(t *testing.T) {
	db := pgtest.IsolatedSchema(t, database.Migrate)
	s := NewSeeder(db)
	users := laneADemoUsers(t, db, "admin@demo.com", "teacher1@demo.com", "teacher2@demo.com",
		"student1@demo.com", "student2@demo.com")
	if err := s.SeedDemoContests(users); err != nil {
		t.Fatalf("seed lần đầu: %v", err)
	}

	const ownSlug = "demo-thi-sql-da-ket-thuc"
	var own model.Contest
	if err := db.Where("slug = ?", ownSlug).First(&own).Error; err != nil {
		t.Fatalf("đọc %s: %v", ownSlug, err)
	}
	// Dòng seed mất đề (DB seed từ bản cũ) + hai cuộc thi do người khác tạo: một trùng TIÊU ĐỀ với dòng seed,
	// một tiêu đề khác. Cả hai đã công khai, đã kết thúc, không có đề.
	foreign := func(slug, title string) model.Contest {
		c := model.Contest{Slug: slug, Title: title, Type: model.ContestTypeQuiz, Status: model.ContestStatusPublished,
			StartTime: time.Now().AddDate(0, 0, -9), EndTime: time.Now().AddDate(0, 0, -8), IsPublic: true,
			CreatedBy: users["admin@demo.com"].ID}
		if err := db.Create(&c).Error; err != nil {
			t.Fatalf("tạo cuộc thi ngoài seed %s: %v", slug, err)
		}
		return c
	}
	sameTitle := foreign("qa-admin-trung-tieu-de", own.Title)
	otherTitle := foreign("qa-admin-khac-tieu-de", "Cuộc thi QA tạo tay")
	if err := db.Exec("UPDATE contests SET quiz_id = NULL WHERE id = ?", own.ID).Error; err != nil {
		t.Fatalf("bỏ đề của dòng seed: %v", err)
	}

	for run := 1; run <= 2; run++ {
		if err := s.SeedDemoContests(users); err != nil {
			t.Fatalf("lần %d: seed lại phải chạy được dù có cuộc thi trùng tiêu đề: %v", run, err)
		}
		var got model.Contest
		if err := db.First(&got, "id = ?", own.ID).Error; err != nil || got.QuizID == nil {
			t.Fatalf("lần %d: dòng seed %s phải được gắn lại đề, quiz_id=%v err=%v", run, ownSlug, got.QuizID, err)
		}
		var quiz model.Quiz
		if err := db.First(&quiz, "id = ?", *got.QuizID).Error; err != nil {
			t.Fatalf("lần %d: đọc quiz: %v", run, err)
		}
		if quiz.Title == "Đề thi: "+own.Title {
			t.Errorf("lần %d: quiz đặt theo tiêu đề %q (trùng tiêu đề sẽ va idx_contests_quiz_id), muốn theo slug", run, quiz.Title)
		}
		for _, c := range []model.Contest{sameTitle, otherTitle} {
			var after model.Contest
			if err := db.First(&after, "id = ?", c.ID).Error; err != nil {
				t.Fatalf("lần %d: đọc %s: %v", run, c.Slug, err)
			}
			if after.QuizID != nil {
				t.Errorf("lần %d: cuộc thi ngoài seed %s bị gắn đề %s", run, c.Slug, *after.QuizID)
			}
		}
	}
}
