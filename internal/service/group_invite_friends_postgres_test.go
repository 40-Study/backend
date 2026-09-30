package service

// Test Postgres THẬT (plan 260930, phase 05): mời vào nhóm dùng chung guard nhắn tin nên nhận bạn bè,
// và thông báo group_added cho người vừa được thêm.

import (
	"strings"
	"testing"

	"github.com/google/uuid"
	"study.com/v1/internal/dto"
	"study.com/v1/internal/model"
)

func newGroupFxWithFriends(t *testing.T) (*groupFx, *fakeFriendNotifier) {
	t.Helper()
	fx := newGroupFx(t)
	fx.svc.SetInviteGuard(newConversationServiceWithFriends(fx.db))
	notif := &fakeFriendNotifier{}
	fx.svc.SetNotifier(notif)
	return fx, notif
}

// Bạn ACCEPTED mời được; người lạ, lời mời PENDING và người đã chặn thì GROUP_INVITE_NOT_ALLOWED. Bỏ checker
// khỏi guard thì dòng "bạn bè" ĐỎ; nhận nhầm PENDING hoặc bỏ kiểm block thì hai dòng còn lại ĐỎ.
func TestInviteMembers_BanBe_MoiDuoc_NguoiLaPendingVaChanKhong(t *testing.T) {
	fx, _ := newGroupFxWithFriends(t)
	owner := guardUser(t, fx.db, "owner") // không phải admin: phải có quan hệ hợp lệ
	g := fx.group(owner, model.GroupPrivacyPrivate, 10)
	friend, stranger, pending, blocked := guardUser(t, fx.db, "friend"), guardUser(t, fx.db, "stranger"),
		guardUser(t, fx.db, "pending"), guardUser(t, fx.db, "blocked")
	guardFriendship(t, fx.db, owner, friend, model.FriendshipStatusAccepted)
	guardFriendship(t, fx.db, pending, owner, model.FriendshipStatusPending)
	guardFriendship(t, fx.db, blocked, owner, model.FriendshipStatusAccepted)
	guardBlock(t, fx.db, blocked, owner)

	res, err := fx.svc.InviteMembers(t.Context(), owner, g.ID, []uuid.UUID{friend, stranger, pending, blocked})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Invited) != 1 || res.Invited[0] != friend {
		t.Fatalf("chỉ bạn ACCEPTED được mời, nhận Invited=%v", res.Invited)
	}
	for name, id := range map[string]uuid.UUID{"người lạ": stranger, "lời mời PENDING": pending, "bạn bị chặn": blocked} {
		if c := rejectedCode(res, id); c != dto.GroupInviteNotAllowedCode {
			t.Errorf("%s: muốn GROUP_INVITE_NOT_ALLOWED, nhận %q", name, c)
		}
		if fx.row(g.ID, id) != nil {
			t.Errorf("%s không được có dòng thành viên", name)
		}
	}
	if !fx.activeParticipant(g.ID, friend) {
		t.Error("bạn được mời phải vào hội thoại nhóm")
	}
}

func TestInviteMembers_ThongBaoGroupAdded(t *testing.T) {
	fx, notif := newGroupFxWithFriends(t)
	admin := fx.adminOwner()
	g := fx.group(admin, model.GroupPrivacyPrivate, 10)
	added1, added2 := guardUser(t, fx.db, "added1"), guardUser(t, fx.db, "added2")
	already, banned := guardUser(t, fx.db, "already"), guardUser(t, fx.db, "banned")
	fx.member(g.ID, already, model.GroupRoleMember, model.GroupMemberActive)
	fx.member(g.ID, banned, model.GroupRoleMember, model.GroupMemberBanned)
	fx.db.Model(&model.Group{}).Where("id = ?", g.ID).Update("member_count", 2)

	if _, err := fx.svc.InviteMembers(t.Context(), admin, g.ID, []uuid.UUID{added1, already, banned, added2}); err != nil {
		t.Fatal(err)
	}
	got := notif.all()
	if len(got) != 1 {
		t.Fatalf("muốn đúng 1 lần gửi (cả lô), nhận %d: %+v", len(got), got)
	}
	n := got[0]
	if n.NotificationType != model.NotificationTypeGroupAdded || n.ReferenceType == nil || *n.ReferenceType != "group" ||
		n.ReferenceID == nil || *n.ReferenceID != g.ID {
		t.Errorf("group_added sai loại/tham chiếu (phải là group + id nhóm): %+v", n)
	}
	if len(n.UserIDs) != 2 || !(contains(n.UserIDs, added1) && contains(n.UserIDs, added2)) {
		t.Errorf("chỉ người VỪA được thêm nhận thông báo (không gồm người đã là thành viên/bị cấm), nhận %v", n.UserIDs)
	}
	if !strings.Contains(n.Content, g.Name) {
		t.Errorf("nội dung phải nêu tên nhóm: %q", n.Content)
	}

	// Không ai được thêm thì không gửi gì.
	before := len(notif.all())
	if _, err := fx.svc.InviteMembers(t.Context(), admin, g.ID, []uuid.UUID{already, banned}); err != nil {
		t.Fatal(err)
	}
	if len(notif.all()) != before {
		t.Error("không ai được thêm: không được gửi thông báo")
	}
}

// Lỗi gửi thông báo chỉ log: người dùng vẫn vào nhóm. Chưa nối notifier cũng không lỗi.
func TestInviteMembers_ThongBaoLoi_KhongLamHongViecThem(t *testing.T) {
	fx, notif := newGroupFxWithFriends(t)
	notif.err = errTest
	admin := fx.adminOwner()
	g := fx.group(admin, model.GroupPrivacyPrivate, 10)
	u := guardUser(t, fx.db, "u")
	res, err := fx.svc.InviteMembers(t.Context(), admin, g.ID, []uuid.UUID{u})
	if err != nil || len(res.Invited) != 1 || fx.row(g.ID, u).Status != model.GroupMemberActive {
		t.Fatalf("lỗi thông báo làm hỏng việc thêm: err=%v res=%+v", err, res)
	}
	fx.svc.SetNotifier(nil)
	v := guardUser(t, fx.db, "v")
	if res, err := fx.svc.InviteMembers(t.Context(), admin, g.ID, []uuid.UUID{v}); err != nil || len(res.Invited) != 1 {
		t.Errorf("thiếu notifier không được làm hỏng: err=%v res=%+v", err, res)
	}
}

func contains(list []uuid.UUID, id uuid.UUID) bool {
	for _, v := range list {
		if v == id {
			return true
		}
	}
	return false
}
