package database

// Test Postgres THẬT cho runParentLinkGhostCleanup (parent_link_ghost_cleanup.go). Bỏ điều kiện
// student_user_id IS NULL, bỏ lọc status='pending', bỏ kiểm tra vai STUDENT, hoặc đổi UPDATE
// thành DELETE thì test ĐỎ.

import (
	"bytes"
	"log"
	"os"
	"strings"
	"testing"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"study.com/v1/internal/model"
	"study.com/v1/internal/testutil/pgtest"
)

func ghostSystemRole(t *testing.T, db *gorm.DB, name string) uuid.UUID {
	t.Helper()
	r := model.SystemRole{Name: name}
	if err := db.Where("name = ?", name).FirstOrCreate(&r).Error; err != nil {
		t.Fatalf("tạo system role %s: %v", name, err)
	}
	return r.ID
}

func ghostUser(t *testing.T, db *gorm.DB, kind, roleName string) model.User {
	t.Helper()
	s := uuid.NewString()
	u := model.User{Email: "QA-ghost-" + kind + "-" + s + "@40study.test", PasswordHash: "x", UserName: "qa-ghost-" + kind + s[:8]}
	if err := db.Create(&u).Error; err != nil {
		t.Fatalf("tạo user: %v", err)
	}
	if roleName != "" {
		usr := model.UserSystemRole{UserID: u.ID, SystemRoleID: ghostSystemRole(t, db, roleName), Status: "active"}
		if err := db.Create(&usr).Error; err != nil {
			t.Fatalf("gán vai %s: %v", roleName, err)
		}
	}
	return u
}

func ghostRequest(t *testing.T, db *gorm.DB, parent uuid.UUID, email, status string, student *uuid.UUID) uuid.UUID {
	t.Helper()
	r := model.ParentLinkRequest{ParentUserID: parent, StudentUserID: student, StudentEmail: strings.ToLower(email),
		Relationship: "parent", Status: status}
	if err := db.Create(&r).Error; err != nil {
		t.Fatalf("tạo yêu cầu: %v", err)
	}
	return r.ID
}

func captureLog(t *testing.T) *bytes.Buffer {
	t.Helper()
	var buf bytes.Buffer
	log.SetOutput(&buf)
	t.Cleanup(func() { log.SetOutput(os.Stderr) })
	return &buf
}

func statusOf(t *testing.T, db *gorm.DB, id uuid.UUID) string {
	t.Helper()
	var r model.ParentLinkRequest
	if err := db.First(&r, "id = ?", id).Error; err != nil {
		t.Fatalf("đọc yêu cầu %s: %v", id, err)
	}
	return r.Status
}

func TestParentLinkGhostCleanup_SoftCancelsOnlyGhostPendingRows_IdempotentNeverDeletes(t *testing.T) {
	db := pgtest.IsolatedSchema(t, Migrate)
	parent := ghostUser(t, db, "parent", "PARENT")
	student := ghostUser(t, db, "student", "STUDENT")
	teacher := ghostUser(t, db, "teacher", "TEACHER")
	norole := ghostUser(t, db, "norole", "")

	ghostNoAccount := ghostRequest(t, db, parent.ID, "khong-ton-tai-"+uuid.NewString()[:8]+"@40study.test", "pending", nil)
	ghostTeacher := ghostRequest(t, db, parent.ID, teacher.Email, "pending", nil)
	ghostNoRole := ghostRequest(t, db, parent.ID, norole.Email, "pending", nil)
	// Email nay da khop mot hoc sinh that (dang ky sau, khac hoa thuong): GIU, con co the tra loi.
	keepMatching := ghostRequest(t, db, parent.ID, strings.ToUpper(student.Email), "pending", nil)
	// Dong pending da gan hoc sinh: khong phai rac.
	keepLinked := ghostRequest(t, db, parent.ID, "linked-"+uuid.NewString()[:8]+"@40study.test", "pending", &student.ID)
	// Dong da xu ly (khong pending), du email khong ai so huu: khong dung.
	keepRejected := ghostRequest(t, db, parent.ID, "rejected-"+uuid.NewString()[:8]+"@40study.test", "rejected", nil)
	keepAccepted := ghostRequest(t, db, parent.ID, "accepted-"+uuid.NewString()[:8]+"@40study.test", "accepted", nil)

	var total int64
	db.Model(&model.ParentLinkRequest{}).Count(&total)
	buf := captureLog(t)

	for run := 1; run <= 2; run++ {
		if err := runParentLinkGhostCleanup(db); err != nil {
			t.Fatalf("lần %d: %v", run, err)
		}
		for name, c := range map[string]struct {
			id   uuid.UUID
			want string
		}{
			"không có tài khoản":  {ghostNoAccount, "cancelled"},
			"tài khoản giáo viên": {ghostTeacher, "cancelled"},
			"tài khoản không vai": {ghostNoRole, "cancelled"},
			"khớp học sinh thật":  {keepMatching, "pending"},
			"đã gắn học sinh":     {keepLinked, "pending"},
			"đã bị từ chối":       {keepRejected, "rejected"},
			"đã chấp nhận":        {keepAccepted, "accepted"},
		} {
			if got := statusOf(t, db, c.id); got != c.want {
				t.Errorf("lần %d, %s: status = %q, muốn %q", run, name, got, c.want)
			}
		}
	}

	var after int64
	db.Model(&model.ParentLinkRequest{}).Count(&after)
	if after != total {
		t.Fatalf("dọn rác không được xoá dòng nào: %d -> %d", total, after)
	}

	out := buf.String()
	for _, id := range []uuid.UUID{ghostNoAccount, ghostTeacher, ghostNoRole} {
		if !strings.Contains(out, id.String()) {
			t.Errorf("log phải ghi id %s đã huỷ mềm; log = %q", id, out)
		}
	}
	for _, id := range []uuid.UUID{keepMatching, keepLinked, keepRejected, keepAccepted} {
		if strings.Contains(out, id.String()) {
			t.Errorf("log ghi nhầm id %s không bị huỷ", id)
		}
	}
	if !strings.Contains(out, "cancelled 3 ") {
		t.Errorf("log phải ghi số dòng đã huỷ (3); log = %q", out)
	}
	if strings.Count(out, "cancelled ") != 1 {
		t.Errorf("lần chạy thứ 2 không còn gì để huỷ nên không được ghi log; log = %q", out)
	}
}

func TestParentLinkGhostCleanup_LogsAtMost200IdsButCancelsAll(t *testing.T) {
	db := pgtest.IsolatedSchema(t, Migrate)
	parent := ghostUser(t, db, "parent", "PARENT")
	const n = parentLinkGhostLogCap + 5
	ids := make([]uuid.UUID, n)
	for i := range ids {
		ids[i] = ghostRequest(t, db, parent.ID, "khong-ton-tai-"+uuid.NewString()+"@40study.test", "pending", nil)
	}
	buf := captureLog(t)

	if err := runParentLinkGhostCleanup(db); err != nil {
		t.Fatal(err)
	}
	var cancelled int64
	db.Model(&model.ParentLinkRequest{}).Where("status = 'cancelled'").Count(&cancelled)
	if cancelled != n {
		t.Fatalf("huỷ %d dòng, muốn %d", cancelled, n)
	}
	logged := 0
	for _, id := range ids {
		if strings.Contains(buf.String(), id.String()) {
			logged++
		}
	}
	if logged != parentLinkGhostLogCap {
		t.Fatalf("log ghi %d id, muốn đúng %d", logged, parentLinkGhostLogCap)
	}
	if !strings.Contains(buf.String(), "cancelled 205 ") {
		t.Fatalf("log phải ghi tổng số 205; log = %q", buf.String())
	}
}
