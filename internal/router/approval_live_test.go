package router

import (
	"context"
	"encoding/json"
	"io"
	"net/http/httptest"
	"strings"
	"testing"

	"study.com/v1/internal/constants"
	"study.com/v1/internal/model"
)

type apvResp struct {
	status int
	body   map[string]interface{}
}

func (e *apvEnv) do(t *testing.T, method, path, token, body string, cookies ...string) apvResp {
	t.Helper()
	var r io.Reader
	if body != "" {
		r = strings.NewReader(body)
	}
	req := httptest.NewRequest(method, path, r)
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	for _, c := range cookies {
		req.Header.Add("Cookie", c)
	}
	res, err := e.app.Test(req, -1)
	if err != nil {
		t.Fatalf("%s %s: %v", method, path, err)
	}
	raw, _ := io.ReadAll(res.Body)
	out := apvResp{status: res.StatusCode, body: map[string]interface{}{}}
	_ = json.Unmarshal(raw, &out.body)
	return out
}

func (r apvResp) data() map[string]interface{} {
	d, _ := r.body["data"].(map[string]interface{})
	return d
}

func expectStatus(t *testing.T, what string, got apvResp, want int) {
	t.Helper()
	if got.status != want {
		t.Fatalf("%s: status=%d, muon %d, body=%v", what, got.status, want, got.body)
	}
}

// Người không phải SYSTEM_ADMIN (học viên, giáo viên, ứng viên) KHÔNG gọi được route duyệt nào.
func TestApprovalLive_NonAdminForbidden(t *testing.T) {
	e := newApvEnv(t)
	cid := e.draftCourseID.String()
	uid := e.applicantID.String()
	routes := []struct{ method, path, body string }{
		{"GET", "/api/admin/courses", ""},
		{"POST", "/api/admin/courses/" + cid + "/approve", ""},
		{"POST", "/api/admin/courses/" + cid + "/reject", `{"reason":"x"}`},
		{"GET", "/api/admin/teacher-applications", ""},
		{"POST", "/api/admin/teacher-applications/" + uid + "/approve", ""},
		{"POST", "/api/admin/teacher-applications/" + uid + "/reject", `{"reason":"x"}`},
	}
	for _, tok := range []string{e.studentTok, e.teacherTok, e.applicantTok} {
		for _, rt := range routes {
			expectStatus(t, rt.method+" "+rt.path, e.do(t, rt.method, rt.path, tok, rt.body), 403)
		}
	}
	if e.courses.courses[e.draftCourseID].Status != model.CourseStatusDraft {
		t.Fatal("khoa hoc bi doi trang thai du request bi 403")
	}
	if e.teacherApp.profiles[e.applicantID].ApprovalStatus != model.TeacherApprovalPending {
		t.Fatal("ho so bi doi trang thai du request bi 403")
	}
}

func TestApprovalLive_CourseSubmitApprove(t *testing.T) {
	e := newApvEnv(t)
	cid := e.draftCourseID.String()

	expectStatus(t, "giao vien khac nop khoa khong phai cua minh",
		e.do(t, "POST", "/api/courses/"+cid+"/submit-review", e.otherTeacherTok, ""), 403)

	r := e.do(t, "POST", "/api/courses/"+cid+"/submit-review", e.teacherTok, "")
	expectStatus(t, "chu khoa nop duyet", r, 200)
	if r.data()["status"] != "pending_review" {
		t.Fatalf("status sau nop = %v", r.data()["status"])
	}
	r = e.do(t, "POST", "/api/courses/"+cid+"/submit-review", e.teacherTok, "")
	expectStatus(t, "nop lai khi dang cho duyet", r, 400)
	if r.body["code"] != "INVALID_COURSE_STATUS" {
		t.Fatalf("code = %v", r.body["code"])
	}

	r = e.do(t, "GET", "/api/admin/courses?status=pending_review", e.adminTok, "")
	expectStatus(t, "admin xem hang cho", r, 200)
	list, _ := r.data()["courses"].([]interface{})
	if len(list) != 1 || list[0].(map[string]interface{})["instructor_email"] != "teacher1@demo.com" {
		t.Fatalf("hang cho sai: %v", r.data())
	}
	expectStatus(t, "status loc sai", e.do(t, "GET", "/api/admin/courses?status=bogus", e.adminTok, ""), 400)

	r = e.do(t, "POST", "/api/admin/courses/"+cid+"/approve", e.adminTok, "")
	expectStatus(t, "admin duyet", r, 200)
	c := e.courses.courses[e.draftCourseID]
	if c.Status != model.CourseStatusPublished || c.PublishedAt == nil || c.ReviewedBy == nil || *c.ReviewedBy != e.adminID {
		t.Fatalf("duyet khong ghi du dau vet: %+v", c)
	}
	expectStatus(t, "duyet lan 2", e.do(t, "POST", "/api/admin/courses/"+cid+"/approve", e.adminTok, ""), 400)
}

func TestApprovalLive_CourseRejectNeedsReasonThenResubmit(t *testing.T) {
	e := newApvEnv(t)
	cid := e.draftCourseID.String()
	expectStatus(t, "tu choi khi con draft", e.do(t, "POST", "/api/admin/courses/"+cid+"/reject", e.adminTok, `{"reason":"x"}`), 400)
	expectStatus(t, "nop", e.do(t, "POST", "/api/courses/"+cid+"/submit-review", e.teacherTok, ""), 200)

	for _, body := range []string{`{}`, `{"reason":"   "}`} {
		r := e.do(t, "POST", "/api/admin/courses/"+cid+"/reject", e.adminTok, body)
		expectStatus(t, "tu choi thieu ly do "+body, r, 400)
	}
	if e.courses.courses[e.draftCourseID].Status != model.CourseStatusPendingReview {
		t.Fatal("tu choi thieu ly do van doi trang thai")
	}
	r := e.do(t, "POST", "/api/admin/courses/"+cid+"/reject", e.adminTok, `{"reason":"  Noi dung chua du chi tiet  "}`)
	expectStatus(t, "tu choi co ly do", r, 200)
	c := e.courses.courses[e.draftCourseID]
	if c.Status != model.CourseStatusRejected || c.RejectionReason == nil || *c.RejectionReason != "Noi dung chua du chi tiet" {
		t.Fatalf("tu choi sai: status=%s reason=%v", c.Status, c.RejectionReason)
	}
	expectStatus(t, "giao vien nop lai sau tu choi", e.do(t, "POST", "/api/courses/"+cid+"/submit-review", e.teacherTok, ""), 200)
}

// Quyết định #6: duyệt hồ sơ GV có hiệu lực NGAY — token cũ bị vô hiệu (401 ROLE_CHANGED), web
// refresh được (không đăng nhập lại) và nhận active_role TEACHER, tạo khoá học được ngay.
func TestApprovalLive_TeacherApprovalTakesEffectImmediately(t *testing.T) {
	e := newApvEnv(t)
	uid := e.applicantID.String()
	course := `{"title":"Khoa hoc moi cua ung vien","price":0}`

	expectStatus(t, "ung vien tao khoa truoc khi duyet", e.do(t, "POST", "/api/courses", e.applicantTok, course), 403)
	expectStatus(t, "ung vien xem ho so", e.do(t, "GET", "/api/teacher-profiles/me", e.applicantTok, ""), 200)

	r := e.do(t, "POST", "/api/admin/teacher-applications/"+uid+"/approve", e.adminTok, "")
	expectStatus(t, "admin duyet ho so", r, 200)
	if !e.usr.hasRole(e.applicantID, "TEACHER") || e.usr.hasRole(e.applicantID, "TEACHER_APPLICANT") {
		t.Fatalf("role sau duyet sai: %v", e.usr.roles[e.applicantID])
	}

	r = e.do(t, "GET", "/api/teacher-profiles/me", e.applicantTok, "")
	expectStatus(t, "token cu sau duyet", r, 401)
	if r.body["code"] != "ROLE_CHANGED" {
		t.Fatalf("token cu phai bao ROLE_CHANGED, body=%v", r.body)
	}

	r = e.do(t, "POST", "/api/auth/refresh-token", "", "", "rfToken="+e.applicantRefresh)
	expectStatus(t, "refresh sau khi doi vai tro", r, 200)
	if r.data()["role_changed"] != true || r.data()["active_role"] != "TEACHER" {
		t.Fatalf("refresh phai tra role_changed + active_role TEACHER: %v", r.data())
	}
	newTok, _ := r.data()["access_token"].(string)
	expectStatus(t, "tao khoa bang token moi", e.do(t, "POST", "/api/courses", newTok, course), 201)
	if len(e.courseRepo.created) != 1 {
		t.Fatalf("khoa hoc chua duoc tao: %d", len(e.courseRepo.created))
	}
}

// Marker đổi vai trò KHÔNG được mở đường cho token đã bị thu hồi vì lý do khác: nếu sau khi duyệt
// user_version lại bị bump (đăng xuất mọi nơi/khoá tài khoản) thì refresh phải bị từ chối.
func TestApprovalLive_RoleChangeRefreshDeniedAfterLaterRevoke(t *testing.T) {
	e := newApvEnv(t)
	expectStatus(t, "duyet", e.do(t, "POST", "/api/admin/teacher-applications/"+e.applicantID.String()+"/approve", e.adminTok, ""), 200)
	if err := e.rdb.Incr(context.Background(), constants.KeyUserVersion(e.applicantID.String())).Err(); err != nil {
		t.Fatal(err)
	}
	r := e.do(t, "POST", "/api/auth/refresh-token", "", "", "rfToken="+e.applicantRefresh)
	expectStatus(t, "refresh sau khi bi thu hoi tiep", r, 401)
	r = e.do(t, "GET", "/api/teacher-profiles/me", e.applicantTok, "")
	if r.status != 401 || r.body["code"] == "ROLE_CHANGED" {
		t.Fatalf("khong duoc bao ROLE_CHANGED khi lan bump cuoi khong phai doi vai tro: %d %v", r.status, r.body)
	}
}

// Quyết định #5: từ chối bắt lý do; nộp lại tối đa 3 lần, lần nộp lại thứ 4 bị chặn.
func TestApprovalLive_TeacherResubmitLimit(t *testing.T) {
	e := newApvEnv(t)
	uid := e.applicantID.String()
	expectStatus(t, "tu choi thieu ly do", e.do(t, "POST", "/api/admin/teacher-applications/"+uid+"/reject", e.adminTok, `{"reason":""}`), 400)
	expectStatus(t, "nop lai khi dang pending", e.do(t, "POST", "/api/teacher-profiles/me/resubmit", e.applicantTok, ""), 400)

	for i := 1; i <= model.MaxTeacherResubmissions; i++ {
		expectStatus(t, "tu choi", e.do(t, "POST", "/api/admin/teacher-applications/"+uid+"/reject", e.adminTok, `{"reason":"Chua du minh chung"}`), 200)
		r := e.do(t, "POST", "/api/teacher-profiles/me/resubmit", e.applicantTok, "")
		expectStatus(t, "nop lai hop le", r, 200)
		if int(r.data()["resubmission_count"].(float64)) != i {
			t.Fatalf("resubmission_count=%v, muon %d", r.data()["resubmission_count"], i)
		}
	}
	expectStatus(t, "tu choi lan 4", e.do(t, "POST", "/api/admin/teacher-applications/"+uid+"/reject", e.adminTok, `{"reason":"Van thieu"}`), 200)
	r := e.do(t, "GET", "/api/teacher-profiles/me", e.applicantTok, "")
	if r.data()["can_resubmit"] != false || r.data()["rejection_reason"] != "Van thieu" {
		t.Fatalf("GET /me sau lan tu choi thu 4 sai: %v", r.data())
	}
	r = e.do(t, "POST", "/api/teacher-profiles/me/resubmit", e.applicantTok, "")
	expectStatus(t, "nop lai lan thu 4", r, 400)
	if r.body["code"] != "RESUBMISSION_LIMIT_REACHED" {
		t.Fatalf("code = %v", r.body["code"])
	}
	if e.teacherApp.profiles[e.applicantID].ApprovalStatus != model.TeacherApprovalRejected {
		t.Fatal("lan nop lai bi chan van doi trang thai")
	}
}
