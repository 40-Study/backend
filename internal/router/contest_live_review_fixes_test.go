package router

// Test cho các finding của review PR #82 (plans/reports/review-260928-contest-pr82.md). Mỗi test
// tái hiện đúng kịch bản của reviewer và phải ĐỎ khi revert bản sửa tương ứng.

import (
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"study.com/v1/internal/model"
)

// MAJOR: 2 giây sau end_time, thí sinh đã nộp KHÔNG được đọc đáp án / BXH trong khi người khác
// vẫn còn ân hạn để nộp. Đáp án và BXH mở từ end_time + ContestSubmitGraceSeconds.
func TestContestLive_AnswersAndLeaderboardClosedDuringGrace(t *testing.T) {
	e := newCtEnv(t)
	id, _, key := e.publishedContest("")
	e.setWindow(id, time.Minute, time.Hour)
	cid := id.String()
	attempts := map[string]string{}
	for _, who := range []string{"student1", "student2"} {
		e.must("join "+who, e.do("POST", "/api/contests/"+cid+"/join", who, ""), 201)
		attempts[who] = e.must("start "+who, e.do("POST", "/api/contests/"+cid+"/start", who, ""), 200).data()["attempt_id"].(string)
	}
	e.must("submit1", e.do("POST", "/api/contests/"+cid+"/submit", "student1", answersBody(attempts["student1"], key, 3)), 200)

	e.setWindow(id, time.Hour, -2*time.Second) // vừa hết giờ 2 giây, còn trong ân hạn
	res := e.must("my-result trong an han", e.do("GET", "/api/contests/"+cid+"/my-result", "student1", ""), 200)
	assertNoAnswerLeak(t, "my-result trong an han", res.raw)
	if res.data()["questions"] != nil {
		t.Fatalf("my-result trong an han phai questions=null: %s", res.raw)
	}
	var c model.Contest
	e.db.First(&c, "id = ?", id)
	at, err := time.Parse(time.RFC3339Nano, res.data()["answers_available_at"].(string))
	if err != nil || !at.Equal(c.EndTime.Add(model.ContestSubmitGraceSeconds*time.Second)) {
		t.Fatalf("answers_available_at = %v, muon end_time + %ds (%v)", res.data()["answers_available_at"], model.ContestSubmitGraceSeconds, c.EndTime)
	}
	for _, who := range []string{"", "student1"} {
		e.must("BXH trong an han "+who, e.do("GET", "/api/contests/"+cid+"/leaderboard", who, ""), 403, "CONTEST_LEADERBOARD_HIDDEN")
	}
	e.must("chu van xem BXH", e.do("GET", "/api/contests/"+cid+"/leaderboard", "teacherA", ""), 200)
	// Người thứ hai vẫn nộp được trong ân hạn — đây là lý do không được mở đáp án lúc end_time.
	e.must("student2 nop trong an han", e.do("POST", "/api/contests/"+cid+"/submit", "student2", answersBody(attempts["student2"], key, 3)), 200)

	e.setWindow(id, time.Hour, -time.Duration(model.ContestSubmitGraceSeconds+1)*time.Second)
	res = e.must("my-result sau an han", e.do("GET", "/api/contests/"+cid+"/my-result", "student1", ""), 200)
	if qs, _ := res.data()["questions"].([]interface{}); len(qs) != 3 {
		t.Fatalf("my-result sau an han phai co dap an: %s", res.raw)
	}
	e.must("BXH sau an han", e.do("GET", "/api/contests/"+cid+"/leaderboard", "", ""), 200)
}

// MINOR: sửa cuộc thi đã PUBLISHED bằng body HỢP LỆ → 409, dữ liệu không đổi.
func TestContestLive_UpdatePublishedRejected(t *testing.T) {
	e := newCtEnv(t)
	quizID, _ := e.newQuiz("teacherA")
	r := e.must("tao", e.do("POST", "/api/contests", "teacherA", contestBody(quizID, "")), 201)
	cid := r.data()["id"].(string)
	e.must("gui duyet", e.do("POST", "/api/contests/"+cid+"/submit-review", "teacherA", ""), 200)
	e.must("duyet", e.do("POST", "/api/admin/contests/"+cid+"/approve", "admin", ""), 200)
	e.must("sua khi PUBLISHED", e.do("PUT", "/api/contests/"+cid, "teacherA",
		contestBody(quizID, `"description":"QA-contest doi sau duyet"`)), 409, "CONTEST_INVALID_STATUS")
	var n int64
	e.db.Model(&model.Contest{}).Where("id = ? AND description = ?", cid, "QA-contest doi sau duyet").Count(&n)
	if n != 0 {
		t.Fatal("cuoc thi PUBLISHED bi sua")
	}
}

func (e *ctEnv) prizeCount(id uuid.UUID) int64 {
	var n int64
	e.db.Model(&model.ContestPrize{}).Where("contest_id = ?", id).Count(&n)
	return n
}

// MINOR: sau khi chốt, admin không đổi được giải (409), giải đã phát giữ nguyên.
func TestContestLive_PrizesAfterFinalizeRejected(t *testing.T) {
	e := newCtEnv(t)
	id := e.finalizeFixture(e.newVoucher(true))
	e.must("chot", e.do("POST", "/api/admin/contests/"+id.String()+"/finalize", "admin", ""), 200)
	e.must("sua giai sau chot", e.do("PUT", "/api/admin/contests/"+id.String()+"/prizes", "admin",
		`{"prizes":[{"rank_from":1,"rank_to":3,"grant_certificate":true}]}`), 409, "CONTEST_INVALID_STATUS")
	if n := e.prizeCount(id); n != 2 {
		t.Fatalf("giai sau chot bi doi: %d dong, muon 2", n)
	}
}

// MINOR: PUT prizes với body {} giữ nguyên giải (giống approve); mảng rỗng tường minh xoá hết.
func TestContestLive_UpdatePrizesEmptyObjectKeeps(t *testing.T) {
	e := newCtEnv(t)
	quizID, _ := e.newQuiz("teacherA")
	r := e.must("tao", e.do("POST", "/api/contests", "teacherA", contestBody(quizID, "")), 201)
	cid := r.data()["id"].(string)
	id := uuid.MustParse(cid)
	e.must("gui duyet", e.do("POST", "/api/contests/"+cid+"/submit-review", "teacherA", ""), 200)
	e.must("duyet kem giai", e.do("POST", "/api/admin/contests/"+cid+"/approve", "admin",
		`{"prizes":[{"rank_from":1,"rank_to":1,"grant_certificate":true},{"rank_from":2,"rank_to":3,"grant_certificate":true}]}`), 200)
	path := "/api/admin/contests/" + cid + "/prizes"
	e.must("body {}", e.do("PUT", path, "admin", `{}`), 200)
	if n := e.prizeCount(id); n != 2 {
		t.Fatalf("body {} lam doi giai: %d dong, muon 2", n)
	}
	e.must("mang rong", e.do("PUT", path, "admin", `{"prizes":[]}`), 200)
	if n := e.prizeCount(id); n != 0 {
		t.Fatalf("prizes [] phai xoa het giai: con %d dong", n)
	}
}

// MINOR: danh sách admin KHÔNG hiện bản nháp (DRAFT) của giảng viên khác; bài chờ duyệt thì có.
func TestContestLive_AdminListHidesOtherTeachersDrafts(t *testing.T) {
	e := newCtEnv(t)
	quizDraft, _ := e.newQuiz("teacherB")
	draft := e.must("nhap GV B", e.do("POST", "/api/contests", "teacherB", contestBody(quizDraft, "")), 201).data()["id"].(string)
	quizPending, _ := e.newQuiz("teacherB")
	pending := e.must("tao GV B", e.do("POST", "/api/contests", "teacherB", contestBody(quizPending, "")), 201).data()["id"].(string)
	e.must("gui duyet", e.do("POST", "/api/contests/"+pending+"/submit-review", "teacherB", ""), 200)

	r := e.must("admin list", e.do("GET", "/api/admin/contests?limit=100", "admin", ""), 200)
	if strings.Contains(r.raw, draft) {
		t.Fatalf("admin list lo ban nhap cua GV khac: %s", r.raw)
	}
	if !strings.Contains(r.raw, pending) {
		t.Fatalf("admin list thieu cuoc thi cho duyet: %s", r.raw)
	}
}
