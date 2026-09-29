package service

// Fixture dùng chung cho test tích hợp Postgres của lane B2 "Cuộc thi" (khoá quiz, engine, phát
// thưởng). Dữ liệu COMMIT trong schema tạm (pgtest.IsolatedSchema, migrateLikeAPIBoot khai báo ở
// withdrawal_service_postgres_test.go) vì các test race cần nhiều kết nối thấy dữ liệu của nhau.

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"gorm.io/gorm"
	"study.com/v1/internal/model"
	"study.com/v1/internal/repository"
	"study.com/v1/internal/testutil/pgtest"
)

type contestFixture struct {
	t    *testing.T
	db   *gorm.DB
	quiz *QuizService
}

func newContestFixture(t *testing.T) *contestFixture {
	t.Helper()
	db := pgtest.IsolatedSchema(t, migrateLikeAPIBoot)
	svc := NewQuizService(repository.NewQuizRepository(db), nil, repository.NewCourseRepository(db),
		repository.NewSectionRepository(db), repository.NewLessonRepository(db), nil, repository.NewEnrollmentRepository(db))
	return &contestFixture{t: t, db: db, quiz: svc}
}

func (f *contestFixture) user(kind string) uuid.UUID {
	f.t.Helper()
	s := uuid.NewString()
	u := model.User{Email: "qa-contest-" + kind + "-" + s + "@40study.test", PasswordHash: "x", UserName: "QA-contest-" + kind + "-" + s[:8]}
	if err := f.db.Create(&u).Error; err != nil {
		f.t.Fatalf("tạo user: %v", err)
	}
	return u.ID
}

// contestQuiz lưu các id cần để nộp bài đúng/sai trong test.
type contestQuiz struct {
	ID            uuid.UUID
	SingleQ       uuid.UUID // single_choice, 1 điểm
	SingleCorrect uuid.UUID
	SingleWrong   uuid.UUID
	MultiQ        uuid.UUID // multiple_choice, 2 điểm
	MultiCorrect  []uuid.UUID
	FillQ         uuid.UUID // fill_blank, 1 điểm, đáp án "Hà Nội"
	TrueFalseQ    uuid.UUID // true_false, 1 điểm
	TrueFalseTrue uuid.UUID
}

// standaloneQuiz tạo quiz đứng riêng (không lesson/course/session) — đúng loại quiz gắn được vào
// cuộc thi (contract §3.3) — gồm 4 câu, tổng 5 điểm.
func (f *contestFixture) standaloneQuiz(creator uuid.UUID, shuffle bool) contestQuiz {
	f.t.Helper()
	q := model.Quiz{Title: "QA contest quiz " + uuid.NewString()[:8], TriggerType: "manual", CreatedBy: &creator,
		ShuffleQuestions: shuffle, ShuffleAnswers: shuffle}
	if err := f.db.Create(&q).Error; err != nil {
		f.t.Fatalf("tạo quiz: %v", err)
	}
	// ShuffleQuestions/ShuffleAnswers có default:true ở DB — GORM bỏ qua zero-value false khi
	// Create, nên phải ghi lại tường minh.
	if err := f.db.Model(&model.Quiz{}).Where("id = ?", q.ID).
		Updates(map[string]interface{}{"shuffle_questions": shuffle, "shuffle_answers": shuffle}).Error; err != nil {
		f.t.Fatalf("đặt shuffle: %v", err)
	}
	out := contestQuiz{ID: q.ID}
	explain := "Giải thích bí mật"
	mk := func(order int, typ string, points float64, answers []model.QuestionAnswer) (uuid.UUID, []model.QuestionAnswer) {
		question := model.Question{QuizID: q.ID, QuestionText: "Câu " + typ, QuestionType: typ,
			Points: decimal.NewFromFloat(points), DisplayOrder: order, Explanation: &explain}
		if err := f.db.Create(&question).Error; err != nil {
			f.t.Fatalf("tạo câu hỏi: %v", err)
		}
		for i := range answers {
			answers[i].QuestionID = question.ID
			answers[i].DisplayOrder = i + 1
		}
		if err := f.db.Create(&answers).Error; err != nil {
			f.t.Fatalf("tạo đáp án: %v", err)
		}
		return question.ID, answers
	}
	var ans []model.QuestionAnswer
	out.SingleQ, ans = mk(1, "single_choice", 1, []model.QuestionAnswer{{AnswerText: "Đúng", IsCorrect: true}, {AnswerText: "Sai"}})
	out.SingleCorrect, out.SingleWrong = ans[0].ID, ans[1].ID
	out.MultiQ, ans = mk(2, "multiple_choice", 2, []model.QuestionAnswer{{AnswerText: "X", IsCorrect: true}, {AnswerText: "Y", IsCorrect: true}, {AnswerText: "Z"}})
	out.MultiCorrect = []uuid.UUID{ans[0].ID, ans[1].ID}
	out.FillQ, _ = mk(3, "fill_blank", 1, []model.QuestionAnswer{{AnswerText: "Hà Nội", IsCorrect: true}})
	out.TrueFalseQ, ans = mk(4, "true_false", 1, []model.QuestionAnswer{{AnswerText: "Đúng", IsCorrect: true}, {AnswerText: "Sai"}})
	out.TrueFalseTrue = ans[0].ID
	return out
}

func (f *contestFixture) count(table, where string, args ...interface{}) int64 {
	f.t.Helper()
	var n int64
	if err := f.db.Table(table).Where(where, args...).Count(&n).Error; err != nil {
		f.t.Fatalf("đếm %s: %v", table, err)
	}
	return n
}

// startContestAttempt tạo attempt "contest" trong transaction riêng rồi commit.
func (f *contestFixture) startContestAttempt(quizID, userID uuid.UUID) uuid.UUID {
	f.t.Helper()
	var id uuid.UUID
	err := f.db.Transaction(func(tx *gorm.DB) error {
		var err error
		id, err = f.quiz.CreateContestAttemptTx(context.Background(), tx, quizID, userID, time.Now())
		return err
	})
	if err != nil {
		f.t.Fatalf("CreateContestAttemptTx: %v", err)
	}
	return id
}

// fakeContestGate mô phỏng ContestService (lane B1) cho luật §3.2: quiz trong locked chỉ người tạo
// cuộc thi và admin được đụng; editable=false = cuộc thi PENDING_REVIEW/PUBLISHED/CANCELLED.
type fakeContestGate struct {
	locked map[uuid.UUID]fakeContestLock
}

type fakeContestLock struct {
	creator  uuid.UUID
	editable bool
}

func (g *fakeContestGate) CheckQuizAccess(_ context.Context, quizID, userID uuid.UUID, isAdmin bool) error {
	l, ok := g.locked[quizID]
	if ok && !isAdmin && userID != l.creator {
		return ErrQuizLockedByContest
	}
	return nil
}

func (g *fakeContestGate) CheckQuizEditable(_ context.Context, quizID uuid.UUID) error {
	if l, ok := g.locked[quizID]; ok && !l.editable {
		return ErrQuizEditLockedByContest
	}
	return nil
}
