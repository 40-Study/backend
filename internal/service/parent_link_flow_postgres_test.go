package service

import (
	"strings"
	"testing"

	"github.com/google/uuid"
	"study.com/v1/internal/model"
)

// Luồng chính Q4: phụ huynh gửi → con xác nhận → phụ huynh mới thấy dữ liệu con. Trước khi con
// xác nhận (và sau khi con từ chối) mọi API dashboard phải từ chối — chống IDOR.
func TestParentLink_Postgres_ChiThayDuLieuConSauKhiConXacNhan(t *testing.T) {
	f := newParentLinkFixture(t)
	parent := f.user("parent", "PARENT")
	child := f.user("child", "STUDENT")

	// Email viết thường vẫn khớp tài khoản đăng ký có chữ HOA.
	out, err := f.svc.CreateRequest(f.ctx, parent.ID, linkReq(strings.ToLower(child.Email)))
	if err != nil {
		t.Fatalf("gửi yêu cầu: %v", err)
	}
	// Khi còn chờ, phụ huynh chỉ thấy email đã nhập, không thấy tên học sinh (MAJOR-1).
	if out.Status != model.ParentLinkRequestStatusPending || out.Student != nil || out.StudentEmail != strings.ToLower(child.Email) {
		t.Fatalf("yêu cầu vừa gửi sai: %+v", out)
	}
	if f.canSeeChild(parent.ID, child.ID) {
		t.Fatal("IDOR: phụ huynh xem được dữ liệu con khi con CHƯA xác nhận")
	}
	if sent, err := f.svc.ListSent(f.ctx, parent.ID); err != nil || len(sent) != 1 || sent[0].Student != nil {
		t.Fatalf("danh sách đã gửi lộ tên học sinh khi còn chờ: %+v, err=%v", sent, err)
	}
	if rows := f.relationRows(parent.ID, child.ID); len(rows) != 0 {
		t.Fatalf("chưa xác nhận mà đã có %d dòng quan hệ", len(rows))
	}

	incoming, err := f.svc.ListIncoming(f.ctx, child.ID)
	if err != nil || len(incoming) != 1 || incoming[0].Parent == nil || incoming[0].Parent.ID != parent.ID.String() {
		t.Fatalf("con không thấy yêu cầu đến: %+v, err=%v", incoming, err)
	}

	reqID := uuid.MustParse(out.ID)
	if err := f.svc.Respond(f.ctx, child.ID, reqID, "accept"); err != nil {
		t.Fatalf("con xác nhận: %v", err)
	}
	if !f.canSeeChild(parent.ID, child.ID) {
		t.Fatal("con đã xác nhận nhưng phụ huynh vẫn không xem được")
	}
	rows := f.relationRows(parent.ID, child.ID)
	if len(rows) != 1 || rows[0].Status != model.ParentStudentStatusActive || rows[0].ConfirmedBy == nil || *rows[0].ConfirmedBy != "student" {
		t.Fatalf("quan hệ sau xác nhận sai: %+v", rows)
	}
	// Con đã trả lời thì phụ huynh mới thấy tên con trong danh sách đã gửi.
	if sent, err := f.svc.ListSent(f.ctx, parent.ID); err != nil || len(sent) != 1 || sent[0].Student == nil ||
		sent[0].Student.ID != child.ID.String() {
		t.Fatalf("sau xác nhận phụ huynh phải thấy học sinh: %+v, err=%v", sent, err)
	}
	// Bấm xác nhận lần 2 không được xử lý lại.
	wantLinkCode(t, f.svc.Respond(f.ctx, child.ID, reqID, "accept"), "LINK_REQUEST_NOT_PENDING")
	// Đã liên kết thì không gửi yêu cầu mới.
	_, err = f.svc.CreateRequest(f.ctx, parent.ID, linkReq(child.Email))
	wantLinkCode(t, err, "LINK_ALREADY_ACTIVE")
}

func TestParentLink_Postgres_ConTuChoiThiPhuHuynhVanKhongXemDuoc(t *testing.T) {
	f := newParentLinkFixture(t)
	parent := f.user("parent", "PARENT")
	child := f.user("child", "STUDENT")
	reqID := uuid.MustParse(f.request(parent, child))

	if err := f.svc.Respond(f.ctx, child.ID, reqID, "reject"); err != nil {
		t.Fatalf("con từ chối: %v", err)
	}
	if f.canSeeChild(parent.ID, child.ID) {
		t.Fatal("IDOR: con đã từ chối mà phụ huynh vẫn xem được dữ liệu")
	}
	sent, err := f.svc.ListSent(f.ctx, parent.ID)
	if err != nil || len(sent) != 1 || sent[0].Status != model.ParentLinkRequestStatusRejected || sent[0].RespondedAt == nil {
		t.Fatalf("phụ huynh phải thấy yêu cầu bị từ chối: %+v, err=%v", sent, err)
	}
	if rows := f.relationRows(parent.ID, child.ID); len(rows) != 0 {
		t.Fatalf("từ chối mà vẫn tạo %d dòng quan hệ", len(rows))
	}
}

// Chỉ đúng người trong cuộc mới thao tác được; người ngoài nhận 404 (không lộ yêu cầu tồn tại).
func TestParentLink_Postgres_NguoiNgoaiKhongThaoTacDuocYeuCau(t *testing.T) {
	f := newParentLinkFixture(t)
	parent := f.user("parent", "PARENT")
	child := f.user("child", "STUDENT")
	otherStudent := f.user("other-student", "STUDENT")
	otherParent := f.user("other-parent", "PARENT")
	reqID := uuid.MustParse(f.request(parent, child))

	wantLinkCode(t, f.svc.Respond(f.ctx, otherStudent.ID, reqID, "accept"), "LINK_REQUEST_NOT_FOUND")
	wantLinkCode(t, f.svc.Respond(f.ctx, parent.ID, reqID, "accept"), "LINK_REQUEST_NOT_FOUND")
	wantLinkCode(t, f.svc.Cancel(f.ctx, otherParent.ID, reqID), "LINK_REQUEST_NOT_FOUND")
	if f.canSeeChild(parent.ID, child.ID) || f.canSeeChild(otherStudent.ID, child.ID) {
		t.Fatal("IDOR: người ngoài tự xác nhận được yêu cầu")
	}

	// Phụ huynh tự rút yêu cầu; con không xác nhận được yêu cầu đã rút.
	if err := f.svc.Cancel(f.ctx, parent.ID, reqID); err != nil {
		t.Fatalf("phụ huynh rút yêu cầu: %v", err)
	}
	wantLinkCode(t, f.svc.Respond(f.ctx, child.ID, reqID, "accept"), "LINK_REQUEST_NOT_PENDING")
	if f.canSeeChild(parent.ID, child.ID) {
		t.Fatal("yêu cầu đã rút mà vẫn liên kết được")
	}
}

func TestParentLink_Postgres_KiemTraVaiTroVaEmail(t *testing.T) {
	f := newParentLinkFixture(t)
	parent := f.user("parent", "PARENT")
	child := f.user("child", "STUDENT")
	teacher := f.user("teacher", "TEACHER")
	notParent := f.user("student-only", "STUDENT")

	_, err := f.svc.CreateRequest(f.ctx, notParent.ID, linkReq(child.Email))
	wantLinkCode(t, err, "PARENT_ROLE_REQUIRED")
	_, err = f.svc.CreateRequest(f.ctx, parent.ID, linkReq(strings.ToUpper(parent.Email)))
	wantLinkCode(t, err, "LINK_SELF")
	// Email lạ / không phải học sinh: KHÔNG còn lỗi riêng (xem parent_link_enumeration_postgres_test.go).
	if _, err := f.svc.CreateRequest(f.ctx, parent.ID, linkReq(teacher.Email)); err != nil {
		t.Fatalf("email giáo viên phải được nhận như mọi email: %v", err)
	}
	// Yêu cầu tới email không phải học sinh: không ai trả lời được, kể cả chủ email đó.
	incoming, err := f.svc.ListIncoming(f.ctx, teacher.ID)
	if err != nil || len(incoming) != 0 {
		t.Fatalf("giáo viên không được thấy yêu cầu liên kết: %+v, err=%v", incoming, err)
	}
	sent, _ := f.svc.ListSent(f.ctx, parent.ID)
	wantLinkCode(t, f.svc.Respond(f.ctx, teacher.ID, uuid.MustParse(sent[0].ID), "accept"), "LINK_REQUEST_NOT_FOUND")
}

// Yêu cầu gửi tới email lúc chưa có tài khoản học sinh: học sinh đăng ký bằng email đó (khác hoa
// thường) thấy yêu cầu và xác nhận được; yêu cầu được gắn id học sinh.
func TestParentLink_Postgres_YeuCauTheoEmailChoTaiKhoanDangKySau(t *testing.T) {
	f := newParentLinkFixture(t)
	parent := f.user("parent", "PARENT")
	email := "qa-r2e-later-" + uuid.NewString()[:8] + "@40study.test"
	out, err := f.svc.CreateRequest(f.ctx, parent.ID, linkReq(email))
	if err != nil {
		t.Fatalf("gửi tới email chưa đăng ký: %v", err)
	}
	child := f.user("later", "STUDENT")
	if err := f.db.Model(&model.User{}).Where("id = ?", child.ID).Update("email", strings.ToUpper(email)).Error; err != nil {
		t.Fatalf("đổi email: %v", err)
	}
	incoming, err := f.svc.ListIncoming(f.ctx, child.ID)
	if err != nil || len(incoming) != 1 || incoming[0].ID != out.ID {
		t.Fatalf("học sinh đăng ký sau phải thấy yêu cầu: %+v, err=%v", incoming, err)
	}
	if err := f.svc.Respond(f.ctx, child.ID, uuid.MustParse(out.ID), "accept"); err != nil {
		t.Fatalf("xác nhận: %v", err)
	}
	if !f.canSeeChild(parent.ID, child.ID) {
		t.Fatal("đã xác nhận mà phụ huynh không xem được")
	}
	var req model.ParentLinkRequest
	f.db.First(&req, "id = ?", out.ID)
	if req.StudentUserID == nil || *req.StudentUserID != child.ID {
		t.Fatalf("yêu cầu phải được gắn id học sinh: %+v", req.StudentUserID)
	}
}

// E2: /parent/children/:id/courses trả instructor_id để phụ huynh nhắn giảng viên của con.
func TestParentDashboard_Postgres_ChildCoursesCoInstructorID(t *testing.T) {
	f := newParentLinkFixture(t)
	parent := f.user("parent", "PARENT")
	child := f.user("child", "STUDENT")
	teacher := f.user("teacher", "TEACHER")
	f.courseWithEnrollment(teacher, child)
	if err := f.svc.Respond(f.ctx, child.ID, uuid.MustParse(f.request(parent, child)), "accept"); err != nil {
		t.Fatalf("con xác nhận: %v", err)
	}

	res, err := f.dash.GetChildCourses(f.ctx, parent.ID, child.ID, 1, 20)
	if err != nil {
		t.Fatalf("GetChildCourses: %v", err)
	}
	if len(res.Courses) != 1 || res.Courses[0].InstructorID != teacher.ID.String() || res.Courses[0].InstructorName == "" {
		t.Fatalf("khoá của con phải có instructor_id=%s: %+v", teacher.ID, res.Courses)
	}
}
