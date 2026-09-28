package service

import (
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"study.com/v1/internal/model"
)

// emailInvitation — lời mời luồng cũ gửi tới email chưa có tài khoản (InviteeUserID nil).
func (f *parentLinkFixture) emailInvitation(child model.User, email string, createdAt time.Time) model.ParentInvitation {
	f.t.Helper()
	inv := model.ParentInvitation{StudentUserID: child.ID, InviteeEmail: email, Relationship: "parent",
		Status: model.ParentInvitationStatusInvited, TokenHash: "qa-r2e-" + uuid.NewString(),
		ExpiresAt: time.Now().Add(6 * 24 * time.Hour), CreatedAt: createdAt}
	if err := f.db.Create(&inv).Error; err != nil {
		f.t.Fatalf("tạo lời mời theo email: %v", err)
	}
	return inv
}

func (f *parentLinkFixture) setEmail(u *model.User, email string) {
	f.t.Helper()
	if err := f.db.Model(&model.User{}).Where("id = ?", u.ID).Update("email", email).Error; err != nil {
		f.t.Fatalf("đổi email: %v", err)
	}
	u.Email = email
}

// Review #81 vòng 2, B-1: lời mời theo email chỉ người có ĐÚNG email đó VÀ đang là phụ huynh mới
// chấp nhận được; mọi trường hợp sai đều nhận cùng một lỗi, không lộ lý do.
func TestParentInvitation_Postgres_LoiMoiTheoEmailChiNguoiDuocMoiChapNhan(t *testing.T) {
	f := newParentLinkFixture(t)
	inv := f.invitationService()
	child := f.user("child", "STUDENT")
	email := "qa-r2e-invitee-" + uuid.NewString()[:8] + "@40study.test"
	row := f.emailInvitation(child, "  "+email+" ", time.Now())

	sameEmailNoParent := f.user("same-email-student", "STUDENT")
	f.setEmail(&sameEmailNoParent, strings.ToUpper(email))
	wrong := []model.User{
		f.user("stranger-student", "STUDENT"),
		f.user("stranger-teacher", "TEACHER"),
		f.user("stranger-parent", "PARENT"),
		sameEmailNoParent,
	}
	var firstErr string
	for _, u := range wrong {
		err := inv.RespondToInvitation(f.ctx, row.ID, u.ID, "accept")
		if err == nil {
			t.Fatalf("%s chấp nhận được lời mời gửi tới email khác / không có vai phụ huynh", u.UserName)
		}
		if firstErr == "" {
			firstErr = err.Error()
		} else if err.Error() != firstErr {
			t.Fatalf("lỗi khác nhau theo lý do (lộ thông tin): %q vs %q", err.Error(), firstErr)
		}
		if f.canSeeChild(u.ID, child.ID) {
			t.Fatalf("%s xem được dữ liệu con", u.UserName)
		}
	}
	if got := f.invitationStatus(row.ID); got != model.ParentInvitationStatusInvited {
		t.Fatalf("lời mời bị đổi trạng thái bởi người không được mời: %q", got)
	}

	// Đúng người: email khác hoa thường (biến thể khác tài khoản trên vì idx_users_email phân biệt
	// hoa thường), đang là phụ huynh.
	realParent := f.user("real-parent", "PARENT")
	f.setEmail(&realParent, strings.ToUpper(email[:1])+email[1:])
	if err := inv.RespondToInvitation(f.ctx, row.ID, realParent.ID, "accept"); err != nil {
		t.Fatalf("người được mời không chấp nhận được: %v", err)
	}
	if !f.canSeeChild(realParent.ID, child.ID) {
		t.Fatal("người được mời đã chấp nhận mà không xem được dữ liệu con")
	}
}

// Kịch bản tài khoản phụ của reviewer: con mời email thứ hai của phụ huynh P (chưa đăng ký), rồi
// liên kết với P và huỷ. P dùng tài khoản phụ P2 (email khác) chấp nhận lời mời thứ hai — phải bị chặn.
func TestParentInvitation_Postgres_HuyLienKetRoiDungTaiKhoanPhuBiChan(t *testing.T) {
	f := newParentLinkFixture(t)
	inv := f.invitationService()
	parent := f.user("p", "PARENT")
	child := f.user("c", "STUDENT")
	second := f.emailInvitation(child, "p-second-"+uuid.NewString()[:8]+"@40study.test", time.Now().Add(-time.Hour))
	f.link(parent, child)
	if err := f.svc.UnlinkByStudent(f.ctx, child.ID, parent.ID); err != nil {
		t.Fatalf("con huỷ liên kết: %v", err)
	}
	p2 := f.user("p2", "PARENT")
	if err := inv.RespondToInvitation(f.ctx, second.ID, p2.ID, "accept"); err == nil {
		t.Fatal("tài khoản phụ chấp nhận được lời mời gửi tới email khác")
	}
	if f.canSeeChild(p2.ID, child.ID) {
		t.Fatal("bỏ qua sự đồng ý của con: tài khoản phụ xem được dữ liệu con")
	}
}

// Review #81 vòng 2, B-2: luồng lời mời cũ và luồng yêu cầu mới cùng xác nhận một cặp chưa có
// quan hệ. Hai luồng dùng chung khoá theo cặp nên không bên nào lỗi (trước đây bên thua nhận
// 23505 -> 500) và luôn đúng 1 dòng quan hệ.
func TestParentLink_Postgres_HaiLuongCungXacNhanKhongLoi(t *testing.T) {
	f := newParentLinkFixture(t)
	inv := f.invitationService()
	for i := 0; i < 15; i++ {
		parent := f.user("p", "PARENT")
		child := f.user("c", "STUDENT")
		invRow := f.oldInvitation(parent, child, time.Now().Add(-time.Minute))
		reqID := uuid.MustParse(f.request(parent, child))
		var wg sync.WaitGroup
		var errOld, errNew error
		start := make(chan struct{})
		wg.Add(2)
		go func() {
			defer wg.Done()
			<-start
			errOld = inv.RespondToInvitation(f.ctx, invRow.ID, parent.ID, "accept")
		}()
		go func() { defer wg.Done(); <-start; errNew = f.svc.Respond(f.ctx, child.ID, reqID, "accept") }()
		close(start)
		wg.Wait()
		if errOld != nil || errNew != nil {
			t.Fatalf("vòng %d: luồng cũ err=%v, luồng mới err=%v", i, errOld, errNew)
		}
		if rows := f.relationRows(parent.ID, child.ID); len(rows) != 1 || rows[0].Status != model.ParentStudentStatusActive {
			t.Fatalf("vòng %d: muốn đúng 1 quan hệ active, nhận %+v", i, rows)
		}
	}
}
