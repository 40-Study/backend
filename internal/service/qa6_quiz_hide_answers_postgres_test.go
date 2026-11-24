package service

// QA6 round 3 (C10): quiz tắt show_correct_answers -> bài chữa của HỌC VIÊN có correct_answer_ids = null,
// explanation = null (kể cả dòng câu bỏ trống) và answers_hidden = true; giảng viên chủ quiz và admin giữ nguyên;
// quiz bật cờ thì học viên vẫn thấy đủ. Mutation đã kiểm: bỏ answersHiddenFrom khỏi GetAttemptByID làm ca ẩn ĐỎ; ẩn cả
// với giảng viên làm ca "giảng viên giữ nguyên" ĐỎ.

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"study.com/v1/internal/model"
	"study.com/v1/internal/repository"
)

func TestQA6_AttemptReview_ShowCorrectAnswersFalse_AnDapAnVoiHocVien_Postgres(t *testing.T) {
	f := newS2Fixture(t)
	teacher, student, admin := f.user("teacher"), f.user("student"), f.user("admin")
	course := f.course(teacher)
	mkQuiz := func(show bool) (model.Quiz, model.Question, string) {
		quiz := model.Quiz{CourseID: &course.ID, CreatedBy: &teacher.ID, Title: "QA6-C10 quiz " + uuid.NewString()[:4]}
		if err := f.db.Create(&quiz).Error; err != nil {
			t.Fatal(err)
		}
		if err := f.db.Model(&model.Quiz{}).Where("id = ?", quiz.ID).Update("show_correct_answers", show).Error; err != nil {
			t.Fatal(err)
		}
		expl := "giải thích"
		var first model.Question
		var correct string
		for i := 1; i <= 2; i++ {
			q := model.Question{QuizID: quiz.ID, QuestionText: "Câu hỏi", QuestionType: "single_choice", Explanation: &expl, Points: decimal.NewFromInt(1), DisplayOrder: i}
			if err := f.db.Create(&q).Error; err != nil {
				t.Fatal(err)
			}
			right := model.QuestionAnswer{QuestionID: q.ID, AnswerText: "đúng", IsCorrect: true, DisplayOrder: 1}
			if err := f.db.Create(&right).Error; err != nil {
				t.Fatal(err)
			}
			if i == 1 {
				first, correct = q, right.ID.String()
			}
		}
		return quiz, first, correct
	}
	submit := func(quiz model.Quiz, q model.Question, correct string) model.QuizAttempt {
		done := time.Now()
		att := model.QuizAttempt{UserID: student.ID, QuizID: quiz.ID, Mode: "practice", CompletedAt: &done}
		if err := f.db.Create(&att).Error; err != nil {
			t.Fatal(err)
		}
		yes := true // chỉ trả lời câu 1; câu 2 bỏ trống
		aa := model.QuizAttemptAnswer{AttemptID: att.ID, QuestionID: q.ID, SelectedAnswerIDs: []string{correct}, IsCorrect: &yes, PointsEarned: decimal.NewFromInt(1)}
		if err := f.db.Create(&aa).Error; err != nil {
			t.Fatal(err)
		}
		return att
	}
	svc := NewQuizService(repository.NewQuizRepository(f.db), nil, repository.NewCourseRepository(f.db),
		repository.NewSectionRepository(f.db), repository.NewLessonRepository(f.db), nil, repository.NewEnrollmentRepository(f.db))
	ctx := context.Background()

	hiddenQuiz, hq, hcorrect := mkQuiz(false)
	hidden := submit(hiddenQuiz, hq, hcorrect)

	got, err := svc.GetAttemptByID(ctx, hidden.ID, student.ID, false)
	if err != nil {
		t.Fatal(err)
	}
	if !got.AnswersHidden || len(got.Answers) != 2 {
		t.Fatalf("học viên: answers_hidden=%v rows=%d, muốn true và đủ 2 dòng (kể cả câu bỏ trống)", got.AnswersHidden, len(got.Answers))
	}
	raw, _ := json.Marshal(got)
	var body struct {
		AnswersHidden bool `json:"answers_hidden"`
		Answers       []map[string]any
	}
	if err := json.Unmarshal(raw, &body); err != nil {
		t.Fatal(err)
	}
	if !body.AnswersHidden {
		t.Errorf("JSON thiếu answers_hidden=true: %s", raw)
	}
	for i, row := range body.Answers {
		for _, field := range []string{"correct_answer_ids", "explanation"} {
			v, present := row[field]
			if !present || v != nil {
				t.Errorf("dòng %d: %s phải có mặt và = null, có present=%v value=%v", i, field, present, v)
			}
		}
	}
	// Id đáp án đúng vẫn có thể nằm trong selected_answer_ids (học viên chọn đúng); chỉ giải thích là chuỗi không được lộ.
	if strings.Contains(string(raw), "giải thích") {
		t.Errorf("JSON học viên vẫn lộ giải thích: %s", raw)
	}

	for name, viewer := range map[string]struct {
		id    uuid.UUID
		admin bool
	}{"giảng viên chủ quiz": {teacher.ID, false}, "admin": {admin.ID, true}} {
		v, err := svc.GetAttemptByID(ctx, hidden.ID, viewer.id, viewer.admin)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if v.AnswersHidden || len(v.Answers) != 2 || len(v.Answers[0].CorrectAnswerIDs) != 1 || v.Answers[0].Explanation == nil ||
			len(v.Answers[1].CorrectAnswerIDs) != 1 || v.Answers[1].Explanation == nil {
			t.Errorf("%s phải thấy nguyên đáp án + giải thích: %+v", name, v)
		}
	}

	// Đối chứng: quiz bật cờ thì học viên vẫn thấy đủ.
	shownQuiz, sq, scorrect := mkQuiz(true)
	shown := submit(shownQuiz, sq, scorrect)
	s, err := svc.GetAttemptByID(ctx, shown.ID, student.ID, false)
	if err != nil {
		t.Fatal(err)
	}
	if s.AnswersHidden || len(s.Answers) != 2 || len(s.Answers[0].CorrectAnswerIDs) != 1 || s.Answers[1].Explanation == nil {
		t.Errorf("quiz bật cờ: học viên phải thấy đáp án + giải thích: %+v", s)
	}
}
