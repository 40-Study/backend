package router

// Contract ĐÍNH CHÍNH 3 (review web M1): my-result trả CHỮ đáp án (options / accepted_answers) —
// chỉ từ answers_available_at, trước đó không có một chữ đáp án nào.

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"study.com/v1/internal/model"
)

type reviewOption struct {
	ID           string `json:"id"`
	AnswerText   string `json:"answer_text"`
	DisplayOrder int    `json:"display_order"`
}

type reviewQuestion struct {
	QuestionID        string          `json:"question_id"`
	QuestionType      string          `json:"question_type"`
	Options           *[]reviewOption `json:"options"`
	AcceptedAnswers   *[]string       `json:"accepted_answers"`
	SelectedAnswerIDs *[]string       `json:"selected_answer_ids"`
	CorrectAnswerIDs  []string        `json:"correct_answer_ids"`
	TextAnswer        *string         `json:"text_answer"`
}

func TestContestLive_MyResultAnswerTexts(t *testing.T) {
	e := newCtEnv(t)
	id, _, key := e.publishedContest("")
	e.setWindow(id, time.Minute, time.Hour)
	cid := id.String()
	e.must("join", e.do("POST", "/api/contests/"+cid+"/join", "student1", ""), 201)
	st := e.must("start", e.do("POST", "/api/contests/"+cid+"/start", "student1", ""), 200)
	// Câu chọn 1 đúng, câu chọn 2 sai, câu điền sai ("sai roi").
	e.must("submit", e.do("POST", "/api/contests/"+cid+"/submit", "student1", answersBody(st.data()["attempt_id"].(string), key, 1)), 200)

	// Trước mốc: không có chữ lựa chọn, đáp án điền, hay field mới nào.
	e.setWindow(id, time.Hour, -2*time.Second)
	before := e.must("my-result truoc moc", e.do("GET", "/api/contests/"+cid+"/my-result", "student1", ""), 200)
	for _, leak := range []string{"lua chon A", "lua chon B", secretFillBlank, `"options"`, `"accepted_answers"`} {
		if strings.Contains(before.raw, leak) {
			t.Fatalf("my-result truoc answers_available_at lo %q: %s", leak, before.raw)
		}
	}

	// Sau mốc: đủ chữ cho mọi câu.
	e.setWindow(id, time.Hour, -time.Duration(model.ContestSubmitGraceSeconds+1)*time.Second)
	after := e.must("my-result sau moc", e.do("GET", "/api/contests/"+cid+"/my-result", "student1", ""), 200)
	var body struct {
		Data struct {
			Questions []reviewQuestion `json:"questions"`
		} `json:"data"`
	}
	if err := json.Unmarshal([]byte(after.raw), &body); err != nil {
		t.Fatalf("parse my-result: %v", err)
	}
	if len(body.Data.Questions) != 3 {
		t.Fatalf("my-result sau moc phai co 3 cau: %s", after.raw)
	}
	for _, q := range body.Data.Questions {
		if q.Options == nil || q.AcceptedAnswers == nil || q.SelectedAnswerIDs == nil {
			t.Fatalf("cau %s thieu options/accepted_answers/selected_answer_ids (phai la mang, khong null): %s", q.QuestionID, after.raw)
		}
		switch q.QuestionType {
		case "fill_blank":
			if len(*q.Options) != 0 || strings.Join(*q.AcceptedAnswers, "|") != secretFillBlank {
				t.Fatalf("fill_blank: options=%v accepted=%v, muon [] va [%s]", *q.Options, *q.AcceptedAnswers, secretFillBlank)
			}
			if q.TextAnswer == nil || *q.TextAnswer != "sai roi" || len(*q.SelectedAnswerIDs) != 0 {
				t.Fatalf("fill_blank: bai lam sai: %+v", q)
			}
		case "single_choice":
			opts := *q.Options
			if len(opts) != 2 || opts[0].AnswerText != "lua chon A" || opts[0].DisplayOrder != 1 ||
				opts[1].AnswerText != "lua chon B" || opts[1].DisplayOrder != 2 || len(*q.AcceptedAnswers) != 0 {
				t.Fatalf("single_choice: options sai thu tu/noi dung: %+v", q)
			}
			// Web tra được chữ cho đáp án đúng và đáp án đã chọn từ options.
			text := map[string]string{}
			for _, o := range opts {
				text[o.ID] = o.AnswerText
			}
			if len(q.CorrectAnswerIDs) != 1 || text[q.CorrectAnswerIDs[0]] != "lua chon A" {
				t.Fatalf("khong tra duoc chu dap an dung: %+v", q)
			}
			if len(*q.SelectedAnswerIDs) != 1 || text[(*q.SelectedAnswerIDs)[0]] == "" {
				t.Fatalf("khong tra duoc chu dap an da chon: %+v", q)
			}
		default:
			t.Fatalf("question_type la %q: %s", q.QuestionType, after.raw)
		}
	}
}
