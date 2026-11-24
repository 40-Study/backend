package service

import (
	"context"
	"errors"

	"github.com/google/uuid"

	"study.com/v1/internal/dto"
)

// withUnansweredQuestions (QA5 R1) trả bài chữa của attempt ĐÃ NỘP theo thứ tự đề (display_order), gồm
// cả câu bỏ trống: id = uuid.Nil, selected_answer_ids rỗng, is_correct=false, 0 điểm, kèm
// correct_answer_ids + explanation cùng dạng với câu đã trả lời. Chỉ gọi sau khi nộp — trước đó đáp
// án đúng không được lộ. hideAnswers (QA6 C10): quiz tắt show_correct_answers và người xem không quản lý quiz thì bỏ
// đáp án/giải thích.
func (s *QuizService) withUnansweredQuestions(ctx context.Context, quizID uuid.UUID, answered []dto.QuizAttemptAnswerDTO, hideAnswers bool) ([]dto.QuizAttemptAnswerDTO, error) {
	quiz, err := s.repo.GetQuizWithQuestions(ctx, quizID)
	if err != nil {
		return nil, err
	}
	if quiz == nil {
		return nil, errors.New("quiz not found")
	}

	byQuestion := make(map[uuid.UUID]dto.QuizAttemptAnswerDTO, len(answered))
	for _, a := range answered {
		byQuestion[a.QuestionID] = a
	}

	out := make([]dto.QuizAttemptAnswerDTO, 0, len(quiz.Questions))
	for i := range quiz.Questions {
		q := &quiz.Questions[i]
		if a, ok := byQuestion[q.ID]; ok {
			out = append(out, a)
			delete(byQuestion, q.ID)
			continue
		}
		correctIDs := []string{}
		for _, qa := range q.Answers {
			if qa.IsCorrect {
				correctIDs = append(correctIDs, qa.ID.String())
			}
		}
		notCorrect := false
		row := dto.QuizAttemptAnswerDTO{
			QuestionID:        q.ID,
			QuestionText:      q.QuestionText,
			SelectedAnswerIDs: []string{},
			IsCorrect:         &notCorrect,
			CorrectAnswerIDs:  correctIDs,
			Explanation:       q.Explanation,
		}
		if hideAnswers { // C10: câu bỏ trống cũng không lộ đáp án đúng/giải thích khi giảng viên đã ẩn; is_correct cũng null (cùng dạng dòng đã trả lời)
			row.CorrectAnswerIDs, row.Explanation, row.IsCorrect, row.AnswersHidden = nil, nil, nil, true
		}
		out = append(out, row)
	}
	// Dòng trả lời của câu không còn trong đề (không xảy ra với FK cascade): giữ nguyên, không bỏ rơi.
	for _, a := range answered {
		if _, left := byQuestion[a.QuestionID]; left {
			out = append(out, a)
		}
	}
	return out, nil
}
