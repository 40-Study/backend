package seeds

import (
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/lib/pq"
	"github.com/shopspring/decimal"
	"gorm.io/gorm"
	"study.com/v1/internal/model"
)

// seedDemoContestJoins ghi các lượt đăng ký trong demoContestJoins để tab "Cuộc thi của tôi"
// (/contests/me) và bảng xếp hạng cuộc thi ENDED có dữ liệu, rồi đồng bộ participant_count với
// số dòng contest_participants thật (luồng join thật tăng bộ đếm này nguyên tử).
func (s *Seeder) seedDemoContestJoins(contests map[string]model.Contest, users map[string]model.User) error {
	touched := map[uuid.UUID]bool{}
	for i, j := range demoContestJoins {
		c, found := contests[j.Slug]
		u := users[j.Email]
		if !found || u.ID == uuid.Nil {
			return fmt.Errorf("demo contest join: thiếu cuộc thi %s hoặc user %s", j.Slug, j.Email)
		}
		if err := s.seedContestJoin(c, u.ID, j.Correct, i); err != nil {
			return err
		}
		touched[c.ID] = true
	}
	for id := range touched {
		if err := s.db.Exec(`UPDATE contests SET participant_count =
			(SELECT COUNT(*) FROM contest_participants cp WHERE cp.contest_id = contests.id) WHERE id = ?`, id).Error; err != nil {
			return fmt.Errorf("sync participant_count: %w", err)
		}
	}
	return nil
}

// seedContestJoin: correct < 0 → chỉ đăng ký (NOT_STARTED, giống POST join thật: attempt_id NULL);
// correct >= 0 → kèm bài làm đã nộp với đáp án từng câu. Đã có participant thì bỏ qua.
func (s *Seeder) seedContestJoin(c model.Contest, userID uuid.UUID, correct, seq int) error {
	var existing int64
	if err := s.db.Model(&model.ContestParticipant{}).Where("contest_id = ? AND user_id = ?", c.ID, userID).
		Count(&existing).Error; err != nil || existing > 0 {
		return err
	}
	p := model.ContestParticipant{ContestID: c.ID, UserID: userID}
	if correct >= 0 {
		attemptID, err := s.seedContestSubmittedAttempt(c, userID, correct, seq)
		if err != nil {
			return err
		}
		p.AttemptID = &attemptID
	}
	if err := s.db.Create(&p).Error; err != nil {
		return fmt.Errorf("seed contest participant %s: %w", c.Slug, err)
	}
	return nil
}

// seedContestSubmittedAttempt tạo quiz_attempt mode "contest" nằm TRONG cửa sổ thi và trong
// thời lượng làm bài, cùng quiz_attempt_answers: `correct` câu đầu chọn đúng đáp án, các câu sau
// chọn một đáp án sai — điểm/phần trăm tính từ chính các câu trả lời này.
func (s *Seeder) seedContestSubmittedAttempt(c model.Contest, userID uuid.UUID, correct, seq int) (uuid.UUID, error) {
	var questions []model.Question
	byOrder := func(db *gorm.DB) *gorm.DB { return db.Order("display_order") }
	if err := s.db.Preload("Answers", byOrder).Where("quiz_id = ?", *c.QuizID).Order("display_order").
		Find(&questions).Error; err != nil {
		return uuid.Nil, fmt.Errorf("load contest questions: %w", err)
	}
	if len(questions) == 0 {
		return uuid.Nil, fmt.Errorf("cuộc thi %s chưa có câu hỏi", c.Slug)
	}
	started := c.StartTime.Add(time.Duration(5+seq) * time.Minute)
	spent := 12*60 + seq*37
	completed := started.Add(time.Duration(spent) * time.Second)

	score, total := decimal.Zero, decimal.Zero
	answers := make([]model.QuizAttemptAnswer, 0, len(questions))
	for i, q := range questions {
		total = total.Add(q.Points)
		wantRight := i < correct
		var picked pq.StringArray
		for _, a := range q.Answers {
			if a.IsCorrect == wantRight {
				picked = append(picked, a.ID.String())
				if !wantRight {
					break // câu sai: chỉ chọn một đáp án sai
				}
			}
		}
		earned := decimal.Zero
		if wantRight {
			earned = q.Points
			score = score.Add(earned)
		}
		answers = append(answers, model.QuizAttemptAnswer{QuestionID: q.ID, SelectedAnswerIDs: picked,
			IsCorrect: ptr(wantRight), PointsEarned: earned})
	}
	pct := score.Div(total).Mul(decimal.NewFromInt(100)).Round(2)
	attempt := model.QuizAttempt{UserID: userID, QuizID: *c.QuizID, Mode: "contest", Score: &score,
		TotalPoints: &total, Percentage: &pct, IsPassed: ptr(pct.GreaterThanOrEqual(decimal.NewFromInt(50))),
		TimeSpentSecs: ptr(spent), StartedAt: started, CompletedAt: &completed}
	if err := s.db.Create(&attempt).Error; err != nil {
		return uuid.Nil, fmt.Errorf("seed contest attempt %s: %w", c.Slug, err)
	}
	for i := range answers {
		answers[i].AttemptID = attempt.ID
	}
	if err := s.db.Create(&answers).Error; err != nil {
		return uuid.Nil, fmt.Errorf("seed contest attempt answers %s: %w", c.Slug, err)
	}
	return attempt.ID, nil
}
