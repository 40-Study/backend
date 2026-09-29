package seeds

import (
	"fmt"
	"log"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"study.com/v1/internal/model"
)

// SeedDemoContests — MVP "Cuộc thi" (contract §1.7 bước 5): 1 cuộc thi đang diễn ra (không khoá
// khoá học) và 1 cuộc thi đã chốt có 2 người đạt giải, đều do teacher1 tạo, mỗi cuộc thi 1 quiz
// standalone riêng (quiz chỉ gắn được 1 cuộc thi). Idempotent theo slug/tiêu đề quiz.
func (s *Seeder) SeedDemoContests(users map[string]model.User) error {
	log.Println("Seeding demo contests...")
	teacher, admin := users["teacher1@demo.com"], users["admin@demo.com"]
	s1, s2 := users["student1@demo.com"], users["student2@demo.com"]
	if teacher.ID == uuid.Nil || admin.ID == uuid.Nil || s1.ID == uuid.Nil || s2.ID == uuid.Nil {
		return fmt.Errorf("demo contests: thiếu tài khoản demo teacher1/admin/student1/student2")
	}
	now := time.Now()

	activeQuiz, err := s.seedContestQuiz("Đề thi demo: Git cơ bản (đang diễn ra)", teacher.ID)
	if err != nil {
		return err
	}
	active := model.Contest{Slug: "demo-thi-git-dang-dien-ra", Title: "Thi nhanh Git cơ bản",
		Description: ptr("Cuộc thi demo đang diễn ra — 3 câu, 30 phút."), Type: model.ContestTypeQuiz,
		Status: model.ContestStatusPublished, DurationMinutes: ptr(30), IsPublic: true, CreatedBy: teacher.ID,
		QuizID: &activeQuiz, ReviewedBy: &admin.ID, ReviewedAt: ptr(now)}
	// Assign lịch mỗi lần seed để cuộc thi luôn ACTIVE (start = now-1h, end = now+7d).
	if err := s.db.Where("slug = ?", active.Slug).
		Assign(map[string]interface{}{"start_time": now.Add(-time.Hour), "end_time": now.AddDate(0, 0, 7)}).
		Attrs(active).FirstOrCreate(&active).Error; err != nil {
		return fmt.Errorf("seed active contest: %w", err)
	}
	if err := s.seedContestPrizes(active.ID); err != nil {
		return err
	}

	doneQuiz, err := s.seedContestQuiz("Đề thi demo: Git cơ bản (đã chốt)", teacher.ID)
	if err != nil {
		return err
	}
	start := now.AddDate(0, 0, -10)
	done := model.Contest{Slug: "demo-thi-git-da-chot", Title: "Thi Git tuần trước (đã có kết quả)",
		Description: ptr("Cuộc thi demo đã chốt kết quả."), Type: model.ContestTypeQuiz,
		Status: model.ContestStatusPublished, StartTime: start, EndTime: start.Add(24 * time.Hour),
		DurationMinutes: ptr(30), IsPublic: true, CreatedBy: teacher.ID, QuizID: &doneQuiz,
		ParticipantCount: 2, ReviewedBy: &admin.ID, ReviewedAt: ptr(start.Add(-time.Hour)),
		FinalizedAt: ptr(start.Add(48 * time.Hour)), FinalizedBy: &admin.ID}
	if err := s.db.Where("slug = ?", done.Slug).Attrs(done).FirstOrCreate(&done).Error; err != nil {
		return fmt.Errorf("seed finalized contest: %w", err)
	}
	if err := s.seedContestPrizes(done.ID); err != nil {
		return err
	}
	for i, u := range []model.User{s1, s2} {
		if err := s.seedContestWinner(done, u.ID, i+1, decimal.NewFromInt(int64(3-i))); err != nil {
			return err
		}
	}
	log.Println("Seeded 2 demo contests")
	return nil
}

func (s *Seeder) seedContestQuiz(title string, teacherID uuid.UUID) (uuid.UUID, error) {
	quiz := model.Quiz{Title: title, PassPercentage: rating(50), TriggerType: "manual"}
	if err := s.db.Where("title = ? AND lesson_id IS NULL AND course_id IS NULL", title).
		Attrs(quiz).FirstOrCreate(&quiz).Error; err != nil {
		return uuid.Nil, fmt.Errorf("seed contest quiz: %w", err)
	}
	// created_by đọc/ghi bằng SQL để không phụ thuộc field của model.Quiz (lane B2).
	if err := s.db.Exec("UPDATE quizzes SET created_by = ? WHERE id = ?", teacherID, quiz.ID).Error; err != nil {
		return uuid.Nil, fmt.Errorf("seed contest quiz owner: %w", err)
	}
	return quiz.ID, s.seedQuizQuestions(quiz.ID)
}

func (s *Seeder) seedContestPrizes(contestID uuid.UUID) error {
	var n int64
	if err := s.db.Model(&model.ContestPrize{}).Where("contest_id = ?", contestID).Count(&n).Error; err != nil || n > 0 {
		return err
	}
	return s.db.Create(&model.ContestPrize{ContestID: contestID, RankFrom: 1, RankTo: 3, GrantCertificate: true}).Error
}

// seedContestWinner — attempt đã nộp + participant có hạng + award có số chứng nhận.
func (s *Seeder) seedContestWinner(c model.Contest, userID uuid.UUID, rank int, score decimal.Decimal) error {
	var existing int64
	if err := s.db.Model(&model.ContestParticipant{}).Where("contest_id = ? AND user_id = ?", c.ID, userID).
		Count(&existing).Error; err != nil || existing > 0 {
		return err // existing > 0: đã seed
	}
	total := decimal.NewFromInt(3)
	pct := score.Div(total).Mul(decimal.NewFromInt(100)).Round(2)
	started := c.StartTime.Add(time.Duration(rank) * time.Minute)
	completed := started.Add(10 * time.Minute)
	attempt := model.QuizAttempt{UserID: userID, QuizID: *c.QuizID, Mode: "contest", Score: &score,
		TotalPoints: &total, Percentage: &pct, IsPassed: ptr(true), TimeSpentSecs: ptr(600),
		StartedAt: started, CompletedAt: &completed}
	if err := s.db.Create(&attempt).Error; err != nil {
		return fmt.Errorf("seed contest attempt: %w", err)
	}
	p := model.ContestParticipant{ContestID: c.ID, UserID: userID, AttemptID: &attempt.ID, Rank: &rank}
	if err := s.db.Create(&p).Error; err != nil {
		return fmt.Errorf("seed contest participant: %w", err)
	}
	cert := fmt.Sprintf("CONTEST-%s-%s", c.FinalizedAt.Format("20060102"), uuid.NewString()[:8])
	award := model.ContestAward{ContestID: c.ID, UserID: userID, Rank: &rank, CertificateNumber: &cert, IssuedAt: *c.FinalizedAt}
	if err := s.db.Create(&award).Error; err != nil {
		return fmt.Errorf("seed contest award: %w", err)
	}
	return nil
}
