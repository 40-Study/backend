package service

import (
	"errors"
	"testing"

	"github.com/google/uuid"
	"study.com/v1/internal/model"
)

// Quyết định D8 (đảo thiết kế chống dò của PR #81): email không có tài khoản HỌC SINH — dù không
// tồn tại hay là tài khoản giáo viên — nhận CÙNG một 404 STUDENT_NOT_FOUND và KHÔNG tạo dòng nào.
// Lần gửi vẫn bị tính vào hạn mức ngày (giới hạn dò email).

func countLinkRows(t *testing.T, f *parentLinkFixture, parentID uuid.UUID) int64 {
	t.Helper()
	var n int64
	if err := f.db.Model(&model.ParentLinkRequest{}).Where("parent_user_id = ?", parentID).Count(&n).Error; err != nil {
		t.Fatalf("đếm yêu cầu: %v", err)
	}
	return n
}

func errOf[T any](_ T, err error) error { return err }

func wantStudentNotFound(t *testing.T, err error) *ParentLinkError {
	t.Helper()
	var le *ParentLinkError
	if !errors.As(err, &le) || le.Status != 404 || le.Code != "STUDENT_NOT_FOUND" {
		t.Fatalf("muốn 404 STUDENT_NOT_FOUND, nhận %v", err)
	}
	return le
}

func TestParentLinkStudentNotFound_NonexistentEmail404NoRowButAttemptCounted(t *testing.T) {
	f := newParentLinkFixture(t)
	parent := f.user("parent", "PARENT")
	ghost := "khong-ton-tai-" + uuid.NewString()[:8] + "@40study.test"

	for i := 0; i < ParentLinkDailyLimit; i++ {
		_, err := f.svc.CreateRequest(f.ctx, parent.ID, linkReq(ghost))
		wantStudentNotFound(t, err)
	}
	if n := countLinkRows(t, f, parent.ID); n != 0 {
		t.Fatalf("email không tồn tại mà vẫn tạo %d dòng", n)
	}
	// Mỗi lần 404 vẫn tiêu một lượt: lần thứ 11 gặp hạn mức ngày, kể cả với email học sinh thật.
	_, err := f.svc.CreateRequest(f.ctx, parent.ID, linkReq(f.user("child", "STUDENT").Email))
	wantLinkCode(t, err, "LINK_REQUEST_DAILY_LIMIT")
}

func TestParentLinkStudentNotFound_NonStudentAccountGetsIdentical404(t *testing.T) {
	f := newParentLinkFixture(t)
	parent := f.user("parent", "PARENT")
	teacher := f.user("teacher", "TEACHER")

	unknown := wantStudentNotFound(t, errOf(f.svc.CreateRequest(f.ctx, parent.ID, linkReq("khong-ton-tai-"+uuid.NewString()[:8]+"@40study.test"))))
	nonStudent := wantStudentNotFound(t, errOf(f.svc.CreateRequest(f.ctx, parent.ID, linkReq(teacher.Email))))
	if *unknown != *nonStudent {
		t.Fatalf("email giáo viên phải nhận đúng phản hồi của email lạ: %+v vs %+v", *nonStudent, *unknown)
	}
	if n := countLinkRows(t, f, parent.ID); n != 0 {
		t.Fatalf("tài khoản không phải học sinh mà vẫn tạo %d dòng", n)
	}
}

func TestParentLinkStudentNotFound_KnownStudentStillGets201WithStudentSetAtCreation(t *testing.T) {
	f := newParentLinkFixture(t)
	parent := f.user("parent", "PARENT")
	child := f.user("child", "STUDENT")

	out, err := f.svc.CreateRequest(f.ctx, parent.ID, linkReq(child.Email))
	if err != nil {
		t.Fatalf("học sinh thật phải gửi được: %v", err)
	}
	if out.Status != model.ParentLinkRequestStatusPending {
		t.Fatalf("status = %q", out.Status)
	}
	var row model.ParentLinkRequest
	if err := f.db.First(&row, "id = ?", out.ID).Error; err != nil {
		t.Fatalf("đọc dòng: %v", err)
	}
	if row.StudentUserID == nil || *row.StudentUserID != child.ID {
		t.Fatalf("student_user_id phải được gán ngay khi tạo: %v", row.StudentUserID)
	}
}

func TestParentLinkStudentNotFound_GhostEmailWithStalePendingRowGets404Not409(t *testing.T) {
	f := newParentLinkFixture(t)
	parent := f.user("parent", "PARENT")
	ghost := "qa-r2e-stale-" + uuid.NewString()[:8] + "@40study.test"
	stale := model.ParentLinkRequest{ParentUserID: parent.ID, StudentEmail: ghost, Relationship: "parent", Status: model.ParentLinkRequestStatusPending}
	if err := f.db.Create(&stale).Error; err != nil {
		t.Fatalf("tạo dòng rác: %v", err)
	}

	_, err := f.svc.CreateRequest(f.ctx, parent.ID, linkReq(ghost))
	wantStudentNotFound(t, err)
}
