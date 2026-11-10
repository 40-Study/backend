package seeds

import (
	"fmt"
	"log"

	"github.com/lib/pq"
	"github.com/shopspring/decimal"
	"gorm.io/gorm"
	"study.com/v1/internal/model"
)

// demoContestAnswers dựng đáp án từng câu cho một bài thi demo: `correct` câu đầu chọn đủ đáp án đúng, các câu sau chọn
// MỘT đáp án sai. Điểm và tổng điểm tính từ chính các dòng này (cùng cách chấm của QuizService.gradeSubmission), để điểm
// trên thẻ kết quả luôn bằng tổng điểm từng câu ở phần "Đáp án và giải thích" (QA-A-03: attempt có score 2 mà không có
// dòng đáp án nào, trang kết quả ghi "Bạn bỏ trống câu này" cho mọi câu). AttemptID để trống cho bên gọi gán.
func demoContestAnswers(questions []model.Question, correct int) ([]model.QuizAttemptAnswer, decimal.Decimal, decimal.Decimal) {
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
	return answers, score, total
}

// demoCorrectCountForScore tìm k sao cho tổng điểm k câu đầu đúng bằng score — để vá đáp án cho attempt cũ đã có điểm
// mà không đổi điểm của nó. Không có k khớp thì trả false (bên gọi bỏ qua thay vì bịa đáp án lệch điểm).
func demoCorrectCountForScore(questions []model.Question, score decimal.Decimal) (int, bool) {
	sum := decimal.Zero
	if sum.Equal(score) {
		return 0, true
	}
	for i, q := range questions {
		sum = sum.Add(q.Points)
		if sum.Equal(score) {
			return i + 1, true
		}
	}
	return 0, false
}

// loadContestQuestions đọc câu hỏi (kèm đáp án) của quiz cuộc thi theo đúng thứ tự hiển thị.
func (s *Seeder) loadContestQuestions(c model.Contest) ([]model.Question, error) {
	var questions []model.Question
	byOrder := func(db *gorm.DB) *gorm.DB { return db.Order("display_order") }
	if err := s.db.Preload("Answers", byOrder).Where("quiz_id = ?", *c.QuizID).Order("display_order").
		Find(&questions).Error; err != nil {
		return nil, fmt.Errorf("load contest questions: %w", err)
	}
	if len(questions) == 0 {
		return nil, fmt.Errorf("cuộc thi %s chưa có câu hỏi", c.Slug)
	}
	return questions, nil
}

// backfillContestAttemptAnswers vá DB đã seed bằng bản cũ: attempt của người đoạt giải có score nhưng 0 dòng đáp án.
// Seed idempotent bỏ qua participant đã có, nên nếu không vá ở đây thì chạy lại seed cũng không sửa được QA-A-03.
// Chỉ thêm đáp án khi attempt CHƯA có dòng nào và điểm cũ khớp được với k câu đầu; không đổi score/rank/giải.
func (s *Seeder) backfillContestAttemptAnswers(c model.Contest, p model.ContestParticipant) error {
	if p.AttemptID == nil {
		return nil
	}
	var n int64
	if err := s.db.Model(&model.QuizAttemptAnswer{}).Where("attempt_id = ?", *p.AttemptID).Count(&n).Error; err != nil || n > 0 {
		return err
	}
	var attempt model.QuizAttempt
	if err := s.db.Where("id = ?", *p.AttemptID).Limit(1).Find(&attempt).Error; err != nil || attempt.Score == nil {
		return err
	}
	questions, err := s.loadContestQuestions(c)
	if err != nil {
		return err
	}
	correct, ok := demoCorrectCountForScore(questions, *attempt.Score)
	if !ok {
		log.Printf("WARN seed: attempt %s (score %s) không khớp tổng điểm k câu đầu, bỏ qua vá đáp án", attempt.ID, attempt.Score)
		return nil
	}
	answers, _, _ := demoContestAnswers(questions, correct)
	for i := range answers {
		answers[i].AttemptID = attempt.ID
	}
	if err := s.db.Create(&answers).Error; err != nil {
		return fmt.Errorf("backfill contest attempt answers %s: %w", c.Slug, err)
	}
	return nil
}
