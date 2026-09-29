package service

// Test Postgres THAT cho bug 4 (Lane S2): POST /groups/{id}/members/invite phai di qua guard
// nhan tin theo quan he cua Lane G (canCreateDirectConversation), khong duoc lach.
// Go bo lenh goi guard trong GroupService.InviteMembers thi test "nguoi la" DO.

import (
	"errors"
	"testing"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"study.com/v1/internal/dto"
	"study.com/v1/internal/model"
	"study.com/v1/internal/repository"
	"study.com/v1/internal/testutil/pgtest"
)

func newGroupServiceForInviteTest(db *gorm.DB) *GroupService {
	svc := NewGroupService(
		repository.NewGroupRepository(db),
		repository.NewGroupMemberRepository(db),
		repository.NewGroupJoinRequestRepository(db),
		repository.NewConversationRepository(db),
		repository.NewConversationParticipantRepository(db),
	)
	svc.SetInviteGuard(newConversationServiceForTest(db))
	return svc
}

// groupWithOwner tao nhom + thanh vien OWNER (nguoi moi hop le ve mat vai tro trong nhom).
func groupWithOwner(t *testing.T, db *gorm.DB, owner uuid.UUID) uuid.UUID {
	t.Helper()
	g := model.Group{Name: "QA-s2 nhom", Slug: "qa-s2-" + uuid.NewString(), CreatedBy: owner, MaxMembers: 100}
	if err := db.Create(&g).Error; err != nil {
		t.Fatalf("tao nhom: %v", err)
	}
	m := model.GroupMember{GroupID: g.ID, UserID: owner, Role: model.GroupRoleOwner, Status: model.GroupMemberActive}
	if err := db.Create(&m).Error; err != nil {
		t.Fatalf("tao owner: %v", err)
	}
	return g.ID
}

func isActiveMember(t *testing.T, db *gorm.DB, groupID, userID uuid.UUID) bool {
	t.Helper()
	var n int64
	if err := db.Model(&model.GroupMember{}).
		Where("group_id = ? AND user_id = ? AND status = ?", groupID, userID, model.GroupMemberActive).
		Count(&n).Error; err != nil {
		t.Fatalf("dem thanh vien: %v", err)
	}
	return n > 0
}

func TestInviteMembers_NguoiLa_BiTuChoi_KhongVaoNhom(t *testing.T) {
	db := pgtest.IsolatedSchema(t, migrateLikeAPIBoot)
	svc := newGroupServiceForInviteTest(db)
	ctx := t.Context()

	inviter, stranger := guardUser(t, db, "gi-inviter"), guardUser(t, db, "gi-stranger")
	gid := groupWithOwner(t, db, inviter)

	res, err := svc.InviteMembers(ctx, inviter, gid, []uuid.UUID{stranger})
	if err != nil {
		t.Fatalf("InviteMembers: %v", err)
	}
	if len(res.Invited) != 0 {
		t.Errorf("nguoi la khong duoc vao nhom, Invited=%v", res.Invited)
	}
	if len(res.Rejected) != 1 || res.Rejected[0].UserID != stranger || res.Rejected[0].Code != dto.GroupInviteNotAllowedCode {
		t.Errorf("muon Rejected=[stranger, GROUP_INVITE_NOT_ALLOWED], duoc %+v", res.Rejected)
	}
	if isActiveMember(t, db, gid, stranger) {
		t.Error("nguoi la van bi them vao group_members")
	}
}

func TestInviteMembers_QuanHeHopLeVaHonHop(t *testing.T) {
	db := pgtest.IsolatedSchema(t, migrateLikeAPIBoot)
	svc := newGroupServiceForInviteTest(db)
	ctx := t.Context()

	teacher, student, stranger := guardUser(t, db, "gi-teacher"), guardUser(t, db, "gi-student"), guardUser(t, db, "gi-stranger2")
	guardEnroll(t, db, student, teacher)
	gid := groupWithOwner(t, db, teacher)

	res, err := svc.InviteMembers(ctx, teacher, gid, []uuid.UUID{student, stranger})
	if err != nil {
		t.Fatalf("InviteMembers: %v", err)
	}
	if len(res.Invited) != 1 || res.Invited[0] != student {
		t.Errorf("hoc vien cua giang vien phai duoc moi, Invited=%v", res.Invited)
	}
	if len(res.Rejected) != 1 || res.Rejected[0].UserID != stranger {
		t.Errorf("nguoi la phai nam trong Rejected, duoc %+v", res.Rejected)
	}
	if !isActiveMember(t, db, gid, student) || isActiveMember(t, db, gid, stranger) {
		t.Error("membership sai: hoc vien phai vao, nguoi la khong")
	}
}

func TestInviteMembers_AdminMoiAiCung_DuocPhep(t *testing.T) {
	db := pgtest.IsolatedSchema(t, migrateLikeAPIBoot)
	svc := newGroupServiceForInviteTest(db)
	ctx := t.Context()

	admin, stranger := guardUser(t, db, "gi-admin"), guardUser(t, db, "gi-stranger3")
	guardMakeAdmin(t, db, admin)
	gid := groupWithOwner(t, db, admin)

	res, err := svc.InviteMembers(ctx, admin, gid, []uuid.UUID{stranger})
	if err != nil {
		t.Fatalf("InviteMembers: %v", err)
	}
	if len(res.Invited) != 1 || len(res.Rejected) != 0 {
		t.Errorf("admin moi ai cung duoc, Invited=%v Rejected=%+v", res.Invited, res.Rejected)
	}
}

func TestInviteMembers_ThieuGuard_TuChoiChuKhongMoCua(t *testing.T) {
	db := pgtest.IsolatedSchema(t, migrateLikeAPIBoot)
	svc := newGroupServiceForInviteTest(db)
	svc.SetInviteGuard(nil)
	inviter, other := guardUser(t, db, "gi-noguard"), guardUser(t, db, "gi-other")
	gid := groupWithOwner(t, db, inviter)

	_, err := svc.InviteMembers(t.Context(), inviter, gid, []uuid.UUID{other})
	if !errors.Is(err, ErrGroupInviteGuardMissing) {
		t.Fatalf("muon ErrGroupInviteGuardMissing, duoc %v", err)
	}
	if isActiveMember(t, db, gid, other) {
		t.Error("thieu guard ma van them thanh vien")
	}
}
