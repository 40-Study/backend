package service

// QA T10 / contract C6: mọi response quiz phải mang question_count THẬT (đếm câu hỏi chưa xoá ở
// server). Trước đây mapQuizToDTO bị gọi với số 0 cố định ở đường tạo/liệt kê/sửa/"quiz của tôi"
// nên danh sách quiz luôn hiện "0 câu hỏi". Test đọc GIÁ TRỊ (không chỉ hình dạng JSON như
// TestQAFollowupContract_QuizResponse_QuestionCountAlwaysPresent): khôi phục số 0 cố định ở bất kỳ
// đường nào dưới đây thì test ĐỎ.

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"study.com/v1/internal/dto"
	"study.com/v1/internal/model"
)

// questionCountQuiz tạo quiz đứng riêng với đúng n câu hỏi còn sống và `deleted` câu đã xoá mềm
// (không được đếm).
func (f *contestFixture) questionCountQuiz(creator uuid.UUID, n, deleted int) uuid.UUID {
	f.t.Helper()
	q := model.Quiz{Title: "QA T10 " + uuid.NewString()[:8], TriggerType: "manual", CreatedBy: &creator}
	if err := f.db.Create(&q).Error; err != nil {
		f.t.Fatalf("tạo quiz: %v", err)
	}
	for i := 0; i < n+deleted; i++ {
		question := model.Question{QuizID: q.ID, QuestionText: "Câu hỏi", QuestionType: "true_false",
			Points: decimal.NewFromInt(1), DisplayOrder: i + 1}
		if err := f.db.Create(&question).Error; err != nil {
			f.t.Fatalf("tạo câu hỏi: %v", err)
		}
		if i >= n {
			if err := f.db.Delete(&question).Error; err != nil {
				f.t.Fatalf("xoá mềm câu hỏi: %v", err)
			}
		}
	}
	return q.ID
}

func TestQuizQuestionCount_CreateUpdateListMine_ReturnRealCount(t *testing.T) {
	f := newContestFixture(t)
	ctx := context.Background()
	teacher := f.user("teacher")

	three := f.questionCountQuiz(teacher, 3, 2) // 2 câu xoá mềm không được đếm
	five := f.questionCountQuiz(teacher, 5, 0)
	empty := f.questionCountQuiz(teacher, 0, 1)
	want := map[uuid.UUID]int{three: 3, five: 5, empty: 0}

	t.Run("create: quiz mới chưa có câu hỏi -> 0", func(t *testing.T) {
		created, err := f.quiz.CreateQuiz(ctx, teacher, dto.CreateQuizDTO{Title: "Mới tạo"})
		if err != nil {
			t.Fatalf("CreateQuiz: %v", err)
		}
		if created.QuestionCount != 0 {
			t.Fatalf("question_count = %d, muốn 0", created.QuestionCount)
		}
	})

	t.Run("update: trả số câu thật", func(t *testing.T) {
		title := "Đổi tên"
		for id, n := range want {
			got, err := f.quiz.UpdateQuiz(ctx, id, teacher, false, dto.UpdateQuizDTO{Title: &title})
			if err != nil {
				t.Fatalf("UpdateQuiz: %v", err)
			}
			if got.QuestionCount != n {
				t.Errorf("UpdateQuiz question_count = %d, muốn %d", got.QuestionCount, n)
			}
		}
	})

	t.Run("list: đếm theo TỪNG quiz, không dùng một số chung", func(t *testing.T) {
		list, err := f.quiz.GetAllQuizzes(ctx, nil, nil, nil, teacher, true, 1, 50)
		if err != nil {
			t.Fatalf("GetAllQuizzes: %v", err)
		}
		assertListCounts(t, "GetAllQuizzes", list.Data, want)
	})

	t.Run("mine: quiz của tôi", func(t *testing.T) {
		list, err := f.quiz.GetMyCreatedQuizzes(ctx, teacher, 1, 50)
		if err != nil {
			t.Fatalf("GetMyCreatedQuizzes: %v", err)
		}
		assertListCounts(t, "GetMyCreatedQuizzes", list.Data, want)
	})

	t.Run("detail vẫn đếm đúng", func(t *testing.T) {
		got, err := f.quiz.GetQuizByID(ctx, three, teacher, false)
		if err != nil {
			t.Fatalf("GetQuizByID: %v", err)
		}
		if got.QuestionCount != 3 {
			t.Fatalf("detail question_count = %d, muốn 3", got.QuestionCount)
		}
	})
}

func assertListCounts(t *testing.T, name string, got []dto.QuizResponseDTO, want map[uuid.UUID]int) {
	t.Helper()
	seen := map[uuid.UUID]bool{}
	for _, q := range got {
		n, ok := want[q.ID]
		if !ok {
			continue // quiz khác (vd. quiz "Mới tạo") không nằm trong bảng kỳ vọng
		}
		seen[q.ID] = true
		if q.QuestionCount != n {
			t.Errorf("%s: quiz %s question_count = %d, muốn %d", name, q.ID, q.QuestionCount, n)
		}
	}
	for id := range want {
		if !seen[id] {
			t.Errorf("%s: thiếu quiz %s trong danh sách", name, id)
		}
	}
}
