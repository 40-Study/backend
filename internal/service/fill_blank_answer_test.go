package service

import (
	"testing"

	"github.com/google/uuid"
	"golang.org/x/text/unicode/norm"
	"study.com/v1/internal/dto"
	"study.com/v1/internal/model"
)

// Chấm fill_blank qua checkAnswer (đường dùng chung của quiz thường và cuộc thi). Mỗi ca ứng với
// một quy tắc chuẩn hoá của contract "Cuộc thi" ĐÍNH CHÍNH 3; bỏ quy tắc nào thì ca đó ĐỎ.
func TestCheckAnswer_FillBlankNormalization(t *testing.T) {
	q := &model.Question{QuestionType: "fill_blank", Answers: []model.QuestionAnswer{
		{ID: uuid.New(), AnswerText: "Hà Nội", IsCorrect: true},
		{ID: uuid.New(), AnswerText: "Sài Gòn", IsCorrect: false}, // không phải đáp án đúng
	}}
	decomposed := norm.NFD.String("Hà Nội")
	if decomposed == "Hà Nội" {
		t.Fatal("fixture: chuỗi NFD phải khác chuỗi dựng sẵn")
	}
	cases := []struct {
		rule, text string
		want       bool
	}{
		{"khớp nguyên văn", "Hà Nội", true},
		{"bỏ khoảng trắng đầu/cuối", "  Hà Nội \t", true},
		{"gộp khoảng trắng liên tiếp", "Hà   Nội", true},
		{"không phân biệt hoa thường", "hà nội", true},
		{"không phân biệt hoa thường (chữ hoa có dấu)", "HÀ NỘI", true},
		{"chuẩn hoá Unicode NFC", decomposed, true},
		{"giữ dấu tiếng Việt", "ha noi", false},
		{"sai dấu", "Hà Nôi", false},
		{"không bỏ khoảng trắng giữa từ", "HàNội", false},
		{"đáp án không is_correct", "Sài Gòn", false},
		{"bỏ trống", "   ", false},
	}
	svc := &QuizService{}
	for _, c := range cases {
		got := svc.checkAnswer(q, dto.SubmitAnswerDTO{QuestionID: uuid.NewString(), TextAnswer: c.text})
		if got != c.want {
			t.Errorf("%s: checkAnswer(%q) = %v, muốn %v", c.rule, c.text, got, c.want)
		}
	}
}

// Bài bỏ trống không bao giờ đúng, kể cả khi đề lỡ có đáp án chỉ gồm khoảng trắng.
func TestCheckAnswer_FillBlankBlankNeverMatches(t *testing.T) {
	q := &model.Question{QuestionType: "fill_blank", Answers: []model.QuestionAnswer{
		{ID: uuid.New(), AnswerText: "  ", IsCorrect: true},
	}}
	for _, text := range []string{"", "   ", "\t"} {
		if (&QuizService{}).checkAnswer(q, dto.SubmitAnswerDTO{TextAnswer: text}) {
			t.Fatalf("bài bỏ trống %q bị chấm đúng", text)
		}
	}
}

// Đáp án lưu trong đề cũng được chuẩn hoá (giảng viên gõ thừa khoảng trắng / NFD vẫn chấm đúng).
func TestCheckAnswer_FillBlankNormalizesStoredAnswer(t *testing.T) {
	q := &model.Question{QuestionType: "fill_blank", Answers: []model.QuestionAnswer{
		{ID: uuid.New(), AnswerText: " " + norm.NFD.String("Hà  Nội") + " ", IsCorrect: true},
	}}
	if !(&QuizService{}).checkAnswer(q, dto.SubmitAnswerDTO{TextAnswer: "hà nội"}) {
		t.Fatal("đáp án trong đề phải được chuẩn hoá giống bài làm")
	}
}
