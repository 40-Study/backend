package seeds

// QA vòng 2, lane A (A1, A6) — seed khoá demo trên Postgres THẬT, trong schema tạm
// (pgtest.IsolatedSchema) bị DROP khi test xong. Không có Postgres: Skip ở local, FAIL khi CI=true.

import (
	"testing"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"gorm.io/gorm"
	"study.com/v1/internal/database"
	"study.com/v1/internal/model"
	"study.com/v1/internal/testutil/pgtest"
)

func seedDemoCoursesOnce(t *testing.T, s *Seeder, users map[string]model.User) map[string]model.Course {
	t.Helper()
	categories, err := s.SeedDemoCategories()
	if err != nil {
		t.Fatalf("seed categories: %v", err)
	}
	tags, err := s.SeedDemoTags()
	if err != nil {
		t.Fatalf("seed tags: %v", err)
	}
	courses, err := s.SeedDemoCourses(users, categories, tags)
	if err != nil {
		t.Fatalf("seed courses: %v", err)
	}
	return courses
}

func countRows(t *testing.T, db *gorm.DB, m interface{}) int64 {
	t.Helper()
	var n int64
	if err := db.Model(m).Count(&n).Error; err != nil {
		t.Fatalf("đếm %T: %v", m, err)
	}
	return n
}

// TestSeedDemoCourses_Postgres_DurationKhopVideoVaSoReviewThat:
//   - A1: mọi content video demo khai duration = độ dài thật của video mẫu (5s), kể cả bản ghi đã
//     seed sai từ trước (backfill), để xem trọn video đạt ngưỡng 90%.
//   - A6: total_reviews/average_rating khớp bảng reviews, không còn 318/204/... ghi cứng.
//   - Chạy lại seed không nhân bản bài học/nội dung (idempotent).
func TestSeedDemoCourses_Postgres_DurationKhopVideoVaSoReviewThat(t *testing.T) {
	db := pgtest.IsolatedSchema(t, database.Migrate)
	s := NewSeeder(db)

	users := map[string]model.User{}
	for _, email := range []string{"teacher1@demo.com", "teacher2@demo.com", "student1@demo.com"} {
		u := model.User{Email: email, PasswordHash: "x", UserName: "qa-seed-" + uuid.NewString()[:8]}
		if err := db.Create(&u).Error; err != nil {
			t.Fatalf("tạo user %s: %v", email, err)
		}
		users[email] = u
	}

	courses := seedDemoCoursesOnce(t, s, users)
	lessonsBefore := countRows(t, db, &model.Lesson{})
	contentsBefore := countRows(t, db, &model.LessonContent{})

	// Mô phỏng DB dev đã seed bằng phiên bản cũ: duration khai theo phút, số review giả; thêm 1
	// review thật (5 sao) để kiểm tính lại đúng.
	if err := db.Model(&model.LessonContent{}).Where("type = ?", "video").Update("duration", 900).Error; err != nil {
		t.Fatalf("làm hỏng duration: %v", err)
	}
	if err := db.Model(&model.Course{}).Where("1 = 1").
		UpdateColumns(map[string]interface{}{"total_reviews": 318, "average_rating": 4.8}).Error; err != nil {
		t.Fatalf("làm hỏng rating: %v", err)
	}
	reviewed := courses["git-github-cho-nguoi-moi-bat-dau"]
	if err := db.Create(&model.Review{UserID: users["student1@demo.com"].ID, CourseID: reviewed.ID, Rating: 5}).Error; err != nil {
		t.Fatalf("tạo review: %v", err)
	}

	for run := 2; run <= 3; run++ {
		seedDemoCoursesOnce(t, s, users)

		var videos []model.LessonContent
		if err := db.Where("type = ?", "video").Find(&videos).Error; err != nil {
			t.Fatalf("đọc content: %v", err)
		}
		if len(videos) == 0 {
			t.Fatal("seed không tạo content video nào")
		}
		for _, v := range videos {
			if v.Duration != demoVideoDurationSeconds {
				t.Errorf("lần %d: content %s duration = %d, muốn %d (độ dài thật của video mẫu)", run, v.ID, v.Duration, demoVideoDurationSeconds)
			}
		}

		var all []model.Course
		if err := db.Find(&all).Error; err != nil {
			t.Fatalf("đọc course: %v", err)
		}
		for _, c := range all {
			wantTotal, wantAvg := 0, decimal.Zero
			if c.ID == reviewed.ID {
				wantTotal, wantAvg = 1, decimal.NewFromInt(5)
			}
			if c.TotalReviews != wantTotal || !c.AverageRating.Equal(wantAvg) {
				t.Errorf("lần %d: khoá %s total_reviews=%d average_rating=%s, muốn %d/%s (khớp bảng reviews)",
					run, c.Slug, c.TotalReviews, c.AverageRating, wantTotal, wantAvg)
			}
		}

		if n := countRows(t, db, &model.Lesson{}); n != lessonsBefore {
			t.Errorf("lần %d: số bài học %d, muốn %d — seed lại không được nhân bản", run, n, lessonsBefore)
		}
		if n := countRows(t, db, &model.LessonContent{}); n != contentsBefore {
			t.Errorf("lần %d: số content %d, muốn %d — seed lại không được nhân bản", run, n, contentsBefore)
		}
	}
}
