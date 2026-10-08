package service

import (
	"errors"
	"fmt"

	"study.com/v1/internal/dto"
)

// ErrInvalidQuestionAnswers: bộ đáp án không hợp lệ với loại câu hỏi (QA T3). Handler trả 400 kèm
// message của lỗi bọc bên trong.
var ErrInvalidQuestionAnswers = errors.New("invalid question answers")

// validateQuestionAnswers (QA T3): câu trắc nghiệm không có đáp án đúng thì học viên không thể đạt
// điểm. single_choice / true_false cần ĐÚNG 1 đáp án đúng; multiple_choice cần ít nhất 1.
// fill_blank và essay không dùng is_correct để chấm theo lựa chọn nên không kiểm ở đây.
func validateQuestionAnswers(questionType string, answers []dto.CreateAnswerDTO) error {
	correct := 0
	for _, a := range answers {
		if a.IsCorrect {
			correct++
		}
	}
	switch questionType {
	case "single_choice", "true_false":
		if correct != 1 {
			return fmt.Errorf("%w: %s question must have exactly 1 correct answer (got %d)", ErrInvalidQuestionAnswers, questionType, correct)
		}
	case "multiple_choice":
		if correct < 1 {
			return fmt.Errorf("%w: multiple_choice question must have at least 1 correct answer (got %d)", ErrInvalidQuestionAnswers, correct)
		}
	}
	return nil
}
