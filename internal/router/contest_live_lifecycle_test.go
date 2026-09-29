package router

import (
	"testing"
	"time"
)

func (e *ctEnv) publicSlugs(query string) map[string]bool {
	e.t.Helper()
	r := e.must("danh sach cong khai", e.do("GET", "/api/contests"+query, "", ""), 200)
	out := map[string]bool{}
	for _, it := range r.items() {
		out[it.(map[string]interface{})["slug"].(string)] = true
	}
	return out
}

// Luồng duyệt: tạo → gửi duyệt → từ chối (bắt buộc lý do) → sửa → gửi lại → duyệt; huỷ.
// (c) Suốt quá trình, danh sách công khai và /:slug KHÔNG lộ DRAFT/PENDING_REVIEW/REJECTED/CANCELLED.
func TestContestLive_ReviewRejectApproveCancel(t *testing.T) {
	e := newCtEnv(t)
	quizID, _ := e.newQuiz("teacherA")
	r := e.must("tao", e.do("POST", "/api/contests", "teacherA", contestBody(quizID, "")), 201)
	cid, slug := r.data()["id"].(string), r.data()["slug"].(string)
	if r.data()["status"] != "DRAFT" || r.data()["phase"] != "DRAFT" || r.data()["type"] != "QUIZ" {
		t.Fatalf("tao sai trang thai: %s", r.raw)
	}
	hidden := func(state string) {
		t.Helper()
		if e.publicSlugs("")[slug] {
			t.Fatalf("%s: lo trong danh sach cong khai", state)
		}
		for _, who := range []string{"", "student1", "teacherB"} {
			e.must(state+" slug voi "+who, e.do("GET", "/api/contests/"+slug, who, ""), 404, "CONTEST_NOT_FOUND")
		}
		e.must(state+" chu van xem duoc", e.do("GET", "/api/contests/"+slug, "teacherA", ""), 200)
	}
	hidden("DRAFT")

	e.must("admin duyet ban nhap cua GV", e.do("POST", "/api/admin/contests/"+cid+"/approve", "admin", ""), 409, "CONTEST_INVALID_STATUS")
	r = e.must("gui duyet", e.do("POST", "/api/contests/"+cid+"/submit-review", "teacherA", ""), 200)
	if r.data()["status"] != "PENDING_REVIEW" || r.data()["submitted_at"] == nil {
		t.Fatalf("gui duyet sai: %s", r.raw)
	}
	hidden("PENDING_REVIEW")
	e.must("sua khi cho duyet", e.do("PUT", "/api/contests/"+cid, "teacherA", contestBody(quizID, "")), 409, "CONTEST_INVALID_STATUS")

	e.must("tu choi thieu ly do", e.do("POST", "/api/admin/contests/"+cid+"/reject", "admin", `{"reason":"   "}`), 400, "REASON_REQUIRED")
	r = e.must("tu choi", e.do("POST", "/api/admin/contests/"+cid+"/reject", "admin", `{"reason":" Thieu mo ta "}`), 200)
	if r.data()["status"] != "REJECTED" || r.data()["reject_reason"] != "Thieu mo ta" {
		t.Fatalf("tu choi sai: %s", r.raw)
	}
	hidden("REJECTED")

	e.must("sua sau tu choi", e.do("PUT", "/api/contests/"+cid, "teacherA", contestBody(quizID, `"description":"Da bo sung"`)), 200)
	e.must("gui lai", e.do("POST", "/api/contests/"+cid+"/submit-review", "teacherA", ""), 200)
	r = e.must("duyet", e.do("POST", "/api/admin/contests/"+cid+"/approve", "admin", ""), 200)
	if r.data()["status"] != "PUBLISHED" || r.data()["phase"] != "UPCOMING" || r.data()["reject_reason"] != nil {
		t.Fatalf("duyet sai: %s", r.raw)
	}
	if !e.publicSlugs("")[slug] {
		t.Fatal("cuoc thi da duyet khong co trong danh sach cong khai")
	}
	e.must("khach xem", e.do("GET", "/api/contests/"+slug, "", ""), 200)
	e.must("duyet lan 2", e.do("POST", "/api/admin/contests/"+cid+"/approve", "admin", ""), 409, "CONTEST_INVALID_STATUS")

	e.must("huy thieu ly do", e.do("POST", "/api/admin/contests/"+cid+"/cancel", "admin", `{}`), 400, "REASON_REQUIRED")
	r = e.must("huy", e.do("POST", "/api/admin/contests/"+cid+"/cancel", "admin", `{"reason":"Trung lich"}`), 200)
	if r.data()["status"] != "CANCELLED" || r.data()["cancel_reason"] != "Trung lich" {
		t.Fatalf("huy sai: %s", r.raw)
	}
	hidden("CANCELLED")
	e.must("join khi da huy", e.do("POST", "/api/contests/"+cid+"/join", "student1", ""), 404, "CONTEST_NOT_FOUND")
	e.must("huy lan 2", e.do("POST", "/api/admin/contests/"+cid+"/cancel", "admin", `{"reason":"x"}`), 409, "CONTEST_INVALID_STATUS")
}

// Quy tắc tạo: quiz đã dùng, quiz của người khác, voucher bởi GV, lịch sai; xoá giải phóng quiz.
func TestContestLive_CreateRules(t *testing.T) {
	e := newCtEnv(t)
	quizA, _ := e.newQuiz("teacherA")
	quizB, _ := e.newQuiz("teacherB")
	r := e.must("tao", e.do("POST", "/api/contests", "teacherA", contestBody(quizA, "")), 201)
	e.must("quiz da dung", e.do("POST", "/api/contests", "teacherA", contestBody(quizA, "")), 409, "CONTEST_QUIZ_IN_USE")
	e.must("quiz cua GV khac", e.do("POST", "/api/contests", "teacherA", contestBody(quizB, "")), 400, "CONTEST_QUIZ_INVALID")
	voucher := e.newVoucher(true).String()
	e.must("GV gan voucher", e.do("POST", "/api/contests", "teacherB", contestBody(quizB,
		`"prizes":[{"rank_from":1,"rank_to":1,"grant_certificate":true,"voucher_id":"`+voucher+`"}]`)), 403, "CONTEST_VOUCHER_ADMIN_ONLY")
	e.must("giai chong khoang", e.do("POST", "/api/contests", "teacherB", contestBody(quizB,
		`"prizes":[{"rank_from":1,"rank_to":3,"grant_certificate":true},{"rank_from":3,"rank_to":5,"grant_certificate":true}]`)), 400, "CONTEST_PRIZES_INVALID")
	past := `{"title":"QA-contest qua khu","quiz_id":"` + quizB.String() + `","start_time":"2020-01-01T00:00:00Z","end_time":"2020-01-02T00:00:00Z","duration_minutes":30,"max_participants":0,"is_public":true,"prizes":[]}`
	e.must("lich qua khu", e.do("POST", "/api/contests", "teacherB", past), 400, "CONTEST_INVALID_SCHEDULE")

	e.must("xoa ban nhap", e.do("DELETE", "/api/contests/"+r.data()["id"].(string), "teacherA", ""), 200)
	e.must("quiz duoc giai phong", e.do("POST", "/api/contests", "teacherA", contestBody(quizA, "")), 201)
}

// Phase theo thời gian (không có job): UPCOMING cho đăng ký nhưng chưa cho làm; ACTIVE cho làm;
// ENDED đóng đăng ký và mở BXH. BXH trước khi đóng bị ẩn với mọi người trừ chủ/admin.
func TestContestLive_PhaseByTimeAndLeaderboardVisibility(t *testing.T) {
	e := newCtEnv(t)
	id, slug, correct := e.publishedContest("")
	cid := id.String()
	lb := "/api/contests/" + cid + "/leaderboard"

	r := e.must("chi tiet UPCOMING", e.do("GET", "/api/contests/"+slug, "", ""), 200)
	if r.data()["phase"] != "UPCOMING" || r.data()["viewer"].(map[string]interface{})["join_block_reason"] != "LOGIN_REQUIRED" {
		t.Fatalf("khach xem UPCOMING sai: %s", r.raw)
	}
	e.must("join UPCOMING", e.do("POST", "/api/contests/"+cid+"/join", "student1", ""), 201)
	e.must("start UPCOMING", e.do("POST", "/api/contests/"+cid+"/start", "student1", ""), 409, "CONTEST_NOT_ACTIVE")
	e.must("BXH UPCOMING khach", e.do("GET", lb, "", ""), 403, "CONTEST_LEADERBOARD_HIDDEN")

	e.setWindow(id, time.Minute, time.Hour)
	if e.must("list ACTIVE", e.do("GET", "/api/contests?phase=ACTIVE", "", ""), 200); !e.publicSlugs("?phase=ACTIVE")[slug] || e.publicSlugs("?phase=UPCOMING")[slug] {
		t.Fatal("loc phase ACTIVE/UPCOMING sai")
	}
	st := e.must("start ACTIVE", e.do("POST", "/api/contests/"+cid+"/start", "student1", ""), 200)
	e.must("submit", e.do("POST", "/api/contests/"+cid+"/submit", "student1", answersBody(st.data()["attempt_id"].(string), correct, 1)), 200)
	for _, who := range []string{"", "student1", "student2", "teacherB", "parent"} {
		e.must("BXH ACTIVE "+who, e.do("GET", lb, who, ""), 403, "CONTEST_LEADERBOARD_HIDDEN")
	}
	e.must("BXH ACTIVE chu", e.do("GET", lb, "teacherA", ""), 200)
	e.must("BXH ACTIVE admin", e.do("GET", lb, "admin", ""), 200)

	e.setWindow(id, 2*time.Hour, -time.Minute)
	e.must("join ENDED", e.do("POST", "/api/contests/"+cid+"/join", "student2", ""), 409, "CONTEST_CLOSED")
	r = e.must("BXH ENDED khach", e.do("GET", lb, "", ""), 200)
	if len(r.items()) != 1 || r.data()["finalized"] != false {
		t.Fatalf("BXH ENDED sai: %s", r.raw)
	}
	r = e.must("chi tiet ENDED", e.do("GET", "/api/contests/"+slug, "student2", ""), 200)
	if r.data()["phase"] != "ENDED" || r.data()["viewer"].(map[string]interface{})["join_block_reason"] != "CLOSED" {
		t.Fatalf("chi tiet ENDED sai: %s", r.raw)
	}
	if !e.publicSlugs("?phase=ENDED")[slug] {
		t.Fatal("loc phase ENDED sai")
	}
	e.must("phase khong hop le", e.do("GET", "/api/contests?phase=DRAFT", "", ""), 400)
}
