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
	if out.Status != model.ParentLinkRequestStatusPending || out.Student == nil || out.Student.ID != child.ID.String() {
		t.Fatalf("yêu cầu vừa gửi sai: %+v", out)
	}
	if f.canSeeChild(parent.ID, child.ID) {
		t.Fatal("IDOR: phụ huynh xem được dữ liệu con khi con CHƯA xác nhận")
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
	_, err = f.svc.CreateRequest(f.ctx, parent.ID, linkReq("khong-ton-tai-"+uuid.NewString()+"@40study.test"))
	wantLinkCode(t, err, "STUDENT_NOT_FOUND")
	// Email có tài khoản nhưng không phải học sinh: cùng thông điệp với không tồn tại.
	_, err = f.svc.CreateRequest(f.ctx, parent.ID, linkReq(teacher.Email))
	wantLinkCode(t, err, "STUDENT_NOT_FOUND")
	_, err = f.svc.CreateRequest(f.ctx, parent.ID, linkReq(parent.Email))
	wantLinkCode(t, err, "LINK_SELF")
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
