package service

// QA6 round 3 (C10): quiz tắt show_correct_answers -> bài chữa của HỌC VIÊN có correct_answer_ids = null,
// explanation = null, is_correct = null và points_earned = null (kể cả dòng câu bỏ trống) và answers_hidden = true, cho cả
// HỌC VIÊN lẫn PHỤ HUYNH đã liên kết, ở bài chữa và tiến độ; điểm tổng + số câu đúng giữ nguyên; giảng viên chủ quiz và admin
// giữ nguyên; quiz bật cờ thì học viên vẫn thấy đủ. Mutation đã kiểm: bỏ answersHiddenFrom khỏi GetAttemptByID làm ca ẩn ĐỎ;
// bỏ nhánh `answers[i].IsCorrect, answers[i].PointsEarned = nil, decimal.Zero` (B1, tầng service) làm ca DTO ĐỎ; bỏ hai field is_correct/points_earned khỏi MarshalJSON làm ca JSON ĐỎ; ẩn cả
// với giảng viên làm ca "giảng viên giữ nguyên" ĐỎ.

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"study.com/v1/internal/dto"
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

	// Phụ huynh đã liên kết (active) xem bài của con: cùng bị ẩn như học viên.
	parent := f.user("parent")
	if err := f.db.Create(&model.ParentStudentRelation{ParentUserID: parent.ID, StudentUserID: student.ID, Status: model.ParentStudentRelationStatusActive}).Error; err != nil {
		t.Fatal(err)
	}
	svc.SetParentLinkChecker(repository.NewParentStudentRepository(f.db))

	hiddenFields := []string{"correct_answer_ids", "explanation", "is_correct", "points_earned"}
	for _, view := range []struct {
		name string
		get  func(attemptID, viewerID uuid.UUID) (*dto.QuizAttemptDetailDTO, error)
	}{
		{"bài chữa", func(a, v uuid.UUID) (*dto.QuizAttemptDetailDTO, error) { return svc.GetAttemptByID(ctx, a, v, false) }},
		{"tiến độ", func(a, v uuid.UUID) (*dto.QuizAttemptDetailDTO, error) { return svc.GetAttemptProgress(ctx, a, v, false) }},
	} {
		for who, viewerID := range map[string]uuid.UUID{"học viên": student.ID, "phụ huynh": parent.ID} {
			got, err := view.get(hidden.ID, viewerID)
			if err != nil {
				t.Fatalf("%s %s: %v", view.name, who, err)
			}
			if !got.AnswersHidden || len(got.Answers) != 2 {
				t.Fatalf("%s %s: answers_hidden=%v rows=%d, muốn true và đủ 2 dòng (kể cả câu bỏ trống)", view.name, who, got.AnswersHidden, len(got.Answers))
			}
			// Điểm tổng và số câu đúng nằm ở cấp attempt, không bị ẩn.
			if got.ID != hidden.ID || got.CompletedAt == nil {
				t.Errorf("%s %s: mất thông tin cấp attempt: %+v", view.name, who, got.QuizAttemptResponseDTO)
			}
			// Tầng service (không chỉ MarshalJSON): consumer Go của DTO cũng không được nhận is_correct/points_earned.
			for i, a := range got.Answers {
				if a.IsCorrect != nil || !a.PointsEarned.IsZero() {
					t.Errorf("%s %s dòng %d: DTO còn is_correct=%v points_earned=%s khi đã ẩn", view.name, who, i, a.IsCorrect, a.PointsEarned)
				}
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
				t.Errorf("%s %s: JSON thiếu answers_hidden=true: %s", view.name, who, raw)
			}
			if len(body.Answers) != 2 {
				t.Fatalf("%s %s: JSON có %d dòng, muốn 2", view.name, who, len(body.Answers))
			}
			for i, row := range body.Answers {
				for _, field := range hiddenFields { // dòng 0 đã trả lời đúng, dòng 1 bỏ trống: cả hai phải null
					v, present := row[field]
					if !present || v != nil {
						t.Errorf("%s %s dòng %d: %s phải có mặt và = null, có present=%v value=%v", view.name, who, i, field, present, v)
					}
				}
			}
			// Id đáp án đúng vẫn có thể nằm trong selected_answer_ids (học viên chọn đúng); chỉ giải thích là chuỗi không được lộ.
			if strings.Contains(string(raw), "giải thích") {
				t.Errorf("%s %s: JSON vẫn lộ giải thích: %s", view.name, who, raw)
			}
		}
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
		if r := v.Answers[0]; r.IsCorrect == nil || !*r.IsCorrect || !r.PointsEarned.Equal(decimal.NewFromInt(1)) {
			t.Errorf("%s phải thấy nguyên is_correct/points_earned của câu đã trả lời: %+v", name, r)
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
