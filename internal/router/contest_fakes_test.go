package router

// Fake cho ranh giới lane B2 (contract §6) — đủ hành vi để test luồng B1 trên Postgres thật:
// engine tạo/chấm attempt mode "contest" trong quiz_attempts (nộp 2 lần → ErrQuizAttemptAlreadySubmitted
// nhờ UPDATE ... WHERE completed_at IS NULL, đúng cơ chế contract §4.2), issuer ghi user_vouchers
// BẰNG tx được truyền vào (để kiểm rollback toàn bộ khi voucher hỏng).

import (
	"context"
	"sort"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"gorm.io/gorm"
	"study.com/v1/internal/dto"
	"study.com/v1/internal/model"
	"study.com/v1/internal/service"
)

type fakeEngine struct{ db *gorm.DB }

func (f *fakeEngine) CreateContestAttemptTx(_ context.Context, tx *gorm.DB, quizID, userID uuid.UUID, startedAt time.Time) (uuid.UUID, error) {
	a := model.QuizAttempt{ID: uuid.New(), UserID: userID, QuizID: quizID, Mode: service.QuizAttemptModeContest, StartedAt: startedAt}
	return a.ID, tx.Create(&a).Error
}

func (f *fakeEngine) questions(quizID uuid.UUID) ([]model.Question, error) {
	var qs []model.Question
	err := f.db.Preload("Answers").Where("quiz_id = ?", quizID).Order("display_order").Find(&qs).Error
	return qs, err
}

func (f *fakeEngine) GetContestAttemptQuestions(_ context.Context, quizID, _ uuid.UUID) ([]dto.AttemptQuestionDTO, error) {
	qs, err := f.questions(quizID)
	out := []dto.AttemptQuestionDTO{}
	for _, q := range qs {
		item := dto.AttemptQuestionDTO{ID: q.ID, QuestionText: q.QuestionText, QuestionType: q.QuestionType, Points: q.Points, DisplayOrder: q.DisplayOrder}
		for _, a := range q.Answers {
			item.Answers = append(item.Answers, dto.AttemptAnswerDTO{ID: a.ID, AnswerText: a.AnswerText, DisplayOrder: a.DisplayOrder})
		}
		out = append(out, item)
	}
	return out, err
}

func correctIDs(q model.Question) []string {
	var ids []string
	for _, a := range q.Answers {
		if a.IsCorrect {
			ids = append(ids, a.ID.String())
		}
	}
	sort.Strings(ids)
	return ids
}

func sameSet(a, b []string) bool {
	a, b = append([]string(nil), a...), append([]string(nil), b...)
	sort.Strings(a)
	sort.Strings(b)
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func (f *fakeEngine) SubmitContestAttempt(_ context.Context, quizID, userID, attemptID uuid.UUID, answers []dto.SubmitAnswerDTO, maxSecs int) (*dto.QuizAttemptResponseDTO, error) {
	qs, err := f.questions(quizID)
	if err != nil {
		return nil, err
	}
	score, total := decimal.Zero, decimal.Zero
	for _, q := range qs {
		total = total.Add(q.Points)
		for _, a := range answers {
			if a.QuestionID == q.ID.String() && sameSet(a.SelectedAnswerIDs, correctIDs(q)) {
				score = score.Add(q.Points)
			}
		}
	}
	var att model.QuizAttempt
	if err := f.db.First(&att, "id = ?", attemptID).Error; err != nil {
		return nil, err
	}
	now := time.Now()
	spent := int(now.Sub(att.StartedAt).Seconds())
	if spent > maxSecs {
		spent = maxSecs
	}
	pct := decimal.Zero
	if total.IsPositive() {
		pct = score.Div(total).Mul(decimal.NewFromInt(100)).Round(2)
	}
	res := f.db.Model(&model.QuizAttempt{}).Where("id = ? AND user_id = ? AND completed_at IS NULL", attemptID, userID).
		Updates(map[string]interface{}{"completed_at": now, "score": score, "total_points": total, "percentage": pct, "time_spent_seconds": spent})
	if res.Error != nil {
		return nil, res.Error
	}
	if res.RowsAffected == 0 {
		return nil, service.ErrQuizAttemptAlreadySubmitted
	}
	return &dto.QuizAttemptResponseDTO{ID: attemptID, UserID: userID, QuizID: quizID, Score: &score, TotalPoints: &total,
		Percentage: &pct, TimeSpentSecs: &spent, StartedAt: att.StartedAt, CompletedAt: &now}, nil
}

func (f *fakeEngine) GetContestAttemptReview(_ context.Context, attemptID uuid.UUID) ([]dto.QuizAttemptAnswerDTO, error) {
	var att model.QuizAttempt
	if err := f.db.First(&att, "id = ?", attemptID).Error; err != nil {
		return nil, err
	}
	qs, err := f.questions(att.QuizID)
	out := []dto.QuizAttemptAnswerDTO{}
	for _, q := range qs {
		out = append(out, dto.QuizAttemptAnswerDTO{ID: uuid.New(), QuestionID: q.ID, QuestionText: q.QuestionText,
			CorrectAnswerIDs: correctIDs(q), Explanation: q.Explanation})
	}
	return out, err
}

type fakeIssuer struct {
	mu       sync.Mutex
	notified [][]service.ContestResultNotice
}

func (f *fakeIssuer) IssueAwardTx(_ context.Context, tx *gorm.DB, g service.ContestAwardGrant) (*service.ContestAwardIssued, error) {
	out := &service.ContestAwardIssued{}
	if g.VoucherID != nil {
		var n int64
		if err := tx.Model(&model.Voucher{}).Where("id = ? AND is_active = true", *g.VoucherID).Count(&n).Error; err != nil {
			return nil, err
		}
		if n == 0 {
			return nil, service.ErrVoucherUnavailableForGrant
		}
		uv := model.UserVoucher{ID: uuid.New(), UserID: g.UserID, VoucherID: *g.VoucherID, Source: "contest_reward", SavedAt: time.Now()}
		if err := tx.Create(&uv).Error; err != nil {
			return nil, err
		}
		out.UserVoucherID = &uv.ID
	}
	if g.GrantCertificate {
		num := "CONTEST-" + g.IssuedAt.Format("20060102") + "-" + uuid.NewString()[:8]
		out.CertificateNumber = &num
	}
	return out, nil
}

func (f *fakeIssuer) NotifyContestResults(_ context.Context, notices []service.ContestResultNotice) (int, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.notified = append(f.notified, notices)
	return len(notices), nil
}

func (f *fakeIssuer) notifyCalls() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.notified)
}
