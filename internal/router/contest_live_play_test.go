package router

import (
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"study.com/v1/internal/model"
)

// Tên field đáp án VÀ chính chuỗi đáp án/giải thích trong đề (secretFillBlank là đáp án đúng của
// câu fill_blank, nằm ở question_answers.answer_text — tìm theo tên field sẽ bỏ lọt nó).
var answerKeys = []string{`"is_correct"`, `"correct_answer_ids"`, `"explanation"`, `"answer_key"`, `"test_cases"`,
	secretFillBlank, secretExplanation}

func assertNoAnswerLeak(t *testing.T, what, raw string) {
	t.Helper()
	for _, k := range answerKeys {
		if strings.Contains(raw, k) {
			t.Fatalf("%s lo dap an (%s): %s", what, k, raw)
		}
	}
}

// (d) Không lộ đáp án: start/submit/detail/my-result (khi ACTIVE)/leaderboard không có field đáp
// án; voucher code không có trong DTO công khai. my-result chỉ có đáp án SAU khi đóng.
func TestContestLive_NoAnswerOrVoucherCodeLeak(t *testing.T) {
	e := newCtEnv(t)
	quizID, correct := e.newQuiz("teacherA")
	r := e.must("tao", e.do("POST", "/api/contests", "teacherA", contestBody(quizID, "")), 201)
	cid, slug := r.data()["id"].(string), r.data()["slug"].(string)
	e.must("gui duyet", e.do("POST", "/api/contests/"+cid+"/submit-review", "teacherA", ""), 200)
	voucherID := e.newVoucher(true)
	var code string
	e.db.Raw("SELECT code FROM vouchers WHERE id = ?", voucherID).Scan(&code)
	e.must("duyet kem voucher", e.do("POST", "/api/admin/contests/"+cid+"/approve", "admin",
		`{"prizes":[{"rank_from":1,"rank_to":1,"grant_certificate":true,"voucher_id":"`+voucherID.String()+`"}]}`), 200)
	id := uuid.MustParse(cid)
	e.setWindow(id, time.Minute, time.Hour)

	e.must("join", e.do("POST", "/api/contests/"+cid+"/join", "student1", ""), 201)
	st := e.must("start", e.do("POST", "/api/contests/"+cid+"/start", "student1", ""), 200)
	assertNoAnswerLeak(t, "start", st.raw)
	if qs, _ := st.data()["questions"].([]interface{}); len(qs) != 3 {
		t.Fatalf("start phai tra 3 cau hoi (co fill_blank): %s", st.raw)
	}
	sub := e.must("submit", e.do("POST", "/api/contests/"+cid+"/submit", "student1", answersBody(st.data()["attempt_id"].(string), correct, 1)), 200)
	assertNoAnswerLeak(t, "submit", sub.raw)
	// Decimal ra JSON NUMBER như cmd/api (ĐÍNH CHÍNH 2): kiểu float64 sau Unmarshal, không phải string.
	if s, ok := sub.data()["score"].(float64); !ok || s != 1 {
		t.Fatalf("score phai la number 1: %s", sub.raw)
	}
	if tp, ok := sub.data()["total_points"].(float64); !ok || tp != 3 {
		t.Fatalf("total_points phai la number 3: %s", sub.raw)
	}
	for _, who := range []string{"", "student1", "student2"} {
		d := e.must("detail "+who, e.do("GET", "/api/contests/"+slug, who, ""), 200)
		assertNoAnswerLeak(t, "detail "+who, d.raw)
		if strings.Contains(d.raw, code) {
			t.Fatalf("detail cong khai lo ma voucher %s: %s", code, d.raw)
		}
	}
	list := e.must("list", e.do("GET", "/api/contests", "", ""), 200)
	if strings.Contains(list.raw, code) || !strings.Contains(list.raw, `"has_voucher_prize":true`) {
		t.Fatalf("list cong khai lo ma voucher hoac thieu has_voucher_prize: %s", list.raw)
	}
	res := e.must("my-result ACTIVE", e.do("GET", "/api/contests/"+cid+"/my-result", "student1", ""), 200)
	assertNoAnswerLeak(t, "my-result ACTIVE", res.raw)
	if res.data()["questions"] != nil {
		t.Fatalf("my-result khi ACTIVE phai questions=null: %s", res.raw)
	}
	lb := e.must("BXH chu", e.do("GET", "/api/contests/"+cid+"/leaderboard", "teacherA", ""), 200)
	assertNoAnswerLeak(t, "leaderboard", lb.raw)

	e.setWindow(id, 2*time.Hour, -time.Minute)
	res = e.must("my-result ENDED", e.do("GET", "/api/contests/"+cid+"/my-result", "student1", ""), 200)
	if qs, _ := res.data()["questions"].([]interface{}); len(qs) != 3 || !strings.Contains(res.raw, `"correct_answer_ids"`) {
		t.Fatalf("my-result sau khi dong phai co dap an: %s", res.raw)
	}
	lb = e.must("BXH ENDED", e.do("GET", "/api/contests/"+cid+"/leaderboard", "", ""), 200)
	assertNoAnswerLeak(t, "leaderboard ENDED", lb.raw)
}

// (e) Race join: 5 học viên cùng đăng ký cuộc thi 3 chỗ → đúng 3 thành công, participant_count = 3.
func TestContestLive_RaceJoinRespectsCapacity(t *testing.T) {
	e := newCtEnv(t)
	id, _, _ := e.publishedContest("")
	e.db.Exec("UPDATE contests SET max_participants = 3 WHERE id = ?", id)
	var wg sync.WaitGroup
	statuses := make([]int, 5)
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			who := "student" + string(rune('1'+i))
			statuses[i] = e.do("POST", "/api/contests/"+id.String()+"/join", who, "").status
		}(i)
	}
	wg.Wait()
	ok, full := 0, 0
	for _, s := range statuses {
		switch s {
		case 201:
			ok++
		case 409:
			full++
		}
	}
	var rows, count int64
	e.db.Model(&model.ContestParticipant{}).Where("contest_id = ?", id).Count(&rows)
	e.db.Raw("SELECT participant_count FROM contests WHERE id = ?", id).Scan(&count)
	if ok != 3 || full != 2 || rows != 3 || count != 3 {
		t.Fatalf("race join: ok=%d full=%d rows=%d count=%d statuses=%v", ok, full, rows, count, statuses)
	}
	e.must("join trung", e.do("POST", "/api/contests/"+id.String()+"/join", "student1", ""), 409)
}

// (f) Race start: 2 request start đồng thời → cùng 1 attempt; reload trả lại đúng attempt đó.
func TestContestLive_RaceStartSingleAttempt(t *testing.T) {
	e := newCtEnv(t)
	id, _, _ := e.publishedContest("")
	e.setWindow(id, time.Minute, time.Hour)
	cid := id.String()
	e.must("join", e.do("POST", "/api/contests/"+cid+"/join", "student1", ""), 201)
	// Engine giữ tx 300ms khi tạo attempt: không có FOR UPDATE thì các request còn lại chắc chắn
	// đọc attempt_id NULL trong cửa sổ đó (mutation M9 đã cho thấy cửa sổ tự nhiên quá hẹp).
	e.engine.createDelay = 300 * time.Millisecond
	var wg sync.WaitGroup
	gate := make(chan struct{})
	res := make([]ctResp, 6)
	for i := range res {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-gate // barrier: mọi request xuất phát cùng lúc
			res[i] = e.do("POST", "/api/contests/"+cid+"/start", "student1", "")
		}(i)
	}
	close(gate)
	wg.Wait()
	e.engine.createDelay = 0
	for i, r := range res {
		if r.status != 200 {
			t.Fatalf("start %d: %d %s", i, r.status, r.raw)
		}
		if r.data()["attempt_id"] != res[0].data()["attempt_id"] {
			t.Fatalf("start %d tra attempt khac: %v vs %v", i, r.data()["attempt_id"], res[0].data()["attempt_id"])
		}
	}
	again := e.must("reload", e.do("POST", "/api/contests/"+cid+"/start", "student1", ""), 200)
	var n int64
	e.db.Model(&model.QuizAttempt{}).Where("user_id = ?", e.ids["student1"]).Count(&n)
	if again.data()["attempt_id"] != res[0].data()["attempt_id"] || n != 1 {
		t.Fatalf("race start tao nhieu attempt: reload=%v, so attempt=%d", again.data()["attempt_id"], n)
	}
}

// (g) Nộp sau deadline + 30s → 409 và KHÔNG chấm; trong 30s ân hạn vẫn nộp được; start lại khi
// đã hết giờ → 409.
func TestContestLive_SubmitDeadline(t *testing.T) {
	e := newCtEnv(t)
	id, _, correct := e.publishedContest("") // duration 30 phút
	e.setWindow(id, 2*time.Hour, 2*time.Hour)
	cid := id.String()
	for _, who := range []string{"student1", "student2"} {
		e.must("join "+who, e.do("POST", "/api/contests/"+cid+"/join", who, ""), 201)
	}
	late := e.must("start 1", e.do("POST", "/api/contests/"+cid+"/start", "student1", ""), 200)
	grace := e.must("start 2", e.do("POST", "/api/contests/"+cid+"/start", "student2", ""), 200)
	e.db.Exec("UPDATE quiz_attempts SET started_at = ? WHERE id = ?", time.Now().Add(-30*time.Minute-31*time.Second), late.data()["attempt_id"])
	e.db.Exec("UPDATE quiz_attempts SET started_at = ? WHERE id = ?", time.Now().Add(-30*time.Minute-15*time.Second), grace.data()["attempt_id"])

	e.must("nop tre", e.do("POST", "/api/contests/"+cid+"/submit", "student1", answersBody(late.data()["attempt_id"].(string), correct, 2)), 409, "CONTEST_DEADLINE_PASSED")
	var graded int64
	e.db.Model(&model.QuizAttempt{}).Where("id = ? AND completed_at IS NOT NULL", late.data()["attempt_id"]).Count(&graded)
	if graded != 0 {
		t.Fatal("bai nop tre van bi cham")
	}
	e.must("start lai khi het gio", e.do("POST", "/api/contests/"+cid+"/start", "student1", ""), 409, "CONTEST_DEADLINE_PASSED")
	e.must("nop trong an han", e.do("POST", "/api/contests/"+cid+"/submit", "student2", answersBody(grace.data()["attempt_id"].(string), correct, 2)), 200)
	e.must("nop lan 2", e.do("POST", "/api/contests/"+cid+"/submit", "student2", answersBody(grace.data()["attempt_id"].(string), correct, 2)), 409, "CONTEST_ALREADY_SUBMITTED")
	e.must("start sau khi nop", e.do("POST", "/api/contests/"+cid+"/start", "student2", ""), 409, "CONTEST_ALREADY_SUBMITTED")
}

// (h) Xếp hạng: điểm cao trước; bằng điểm thì ai NỘP TRƯỚC xếp trên.
func TestContestLive_LeaderboardTieBreakBySubmitTime(t *testing.T) {
	e := newCtEnv(t)
	id, _, correct := e.publishedContest("")
	e.setWindow(id, time.Minute, time.Hour)
	cid := id.String()
	attempts := map[string]string{}
	for who, right := range map[string]int{"student1": 2, "student2": 2, "student3": 1} {
		e.must("join "+who, e.do("POST", "/api/contests/"+cid+"/join", who, ""), 201)
		st := e.must("start "+who, e.do("POST", "/api/contests/"+cid+"/start", who, ""), 200)
		attempts[who] = st.data()["attempt_id"].(string)
		e.must("submit "+who, e.do("POST", "/api/contests/"+cid+"/submit", who, answersBody(attempts[who], correct, right)), 200)
	}
	// student2 nộp TRƯỚC student1 (cùng 2 điểm) → student2 hạng 1.
	base := time.Now().Add(-10 * time.Minute)
	e.db.Exec("UPDATE quiz_attempts SET completed_at = ? WHERE id = ?", base.Add(2*time.Minute), attempts["student1"])
	e.db.Exec("UPDATE quiz_attempts SET completed_at = ? WHERE id = ?", base.Add(time.Minute), attempts["student2"])
	e.db.Exec("UPDATE quiz_attempts SET completed_at = ? WHERE id = ?", base, attempts["student3"])
	e.setWindow(id, 2*time.Hour, -time.Minute)
	r := e.must("BXH", e.do("GET", "/api/contests/"+cid+"/leaderboard", "student1", ""), 200)
	got := []string{}
	for _, it := range r.items() {
		m := it.(map[string]interface{})
		got = append(got, m["user_name"].(string))
		if (m["user_name"] == "QA-contest student1") != (m["is_me"] == true) {
			t.Fatalf("is_me sai: %v", m)
		}
	}
	want := []string{"QA-contest student2", "QA-contest student1", "QA-contest student3"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("thu tu BXH = %v, muon %v", got, want)
	}
}
