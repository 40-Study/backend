package dto

import (
	"testing"

	"study.com/v1/internal/utils"
)

// Q2-04 (QA vòng 2 261013): form tạo/sửa quiz của giáo viên giờ gửi pass_percentage và max_attempts
// thật (trước đây web cứng 70/3). Backend đã nhận và lưu hai trường này, nhưng không kiểm miền giá
// trị: pass_percentage=150 tràn cột decimal(5,2) hoặc lưu ngưỡng không thể đạt, max_attempts=0/-1 khoá
// học viên khỏi quiz ngay từ lần làm đầu. Miền hợp lệ: pass 0..100, max_attempts 1..100.

func f64(v float64) *float64 { return &v }
func ip(v int) *int          { return &v }

func quizValidationFields(errs []utils.ValidationError) map[string]bool {
	out := map[string]bool{}
	for _, e := range errs {
		out[e.Field] = true
	}
	return out
}

func TestQuizSettings_MienGiaTriHopLe(t *testing.T) {
	cases := []struct {
		name      string
		pass      *float64
		attempts  *int
		wantField string // "" = hợp lệ
	}{
		{"mac dinh de trong", nil, nil, ""},
		{"pass 0 hop le", f64(0), ip(1), ""},
		{"pass 100 hop le", f64(100), ip(100), ""},
		{"pass thap phan", f64(72.5), ip(3), ""},
		{"pass am", f64(-1), nil, "pass_percentage"},
		{"pass vuot 100", f64(150), nil, "pass_percentage"},
		{"attempts bang 0", nil, ip(0), "max_attempts"},
		{"attempts am", nil, ip(-3), "max_attempts"},
		{"attempts qua lon", nil, ip(101), "max_attempts"},
	}
	for _, tc := range cases {
		t.Run("create/"+tc.name, func(t *testing.T) {
			got := quizValidationFields(utils.ValidateStruct(CreateQuizDTO{Title: "Bai kiem tra", PassPercentage: tc.pass, MaxAttempts: tc.attempts}))
			assertQuizField(t, got, tc.wantField)
		})
		t.Run("update/"+tc.name, func(t *testing.T) {
			got := quizValidationFields(utils.ValidateStruct(UpdateQuizDTO{PassPercentage: tc.pass, MaxAttempts: tc.attempts}))
			assertQuizField(t, got, tc.wantField)
		})
	}
}

func assertQuizField(t *testing.T, got map[string]bool, want string) {
	t.Helper()
	if want == "" {
		if len(got) != 0 {
			t.Fatalf("muon hop le, nhung loi tren %v", got)
		}
		return
	}
	if !got[want] || len(got) != 1 {
		t.Fatalf("muon dung 1 loi tren %q, duoc %v", want, got)
	}
}
