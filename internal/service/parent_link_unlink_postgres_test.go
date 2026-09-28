package service

import (
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"study.com/v1/internal/database"
	"study.com/v1/internal/model"
	"study.com/v1/internal/repository"
)

func (f *parentLinkFixture) link(parent, child model.User) {
	f.t.Helper()
	if err := f.svc.Respond(f.ctx, child.ID, uuid.MustParse(f.request(parent, child)), "accept"); err != nil {
		f.t.Fatalf("con xác nhận: %v", err)
	}
}

// oldInvitation — lời mời luồng cũ (con mời phụ huynh) còn chờ, tạo lúc `createdAt`.
func (f *parentLinkFixture) oldInvitation(parent, child model.User, createdAt time.Time) model.ParentInvitation {
	f.t.Helper()
	inv := model.ParentInvitation{StudentUserID: child.ID, InviteeEmail: parent.Email, InviteeUserID: &parent.ID,
		Relationship: "parent", Status: model.ParentInvitationStatusPending, TokenHash: "qa-r2e-" + uuid.NewString(),
		ExpiresAt: time.Now().Add(24 * time.Hour), CreatedAt: createdAt}
	if err := f.db.Create(&inv).Error; err != nil {
		f.t.Fatalf("tạo lời mời: %v", err)
	}
	return inv
}

func (f *parentLinkFixture) invitationService() *ParentInvitationService {
	return NewParentInvitationService(nil, nil, repository.NewParentInvitationRepository(f.db),
		repository.NewParentStudentRepository(f.db), repository.NewUserRepository(f.db), repository.NewUserSystemRoleRepository(f.db))
}

func (f *parentLinkFixture) invitationStatus(id uuid.UUID) string {
	f.t.Helper()
	var inv model.ParentInvitation
	if err := f.db.First(&inv, "id = ?", id).Error; err != nil {
		f.t.Fatalf("đọc lời mời: %v", err)
	}
	return inv.Status
}

// MAJOR-2: con huỷ liên kết thì mọi lời mời cũ và yêu cầu mới còn chờ của cặp bị huỷ theo; phụ
// huynh không dùng lời mời cũ để tự liên kết lại được.
func TestParentLink_Postgres_HuyLienKetDonSachLoiMoiVaYeuCauCuaCap(t *testing.T) {
	f := newParentLinkFixture(t)
	parent := f.user("parent", "PARENT")
	child := f.user("child", "STUDENT")
	other := f.user("other-parent", "PARENT")
	inv := f.oldInvitation(parent, child, time.Now().Add(-time.Hour))
	otherInv := f.oldInvitation(other, child, time.Now().Add(-time.Hour))
	f.link(parent, child)
	// Yêu cầu còn chờ của cặp (vd ghi từ đường khác) — phải bị huỷ kèm.
	stray := model.ParentLinkRequest{ParentUserID: parent.ID, StudentEmail: strings.ToLower(child.Email), Relationship: "parent", Status: "pending"}
	if err := f.db.Create(&stray).Error; err != nil {
		t.Fatalf("tạo yêu cầu chờ: %v", err)
	}

	if err := f.svc.UnlinkByStudent(f.ctx, child.ID, parent.ID); err != nil {
		t.Fatalf("con huỷ liên kết: %v", err)
	}
	if got := f.invitationStatus(inv.ID); got != model.ParentInvitationStatusRevoked {
		t.Fatalf("lời mời cũ của cặp phải bị thu hồi, đang là %q", got)
	}
	if got := f.invitationStatus(otherInv.ID); got != model.ParentInvitationStatusPending {
		t.Fatalf("lời mời của phụ huynh KHÁC không được đụng tới, đang là %q", got)
	}
	var s model.ParentLinkRequest
	f.db.First(&s, "id = ?", stray.ID)
	if s.Status != model.ParentLinkRequestStatusCancelled {
		t.Fatalf("yêu cầu chờ của cặp phải bị huỷ, đang là %q", s.Status)
	}
	if err := f.invitationService().RespondToInvitation(f.ctx, inv.ID, parent.ID, "accept"); err == nil {
		t.Fatal("phụ huynh chấp nhận được lời mời cũ sau khi con đã huỷ liên kết")
	}
	if f.canSeeChild(parent.ID, child.ID) {
		t.Fatal("bỏ qua sự đồng ý của con: phụ huynh xem lại được dữ liệu qua lời mời cũ")
	}
}

// MAJOR-2, lớp 2: kể cả khi lời mời cũ chưa bị thu hồi (dữ liệu cũ / đường huỷ khác), chấp nhận
// một lời mời tạo TRƯỚC lúc liên kết bị huỷ vẫn bị từ chối.
func TestParentInvitation_Postgres_LoiMoiTaoTruocKhiHuyLienKetKhongConHieuLuc(t *testing.T) {
	f := newParentLinkFixture(t)
	parent := f.user("parent", "PARENT")
	child := f.user("child", "STUDENT")
	f.link(parent, child)
	inv := f.oldInvitation(parent, child, time.Now().Add(-time.Hour))
	if err := f.db.Model(&model.ParentStudentRelation{}).Where("parent_user_id = ? AND student_user_id = ?", parent.ID, child.ID).
		Updates(map[string]interface{}{"status": model.ParentStudentStatusRevoked, "revoked_at": time.Now(), "revoked_by": "student"}).Error; err != nil {
		t.Fatalf("huỷ liên kết trực tiếp: %v", err)
	}
	if err := f.invitationService().RespondToInvitation(f.ctx, inv.ID, parent.ID, "accept"); err == nil {
		t.Fatal("lời mời tạo trước khi huỷ liên kết vẫn chấp nhận được")
	}
	if f.canSeeChild(parent.ID, child.ID) {
		t.Fatal("lời mời hết hiệu lực mà vẫn liên kết lại")
	}
}

// MINOR-5: con huỷ liên kết thì phụ huynh chờ ParentLinkCooldown mới gửi lại được; phụ huynh tự
// huỷ thì không phải chờ (đã có trong TestParentLink_Postgres_HuyLienKetVaLienKetLai).
func TestParentLink_Postgres_ConHuyLienKetThiPhuHuynhPhaiCho(t *testing.T) {
	f := newParentLinkFixture(t)
	parent := f.user("parent", "PARENT")
	child := f.user("child", "STUDENT")
	f.link(parent, child)
	if err := f.svc.UnlinkByStudent(f.ctx, child.ID, parent.ID); err != nil {
		t.Fatalf("con huỷ liên kết: %v", err)
	}
	_, err := f.svc.CreateRequest(f.ctx, parent.ID, linkReq(child.Email))
	wantLinkCode(t, err, "LINK_REQUEST_COOLDOWN")

	realNow := f.svc.now
	f.svc.now = func() time.Time { return realNow().Add(ParentLinkCooldown + time.Hour) }
	if _, err := f.svc.CreateRequest(f.ctx, parent.ID, linkReq(child.Email)); err != nil {
		t.Fatalf("hết thời gian chờ mà vẫn không gửi lại được: %v", err)
	}
}

// MINOR-7: hai tài khoản giữ cả 2 vai không thành phụ huynh của nhau — chặn cả lúc gửi lẫn lúc xác nhận.
func TestParentLink_Postgres_KhongLienKetVongNguoc(t *testing.T) {
	f := newParentLinkFixture(t)
	a := f.user("a", "PARENT", "STUDENT")
	b := f.user("b", "PARENT", "STUDENT")
	reverseReq := f.request(b, a) // b gửi cho a trước, còn chờ
	f.link(a, b)                  // a thành phụ huynh của b

	_, err := f.svc.CreateRequest(f.ctx, b.ID, linkReq(a.Email))
	wantLinkCode(t, err, "LINK_REQUEST_PENDING") // yêu cầu cũ còn chờ, trùng email
	wantLinkCode(t, f.svc.Respond(f.ctx, a.ID, uuid.MustParse(reverseReq), "accept"), "LINK_CIRCULAR")
	if rows := f.relationRows(b.ID, a.ID); len(rows) != 0 {
		t.Fatalf("đã tạo quan hệ vòng ngược: %+v", rows)
	}
	if err := f.svc.Cancel(f.ctx, b.ID, uuid.MustParse(reverseReq)); err != nil {
		t.Fatalf("rút yêu cầu: %v", err)
	}
	_, err = f.svc.CreateRequest(f.ctx, b.ID, linkReq(a.Email))
	wantLinkCode(t, err, "LINK_CIRCULAR")
}

// MINOR-9: người gửi bị gỡ vai PARENT trước khi con xác nhận thì không tạo được liên kết.
func TestParentLink_Postgres_NguoiGuiMatVaiPhuHuynhThiKhongXacNhanDuoc(t *testing.T) {
	f := newParentLinkFixture(t)
	parent := f.user("parent", "PARENT")
	child := f.user("child", "STUDENT")
	reqID := uuid.MustParse(f.request(parent, child))
	if err := f.db.Model(&model.UserSystemRole{}).Where("user_id = ?", parent.ID).Update("status", "inactive").Error; err != nil {
		t.Fatalf("gỡ vai: %v", err)
	}
	wantLinkCode(t, f.svc.Respond(f.ctx, child.ID, reqID, "accept"), "PARENT_ROLE_REVOKED")
	if rows := f.relationRows(parent.ID, child.ID); len(rows) != 0 {
		t.Fatalf("đã tạo quan hệ cho tài khoản không còn là phụ huynh: %+v", rows)
	}
}

// MINOR-8: DB cũ có status rác thì migration chuẩn hoá về 'revoked' rồi mới gắn CHECK, không làm
// API chết lúc khởi động.
func TestParentLink_Postgres_MigrationChuanHoaStatusRacTruocCheck(t *testing.T) {
	f := newParentLinkFixture(t)
	parent := f.user("parent", "PARENT")
	child := f.user("child", "STUDENT")
	if err := f.db.Exec("ALTER TABLE parent_student_relations DROP CONSTRAINT chk_parent_student_relations_status").Error; err != nil {
		t.Fatalf("bỏ CHECK: %v", err)
	}
	junk := model.ParentStudentRelation{ParentUserID: parent.ID, StudentUserID: child.ID, Status: "legacy_junk"}
	if err := f.db.Create(&junk).Error; err != nil {
		t.Fatalf("chèn dòng rác: %v", err)
	}
	if err := database.RunPostMigrations(f.db); err != nil {
		t.Fatalf("RunPostMigrations trên DB có status rác: %v", err)
	}
	if rows := f.relationRows(parent.ID, child.ID); len(rows) != 1 || rows[0].Status != model.ParentStudentStatusRevoked {
		t.Fatalf("dòng rác phải thành 'revoked': %+v", rows)
	}
	bad := model.ParentStudentRelation{ParentUserID: child.ID, StudentUserID: parent.ID, Status: "bogus"}
	if err := f.db.Create(&bad).Error; err == nil {
		t.Fatal("CHECK không được gắn lại sau khi chuẩn hoá")
	}
}
