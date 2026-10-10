package service

// Test cho QA S4 (261008): QuizAttemptResponseDTO không có `mode`, client không phân biệt được
// attempt practice với official nên đếm hết vào "số lần làm chính thức". Server đã đếm đúng
// (CountAttemptsByUserAndQuiz lọc mode='official'); thiếu là field trên response.
//
// Mutation muốn bắt: bỏ dòng `Mode: a.Mode` trong mapAttemptToDTO phải làm test ĐỎ.

import (
	"encoding/json"
	"testing"

	"study.com/v1/internal/model"
)

func TestMapAttemptToDTO_GiuNguyenMode(t *testing.T) {
	s := NewQuizService(nil, nil, nil, nil, nil, nil, nil)
	for _, mode := range []string{"official", "practice", "contest"} {
		out := s.mapAttemptToDTO(&model.QuizAttempt{Mode: mode})
		raw, err := json.Marshal(out)
		if err != nil {
			t.Fatal(err)
		}
		var got map[string]any
		if err := json.Unmarshal(raw, &got); err != nil {
			t.Fatal(err)
		}
		if got["mode"] != mode {
			t.Errorf("attempt mode=%q: JSON response có mode=%v, muốn %q", mode, got["mode"], mode)
		}
	}
}
