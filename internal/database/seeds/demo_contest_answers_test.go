package seeds

// QA-A-03: điểm bài thi demo phải bằng tổng điểm các dòng đáp án (test thuần, không cần DB).

import (
	"testing"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"study.com/v1/internal/model"
)

func demoTestQuestions(n int) []model.Question {
	qs := make([]model.Question, n)
	for i := range qs {
		qs[i] = model.Question{Points: decimal.NewFromInt(1), QuestionType: "single_choice",
			Answers: []model.QuestionAnswer{{ID: uuid.New(), IsCorrect: true}, {ID: uuid.New()}, {ID: uuid.New()}}}
		qs[i].ID = uuid.New()
	}
	return qs
}

func TestDemoContestAnswers_ScoreEqualsSumOfAnswerRows(t *testing.T) {
	qs := demoTestQuestions(3)
	answers, score, total := demoContestAnswers(qs, 2)
	if len(answers) != len(qs) {
		t.Fatalf("%d dòng đáp án, muốn %d (mỗi câu một dòng, không câu nào bị bỏ trống)", len(answers), len(qs))
	}
	sum := decimal.Zero
	for i, a := range answers {
		sum = sum.Add(a.PointsEarned)
		if len(a.SelectedAnswerIDs) == 0 {
			t.Errorf("câu %d không có đáp án được chọn", i+1)
		}
		wantRight := i < 2
		if a.IsCorrect == nil || *a.IsCorrect != wantRight {
			t.Errorf("câu %d is_correct = %v, muốn %v", i+1, a.IsCorrect, wantRight)
		}
		picked := a.SelectedAnswerIDs[0]
		if (picked == qs[i].Answers[0].ID.String()) != wantRight {
			t.Errorf("câu %d chọn %s, không khớp is_correct=%v", i+1, picked, wantRight)
		}
	}
	if !score.Equal(decimal.NewFromInt(2)) || !sum.Equal(score) || !total.Equal(decimal.NewFromInt(3)) {
		t.Fatalf("score=%s, tổng điểm từng câu=%s, total=%s; muốn 2 = 2 / 3", score, sum, total)
	}
}

func TestDemoCorrectCountForScore(t *testing.T) {
	qs := demoTestQuestions(3)
	for _, tc := range []struct {
		score int64
		want  int
		ok    bool
	}{{0, 0, true}, {2, 2, true}, {3, 3, true}, {4, 0, false}} {
		got, ok := demoCorrectCountForScore(qs, decimal.NewFromInt(tc.score))
		if got != tc.want || ok != tc.ok {
			t.Errorf("score %d: (%d,%v), muốn (%d,%v)", tc.score, got, ok, tc.want, tc.ok)
		}
	}
}
