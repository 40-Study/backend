package router

import (
	"strings"
	"testing"
	"time"

	"study.com/v1/internal/model"
)

// (a) Ma trận quyền: MỌI endpoint contract §2.2 × {khách, học viên, phụ huynh, GV chủ, GV khác,
// admin}. Cuộc thi P đang ACTIVE, do teacherA tạo. Mã mong đợi là kết quả SAU cả middleware lẫn
// service, nên một endpoint lỡ thiếu RequirePermissions/ownership sẽ lệch ô tương ứng.
func TestContestLive_PermissionMatrix(t *testing.T) {
	e := newCtEnv(t)
	id, slug, _ := e.publishedContest("")
	e.setWindow(id, time.Minute, 2*time.Hour)
	cid := id.String()
	bogus := `{"attempt_id":"` + cid + `","answers":[]}`

	type row struct{ method, path, body string }
	rows := []row{
		{"GET", "/api/contests", ""}, {"GET", "/api/contests/me", ""},
		{"GET", "/api/contests/manage", ""}, {"GET", "/api/contests/manage/quiz-options", ""},
		{"GET", "/api/contests/manage/" + cid, ""}, {"GET", "/api/contests/manage/" + cid + "/participants", ""},
		{"GET", "/api/contests/" + slug, ""}, {"POST", "/api/contests", `{}`},
		{"PUT", "/api/contests/" + cid, `{}`}, {"DELETE", "/api/contests/" + cid, ""},
		{"POST", "/api/contests/" + cid + "/submit-review", ""}, {"POST", "/api/contests/" + cid + "/join", ""},
		{"POST", "/api/contests/" + cid + "/start", ""}, {"POST", "/api/contests/" + cid + "/submit", bogus},
		{"GET", "/api/contests/" + cid + "/my-result", ""}, {"GET", "/api/contests/" + cid + "/leaderboard", ""},
		{"GET", "/api/contests/" + cid + "/certificate", ""}, {"GET", "/api/admin/contests", ""},
		{"POST", "/api/admin/contests/" + cid + "/approve", ""}, {"POST", "/api/admin/contests/" + cid + "/reject", `{"reason":"x"}`},
		{"POST", "/api/admin/contests/" + cid + "/cancel", `{}`}, {"PUT", "/api/admin/contests/" + cid + "/prizes", `{"prizes":[]}`},
		{"POST", "/api/admin/contests/" + cid + "/finalize", ""},
	}
	// Thứ tự cột khớp rows ở trên. student1 join (201) TRƯỚC start (200); submit với attempt sai → 409.
	want := map[string][]int{
		"":         {200, 401, 401, 401, 401, 401, 200, 401, 401, 401, 401, 401, 401, 401, 401, 403, 401, 401, 401, 401, 401, 401, 401},
		"student1": {200, 200, 403, 403, 403, 403, 200, 403, 403, 403, 403, 201, 200, 409, 404, 403, 404, 403, 403, 403, 403, 403, 403},
		"parent":   {200, 200, 403, 403, 403, 403, 200, 403, 403, 403, 403, 403, 403, 403, 404, 403, 404, 403, 403, 403, 403, 403, 403},
		"teacherA": {200, 200, 200, 200, 200, 200, 200, 400, 400, 409, 409, 403, 403, 403, 404, 200, 404, 403, 403, 403, 403, 403, 403},
		"teacherB": {200, 200, 200, 200, 404, 404, 200, 400, 400, 403, 403, 403, 403, 403, 404, 403, 404, 403, 403, 403, 403, 403, 403},
		"admin":    {200, 200, 200, 200, 200, 200, 200, 400, 400, 409, 403, 403, 403, 403, 404, 200, 404, 200, 409, 409, 400, 200, 409},
	}
	for _, who := range []string{"", "student1", "parent", "teacherA", "teacherB", "admin"} {
		for i, r := range rows {
			got := e.do(r.method, r.path, who, r.body)
			if got.status != want[who][i] {
				t.Errorf("[%s] %s %s: status=%d muon %d body=%s", who, r.method, r.path, got.status, want[who][i], got.raw)
			}
		}
	}
	var c model.Contest
	e.db.First(&c, "id = ?", id)
	if c.Status != model.ContestStatusPublished || c.FinalizedAt != nil {
		t.Fatalf("ma tran quyen lam doi trang thai cuoc thi: status=%s finalized=%v", c.Status, c.FinalizedAt)
	}
}

// (b) IDOR: GV B không xem/sửa/xoá/gửi duyệt/chốt/xem người tham gia cuộc thi của GV A.
func TestContestLive_IDORTeacherB(t *testing.T) {
	e := newCtEnv(t)
	quizID, _ := e.newQuiz("teacherA")
	r := e.must("A tao", e.do("POST", "/api/contests", "teacherA", contestBody(quizID, "")), 201)
	cid := r.data()["id"].(string)
	body := contestBody(quizID, "")

	e.must("B xem ban quan ly", e.do("GET", "/api/contests/manage/"+cid, "teacherB", ""), 404, "CONTEST_NOT_FOUND")
	e.must("B xem nguoi tham gia", e.do("GET", "/api/contests/manage/"+cid+"/participants", "teacherB", ""), 404, "CONTEST_NOT_FOUND")
	e.must("B sua", e.do("PUT", "/api/contests/"+cid, "teacherB", body), 403, "CONTEST_FORBIDDEN")
	e.must("B xoa", e.do("DELETE", "/api/contests/"+cid, "teacherB", ""), 403, "CONTEST_FORBIDDEN")
	e.must("B gui duyet", e.do("POST", "/api/contests/"+cid+"/submit-review", "teacherB", ""), 403, "CONTEST_FORBIDDEN")
	e.must("B chot", e.do("POST", "/api/admin/contests/"+cid+"/finalize", "teacherB", ""), 403)
	e.must("B xem ban nhap qua slug", e.do("GET", "/api/contests/"+r.data()["slug"].(string), "teacherB", ""), 404)
	list := e.must("B danh sach quan ly", e.do("GET", "/api/contests/manage", "teacherB", ""), 200)
	if len(list.items()) != 0 {
		t.Fatalf("B thay cuoc thi cua A trong /manage: %s", list.raw)
	}
	var c model.Contest
	e.db.First(&c, "id = ?", cid)
	if c.Status != model.ContestStatusDraft || c.Title != "QA-contest thi thu" {
		t.Fatalf("cuoc thi cua A bi B thay doi: %+v", c)
	}
	// Chủ vẫn làm được; admin (không phải chủ) cũng xem được bản quản lý.
	e.must("A xem", e.do("GET", "/api/contests/manage/"+cid, "teacherA", ""), 200)
	e.must("admin xem", e.do("GET", "/api/contests/manage/"+cid, "admin", ""), 200)
}

// (b) Học viên chỉ đọc kết quả/chứng nhận của CHÍNH mình — không có tham số user nào để đổi.
func TestContestLive_StudentOnlyOwnResult(t *testing.T) {
	e := newCtEnv(t)
	id, _, correct := e.publishedContest("")
	e.setWindow(id, time.Minute, time.Hour)
	cid := id.String()
	e.must("join", e.do("POST", "/api/contests/"+cid+"/join", "student1", ""), 201)
	st := e.must("start", e.do("POST", "/api/contests/"+cid+"/start", "student1", ""), 200)
	e.must("submit", e.do("POST", "/api/contests/"+cid+"/submit", "student1", answersBody(st.data()["attempt_id"].(string), correct, 2)), 200)
	e.must("student1 xem ket qua", e.do("GET", "/api/contests/"+cid+"/my-result", "student1", ""), 200)
	e.must("student2 khong co ket qua", e.do("GET", "/api/contests/"+cid+"/my-result", "student2", ""), 404, "CONTEST_RESULT_NOT_FOUND")
	// student2 dùng attempt_id của student1 → không phải attempt của mình.
	e.must("student2 join", e.do("POST", "/api/contests/"+cid+"/join", "student2", ""), 201)
	e.must("student2 nop bai ho", e.do("POST", "/api/contests/"+cid+"/submit", "student2",
		answersBody(st.data()["attempt_id"].(string), correct, 2)), 409, "CONTEST_ATTEMPT_MISMATCH")
}

// (k) Route tĩnh không bị "/:slug" nuốt; route cũ (lộ đáp án) trả 404.
func TestContestLive_RouterOrderAndLegacyRoutesGone(t *testing.T) {
	e := newCtEnv(t)
	e.newQuiz("teacherA")
	if r := e.must("/me", e.do("GET", "/api/contests/me", "student1", ""), 200); r.data()["items"] == nil {
		t.Fatalf("/me bi route slug nuot: %s", r.raw)
	}
	if r := e.must("/manage", e.do("GET", "/api/contests/manage", "teacherA", ""), 200); r.data()["items"] == nil {
		t.Fatalf("/manage bi route slug nuot: %s", r.raw)
	}
	r := e.must("/manage/quiz-options", e.do("GET", "/api/contests/manage/quiz-options", "teacherA", ""), 200)
	if opts, _ := r.body["data"].([]interface{}); len(opts) != 1 {
		t.Fatalf("quiz-options phai co 1 quiz cua teacherA: %s", r.raw)
	}
	cid, _, _ := e.publishedContest("")
	id := cid.String()
	pid := "00000000-0000-0000-0000-000000000001"
	for _, rt := range [][2]string{
		{"POST", "/api/contests/" + id + "/publish"}, {"GET", "/api/contests/" + id + "/problems"},
		{"POST", "/api/contests/" + id + "/problems"}, {"PUT", "/api/contests/" + id + "/problems/" + pid},
		{"DELETE", "/api/contests/" + id + "/problems/" + pid}, {"POST", "/api/contests/" + id + "/problems/" + pid + "/submit"},
		{"GET", "/api/contests/" + id + "/submissions/me"},
	} {
		e.must("route cu "+rt[0]+" "+rt[1], e.do(rt[0], rt[1], "admin", `{}`), 404)
	}
}

// (l) CHECK chk_contests_status trên DB khớp model.ContestStatuses: mọi giá trị SSOT ghi được,
// giá trị cũ (UPCOMING/ACTIVE/ENDED) bị từ chối.
func TestContestLive_StatusCheckConstraint(t *testing.T) {
	e := newCtEnv(t)
	id, _, _ := e.publishedContest("")
	for _, s := range model.ContestStatuses {
		if err := e.db.Exec("UPDATE contests SET status = ? WHERE id = ?", s, id).Error; err != nil {
			t.Errorf("status %s hop le nhung DB tu choi: %v", s, err)
		}
	}
	for _, s := range []string{"UPCOMING", "ACTIVE", "ENDED", "published"} {
		err := e.db.Exec("UPDATE contests SET status = ? WHERE id = ?", s, id).Error
		if err == nil || !strings.Contains(err.Error(), "chk_contests_status") {
			t.Errorf("status %s phai bi chk_contests_status chan, err=%v", s, err)
		}
	}
}
