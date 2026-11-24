package service

// QA5 R1 (S5-11): sau khi nộp, bài chữa trả MỌI câu của đề theo display_order, kể cả câu bỏ trống, mỗi câu
// kèm đáp án đúng cùng dạng với câu đã trả lời (selected rỗng). Trước khi nộp thì không lộ gì. Bỏ
// withUnansweredQuestions khỏi GetAttemptByID thì ca "đủ đề" ĐỎ.

import (
	"context"
	"testing"
	"time"

	"github.com/shopspring/decimal"
	"study.com/v1/internal/model"
	"study.com/v1/internal/repository"
)

func TestQA5R1_AttemptReview_IncludesUnansweredQuestions_Postgres(t *testing.T) {
	f := newS2Fixture(t)
	teacher, student := f.user("teacher"), f.user("student")
	course := f.course(teacher)
	quiz := model.Quiz{CourseID: &course.ID, CreatedBy: &teacher.ID, Title: "QA5-R1 quiz"}
	if err := f.db.Create(&quiz).Error; err != nil {
		t.Fatal(err)
	}

	type seeded struct {
		q       model.Question
		correct string
	}
	qs := make([]seeded, 3)
	for i := range qs {
		expl := "giải thích câu " + string(rune('1'+i))
		q := model.Question{QuizID: quiz.ID, QuestionText: "Câu hỏi " + string(rune('1'+i)), QuestionType: "single_choice",
			Explanation: &expl, Points: decimal.NewFromInt(1), DisplayOrder: i + 1}
		if err := f.db.Create(&q).Error; err != nil {
			t.Fatal(err)
		}
		right := model.QuestionAnswer{QuestionID: q.ID, AnswerText: "đúng", IsCorrect: true, DisplayOrder: 1}
		wrong := model.QuestionAnswer{QuestionID: q.ID, AnswerText: "sai", DisplayOrder: 2}
		if err := f.db.Create(&right).Error; err != nil {
			t.Fatal(err)
		}
		if err := f.db.Create(&wrong).Error; err != nil {
			t.Fatal(err)
		}
		qs[i] = seeded{q: q, correct: right.ID.String()}
	}

	// Bài làm đã nộp: chỉ trả lời câu 1 và câu 2, bỏ câu 3.
	done := time.Now()
	submitted := model.QuizAttempt{UserID: student.ID, QuizID: quiz.ID, Mode: "practice", CompletedAt: &done}
	if err := f.db.Create(&submitted).Error; err != nil {
		t.Fatal(err)
	}
	yes := true
	for _, s := range qs[:2] {
		aa := model.QuizAttemptAnswer{AttemptID: submitted.ID, QuestionID: s.q.ID, SelectedAnswerIDs: []string{s.correct}, IsCorrect: &yes, PointsEarned: decimal.NewFromInt(1)}
		if err := f.db.Create(&aa).Error; err != nil {
			t.Fatal(err)
		}
	}
	// Bài làm đang dở, cũng chưa trả lời câu nào.
	inProgress := model.QuizAttempt{UserID: student.ID, QuizID: quiz.ID, Mode: "practice"}
	if err := f.db.Create(&inProgress).Error; err != nil {
		t.Fatal(err)
	}

	svc := NewQuizService(repository.NewQuizRepository(f.db), nil, repository.NewCourseRepository(f.db),
		repository.NewSectionRepository(f.db), repository.NewLessonRepository(f.db), nil, repository.NewEnrollmentRepository(f.db))
	ctx := context.Background()

	got, err := svc.GetAttemptByID(ctx, submitted.ID, student.ID, false)
	if err != nil {
		t.Fatalf("GetAttemptByID: %v", err)
	}
	if len(got.Answers) != 3 {
		t.Fatalf("muốn đủ 3 câu của đề, có %d", len(got.Answers))
	}
	for i, a := range got.Answers {
		if a.QuestionID != qs[i].q.ID {
			t.Errorf("câu %d phải theo display_order: muốn %s, có %s", i+1, qs[i].q.ID, a.QuestionID)
		}
		if len(a.CorrectAnswerIDs) != 1 || a.CorrectAnswerIDs[0] != qs[i].correct {
			t.Errorf("câu %d phải kèm đáp án đúng %s, có %v", i+1, qs[i].correct, a.CorrectAnswerIDs)
		}
		if a.Explanation == nil || *a.Explanation == "" {
			t.Errorf("câu %d phải kèm giải thích", i+1)
		}
	}
	skipped := got.Answers[2]
	if skipped.QuestionText != qs[2].q.QuestionText {
		t.Errorf("câu bỏ trống phải mang đề bài, có %q", skipped.QuestionText)
	}
	if skipped.SelectedAnswerIDs == nil || len(skipped.SelectedAnswerIDs) != 0 {
		t.Errorf("câu bỏ trống: selected phải là mảng rỗng, có %#v", skipped.SelectedAnswerIDs)
	}
	if skipped.IsCorrect == nil || *skipped.IsCorrect || !skipped.PointsEarned.IsZero() {
		t.Errorf("câu bỏ trống: is_correct=false, 0 điểm; có %v / %v", skipped.IsCorrect, skipped.PointsEarned)
	}
	if got.Answers[0].IsCorrect == nil || !*got.Answers[0].IsCorrect {
		t.Errorf("câu đã trả lời đúng phải giữ nguyên is_correct=true")
	}

	// Chưa nộp: không bổ sung câu nào và không lộ đáp án đúng.
	live, err := svc.GetAttemptByID(ctx, inProgress.ID, student.ID, false)
	if err != nil {
		t.Fatalf("GetAttemptByID (đang làm): %v", err)
	}
	if len(live.Answers) != 0 {
		t.Errorf("attempt chưa nộp không được liệt kê câu hỏi/đáp án, có %d", len(live.Answers))
	}
	for _, a := range live.Answers {
		if len(a.CorrectAnswerIDs) != 0 || a.Explanation != nil {
			t.Errorf("attempt chưa nộp lộ đáp án: %+v", a)
		}
	}
}
