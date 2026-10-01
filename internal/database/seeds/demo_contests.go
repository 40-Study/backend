package seeds

import (
	"fmt"
	"log"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"study.com/v1/internal/model"
)

// SeedDemoContests seed dữ liệu cho mọi tab của trang "Cuộc thi" trên web:
//   - Danh sách công khai (GET /contests, chỉ PUBLISHED + is_public): 2 UPCOMING, 1 ACTIVE, 1 ENDED
//     (chưa chốt), 1 FINALIZED — phase tính từ thời gian (model.ContestPhaseAt).
//   - Hàng chờ duyệt của admin: 1 PENDING_REVIEW; trang giáo viên của teacher2: 1 DRAFT.
//   - "Cuộc thi của tôi" (/contests/me, theo contest_participants): student1 ở UPCOMING, ACTIVE,
//     ENDED và FINALIZED.
//
// Mỗi cuộc thi có quiz standalone riêng (quiz_id unique, FK RESTRICT) với câu hỏi thật.
// Idempotent theo slug cuộc thi / tiêu đề quiz / cặp (contest, user).
func (s *Seeder) SeedDemoContests(users map[string]model.User) error {
	log.Println("Seeding demo contests...")
	teacher, admin := users["teacher1@demo.com"], users["admin@demo.com"]
	s1, s2 := users["student1@demo.com"], users["student2@demo.com"]
	if teacher.ID == uuid.Nil || admin.ID == uuid.Nil || s1.ID == uuid.Nil || s2.ID == uuid.Nil {
		return fmt.Errorf("demo contests: thiếu tài khoản demo teacher1/admin/student1/student2")
	}
	now := time.Now()

	activeQuiz, err := s.seedContestQuiz("Đề thi demo: Git cơ bản (đang diễn ra)", teacher.ID, demoQuizQuestions)
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
	if err := s.seedContestPrizeRanges(active.ID, []contestPrizeSpec{{1, 3}}); err != nil {
		return err
	}

	doneQuiz, err := s.seedContestQuiz("Đề thi demo: Git cơ bản (đã chốt)", teacher.ID, demoQuizQuestions)
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
	if err := s.seedContestPrizeRanges(done.ID, []contestPrizeSpec{{1, 3}}); err != nil {
		return err
	}
	for i, u := range []model.User{s1, s2} {
		if err := s.seedContestWinner(done, u.ID, i+1, decimal.NewFromInt(int64(3-i))); err != nil {
			return err
		}
	}

	bySlug := map[string]model.Contest{active.Slug: active, done.Slug: done}
	for _, spec := range demoExtraContests {
		c, err := s.upsertDemoContest(spec, users, admin.ID, now)
		if err != nil {
			return err
		}
		bySlug[c.Slug] = c
	}
	if err := s.seedDemoContestJoins(bySlug, users); err != nil {
		return err
	}
	log.Printf("Seeded %d demo contests\n", len(bySlug))
	return nil
}

// upsertDemoContest tạo (hoặc tìm theo slug) một cuộc thi từ spec, kèm quiz và giải thưởng.
func (s *Seeder) upsertDemoContest(spec contestSeedSpec, users map[string]model.User, adminID uuid.UUID, now time.Time) (model.Contest, error) {
	creator := users[spec.CreatorEmail]
	if creator.ID == uuid.Nil {
		return model.Contest{}, fmt.Errorf("demo contest %s: thiếu tài khoản %s", spec.Slug, spec.CreatorEmail)
	}
	quizID, err := s.seedContestQuiz(spec.QuizTitle, creator.ID, spec.Questions)
	if err != nil {
		return model.Contest{}, err
	}
	startAt, endAt := now.Add(spec.StartOffset), now.Add(spec.EndOffset)
	c := model.Contest{Slug: spec.Slug, Title: spec.Title, Description: ptr(spec.Description),
		Type: model.ContestTypeQuiz, Status: spec.Status, StartTime: startAt, EndTime: endAt,
		DurationMinutes: ptr(spec.DurationMinutes), MaxParticipants: spec.MaxParticipants, IsPublic: true,
		CreatedBy: creator.ID, QuizID: &quizID, CertificateMinPercentage: ptr(decimal.NewFromInt(50))}
	switch spec.Status {
	case model.ContestStatusPublished:
		c.SubmittedAt, c.ReviewedBy, c.ReviewedAt = ptr(startAt.Add(-48*time.Hour)), &adminID, ptr(startAt.Add(-24*time.Hour))
	case model.ContestStatusPendingReview:
		c.SubmittedAt = ptr(now.Add(-2 * time.Hour))
	}
	q := s.db.Where("slug = ?", spec.Slug)
	if spec.ReassignWindow {
		q = q.Assign(map[string]interface{}{"start_time": startAt, "end_time": endAt})
	}
	if err := q.Attrs(c).FirstOrCreate(&c).Error; err != nil {
		return model.Contest{}, fmt.Errorf("seed contest %s: %w", spec.Slug, err)
	}
	return c, s.seedContestPrizeRanges(c.ID, spec.Prizes)
}

// seedContestQuiz tạo quiz standalone (không gắn lesson/course) cho một cuộc thi, tra theo tiêu đề.
func (s *Seeder) seedContestQuiz(title string, teacherID uuid.UUID, questions []demoQuestionSpec) (uuid.UUID, error) {
	quiz := model.Quiz{Title: title, PassPercentage: rating(50), TriggerType: "manual"}
	if err := s.db.Where("title = ? AND lesson_id IS NULL AND course_id IS NULL", title).
		Attrs(quiz).FirstOrCreate(&quiz).Error; err != nil {
		return uuid.Nil, fmt.Errorf("seed contest quiz: %w", err)
	}
	// created_by đọc/ghi bằng SQL để không phụ thuộc field của model.Quiz (lane B2).
	if err := s.db.Exec("UPDATE quizzes SET created_by = ? WHERE id = ?", teacherID, quiz.ID).Error; err != nil {
		return uuid.Nil, fmt.Errorf("seed contest quiz owner: %w", err)
	}
	return quiz.ID, s.seedContestQuestionSet(quiz.ID, questions)
}

// seedContestQuestionSet ghi câu hỏi/đáp án theo thứ tự trong spec (display_order = vị trí + 1
// khi spec không tự khai Order). Tra theo (quiz_id, display_order) nên chạy lại không nhân bản.
func (s *Seeder) seedContestQuestionSet(quizID uuid.UUID, specs []demoQuestionSpec) error {
	for i, q := range specs {
		order := q.Order
		if order == 0 {
			order = i + 1
		}
		question := model.Question{QuizID: quizID, QuestionText: q.Text, QuestionType: q.Type,
			Points: rating(1), DisplayOrder: order}
		if err := s.db.Where("quiz_id = ? AND display_order = ?", quizID, order).
			Attrs(question).FirstOrCreate(&question).Error; err != nil {
			return fmt.Errorf("seed contest question %d: %w", order, err)
		}
		for j, a := range q.Answers {
			aOrder := a.Order
			if aOrder == 0 {
				aOrder = j + 1
			}
			answer := model.QuestionAnswer{QuestionID: question.ID, AnswerText: a.Text, IsCorrect: a.IsCorrect, DisplayOrder: aOrder}
			if err := s.db.Where("question_id = ? AND display_order = ?", question.ID, aOrder).
				Attrs(answer).FirstOrCreate(&answer).Error; err != nil {
				return fmt.Errorf("seed contest answer %d: %w", aOrder, err)
			}
		}
	}
	return nil
}

// seedContestPrizeRanges chỉ ghi giải khi cuộc thi chưa có giải nào (không đè giải admin đã sửa).
func (s *Seeder) seedContestPrizeRanges(contestID uuid.UUID, ranges []contestPrizeSpec) error {
	var n int64
	if err := s.db.Model(&model.ContestPrize{}).Where("contest_id = ?", contestID).Count(&n).Error; err != nil || n > 0 {
		return err
	}
	for _, r := range ranges {
		p := model.ContestPrize{ContestID: contestID, RankFrom: r.From, RankTo: r.To, GrantCertificate: true}
		if err := s.db.Create(&p).Error; err != nil {
			return fmt.Errorf("seed contest prize: %w", err)
		}
	}
	return nil
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
