package service

// Test Postgres THẬT cho phase 02 (plan 260930): mời/duyệt/vào nhóm dùng chung activateMember, mã lỗi nhóm,
// sức chứa, xin vào lại, my_join_request và danh sách thành viên theo trạng thái.
// Mỗi test đảo một lỗi đã xác nhận ở group_service.go trước phase này (xem phase-02-backend-group-support.md).

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"study.com/v1/internal/dto"
	"study.com/v1/internal/model"
	"study.com/v1/internal/testutil/pgtest"
)

type groupFx struct {
	t   *testing.T
	db  *gorm.DB
	svc *GroupService
}

func newGroupFx(t *testing.T) *groupFx {
	t.Helper()
	db := pgtest.IsolatedSchema(t, migrateLikeAPIBoot)
	return &groupFx{t: t, db: db, svc: newGroupServiceForInviteTest(db)}
}

// group tạo nhóm + hội thoại nhóm + owner (đủ như CreateGroup), với privacy và sức chứa cho trước.
func (fx *groupFx) group(owner uuid.UUID, privacy model.GroupPrivacy, maxMembers int) *model.Group {
	fx.t.Helper()
	g := model.Group{Name: "QA nhom", Slug: "qa-" + uuid.NewString(), CreatedBy: owner, Privacy: privacy, MaxMembers: maxMembers, MemberCount: 1}
	if err := fx.db.Create(&g).Error; err != nil {
		fx.t.Fatalf("tạo nhóm: %v", err)
	}
	m := model.GroupMember{GroupID: g.ID, UserID: owner, Role: model.GroupRoleOwner, Status: model.GroupMemberActive}
	if err := fx.db.Create(&m).Error; err != nil {
		fx.t.Fatalf("tạo owner: %v", err)
	}
	conv := model.Conversation{Type: model.ConversationTypeGroup, GroupID: &g.ID}
	if err := fx.db.Create(&conv).Error; err != nil {
		fx.t.Fatalf("tạo hội thoại: %v", err)
	}
	p := model.ConversationParticipant{ConversationID: conv.ID, UserID: owner}
	if err := fx.db.Create(&p).Error; err != nil {
		fx.t.Fatalf("tạo participant: %v", err)
	}
	return &g
}

// member thêm thẳng một dòng thành viên với role/status tuỳ ý (không đi qua service).
func (fx *groupFx) member(gid, uid uuid.UUID, role model.GroupMemberRole, status model.GroupMemberStatus) {
	fx.t.Helper()
	m := model.GroupMember{GroupID: gid, UserID: uid, Role: role, Status: status}
	if err := fx.db.Create(&m).Error; err != nil {
		fx.t.Fatalf("tạo thành viên: %v", err)
	}
}

func (fx *groupFx) count(gid uuid.UUID) int {
	fx.t.Helper()
	var g model.Group
	if err := fx.db.First(&g, "id = ?", gid).Error; err != nil {
		fx.t.Fatal(err)
	}
	return g.MemberCount
}

func (fx *groupFx) row(gid, uid uuid.UUID) *model.GroupMember {
	fx.t.Helper()
	var m model.GroupMember
	err := fx.db.Where("group_id = ? AND user_id = ?", gid, uid).First(&m).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil
	}
	if err != nil {
		fx.t.Fatal(err)
	}
	return &m
}

func (fx *groupFx) activeParticipant(gid, uid uuid.UUID) bool {
	fx.t.Helper()
	var n int64
	err := fx.db.Table("conversation_participants cp").
		Joins("JOIN conversations c ON c.id = cp.conversation_id").
		Where("c.group_id = ? AND cp.user_id = ? AND cp.left_at IS NULL", gid, uid).Count(&n).Error
	if err != nil {
		fx.t.Fatal(err)
	}
	return n > 0
}

func rejectedCode(res *dto.InviteMembersResult, uid uuid.UUID) string {
	for _, r := range res.Rejected {
		if r.UserID == uid {
			return r.Code
		}
	}
	return ""
}

// admin hệ thống làm người mời: đi qua guard quan hệ để test tập trung vào logic thành viên.
func (fx *groupFx) adminOwner() uuid.UUID {
	fx.t.Helper()
	a := guardUser(fx.t, fx.db, "gm-admin")
	guardMakeAdmin(fx.t, fx.db, a)
	return a
}

func TestInviteMembers_MoiLaiNguoiDaRoi_KichHoatLai_VaResetVaiTro(t *testing.T) {
	fx := newGroupFx(t)
	admin := fx.adminOwner()
	g := fx.group(admin, model.GroupPrivacyPrivate, 10)
	left := guardUser(t, fx.db, "left")
	fx.member(g.ID, left, model.GroupRoleAdmin, model.GroupMemberLeft) // từng là ADMIN rồi rời nhóm
	// Hội thoại: participant cũ đã rời.
	var conv model.Conversation
	fx.db.First(&conv, "group_id = ?", g.ID)
	leftAt := time.Now()
	fx.db.Create(&model.ConversationParticipant{ConversationID: conv.ID, UserID: left, LeftAt: &leftAt})

	res, err := fx.svc.InviteMembers(t.Context(), admin, g.ID, []uuid.UUID{left})
	if err != nil {
		t.Fatalf("InviteMembers: %v", err)
	}
	if len(res.Invited) != 1 || res.Invited[0] != left || len(res.Rejected) != 0 {
		t.Fatalf("muốn mời lại được người đã rời, nhận %+v", res)
	}
	row := fx.row(g.ID, left)
	if row.Status != model.GroupMemberActive {
		t.Errorf("muốn ACTIVE, nhận %s", row.Status)
	}
	if row.Role != model.GroupRoleMember {
		t.Errorf("vai trò phải reset về MEMBER (không thừa hưởng ADMIN cũ), nhận %s", row.Role)
	}
	if row.InvitedBy == nil || *row.InvitedBy != admin {
		t.Errorf("phải ghi người mời, nhận %v", row.InvitedBy)
	}
	if got := fx.count(g.ID); got != 2 {
		t.Errorf("member_count muốn 2, nhận %d", got)
	}
	if !fx.activeParticipant(g.ID, left) {
		t.Error("người được mời lại phải là participant còn hiệu lực của hội thoại nhóm")
	}
}

func TestInviteMembers_LyDoTuChoi_BiCam_DaLaThanhVien(t *testing.T) {
	fx := newGroupFx(t)
	admin := fx.adminOwner()
	g := fx.group(admin, model.GroupPrivacyPrivate, 10)
	banned, member, fresh := guardUser(t, fx.db, "banned"), guardUser(t, fx.db, "member"), guardUser(t, fx.db, "fresh")
	fx.member(g.ID, banned, model.GroupRoleMember, model.GroupMemberBanned)
	fx.member(g.ID, member, model.GroupRoleMember, model.GroupMemberActive)
	fx.db.Model(&model.Group{}).Where("id = ?", g.ID).Update("member_count", 2)

	res, err := fx.svc.InviteMembers(t.Context(), admin, g.ID, []uuid.UUID{banned, member, fresh})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Invited) != 1 || res.Invited[0] != fresh {
		t.Errorf("chỉ người mới được mời, nhận Invited=%v", res.Invited)
	}
	if c := rejectedCode(res, banned); c != dto.GroupMemberBannedCode {
		t.Errorf("người bị cấm: muốn %s, nhận %q", dto.GroupMemberBannedCode, c)
	}
	if c := rejectedCode(res, member); c != dto.GroupAlreadyMemberCode {
		t.Errorf("đã là thành viên: muốn %s, nhận %q", dto.GroupAlreadyMemberCode, c)
	}
	if fx.row(g.ID, banned).Status != model.GroupMemberBanned {
		t.Error("mời không được gỡ cấm")
	}
	if got := fx.count(g.ID); got != 3 {
		t.Errorf("member_count muốn 3 (2 + 1 người mới), nhận %d", got)
	}
}

func TestInviteMembers_NhomDay_TraGroupFull_KhongVuotSucChua(t *testing.T) {
	fx := newGroupFx(t)
	admin := fx.adminOwner()
	g := fx.group(admin, model.GroupPrivacyPrivate, 3) // owner + 2 chỗ
	users := []uuid.UUID{guardUser(t, fx.db, "a"), guardUser(t, fx.db, "b"), guardUser(t, fx.db, "c"), guardUser(t, fx.db, "d")}

	res, err := fx.svc.InviteMembers(t.Context(), admin, g.ID, users)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Invited) != 2 || len(res.Rejected) != 2 {
		t.Fatalf("muốn 2 được mời + 2 bị từ chối, nhận %+v", res)
	}
	for _, r := range res.Rejected {
		if r.Code != dto.GroupFullCode {
			t.Errorf("muốn GROUP_FULL, nhận %s", r.Code)
		}
	}
	if got := fx.count(g.ID); got != 3 {
		t.Errorf("member_count muốn đúng bằng sức chứa 3, nhận %d", got)
	}
	var active int64
	fx.db.Model(&model.GroupMember{}).Where("group_id = ? AND status = ?", g.ID, model.GroupMemberActive).Count(&active)
	if active != 3 {
		t.Errorf("số thành viên ACTIVE muốn 3, nhận %d", active)
	}
}

func TestInviteMembers_GuardChayTruoc_KhongLoTrangThaiThanhVien(t *testing.T) {
	fx := newGroupFx(t)
	owner := guardUser(t, fx.db, "owner-not-admin") // không phải admin: phải qua guard quan hệ
	g := fx.group(owner, model.GroupPrivacyPrivate, 10)
	strangerBanned := guardUser(t, fx.db, "stranger-banned")
	fx.member(g.ID, strangerBanned, model.GroupRoleMember, model.GroupMemberBanned)

	res, err := fx.svc.InviteMembers(t.Context(), owner, g.ID, []uuid.UUID{strangerBanned})
	if err != nil {
		t.Fatal(err)
	}
	if c := rejectedCode(res, strangerBanned); c != dto.GroupInviteNotAllowedCode {
		t.Errorf("người lạ phải nhận GROUP_INVITE_NOT_ALLOWED, không được lộ trạng thái bị cấm, nhận %q", c)
	}
}

func TestInviteMembers_IdLap_ChiXuLyMotLan(t *testing.T) {
	fx := newGroupFx(t)
	admin := fx.adminOwner()
	g := fx.group(admin, model.GroupPrivacyPrivate, 10)
	u := guardUser(t, fx.db, "dup")

	res, err := fx.svc.InviteMembers(t.Context(), admin, g.ID, []uuid.UUID{u, u, u})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Invited) != 1 || len(res.Rejected) != 0 {
		t.Errorf("id lặp: muốn Invited=1 Rejected=0, nhận %+v", res)
	}
	if got := fx.count(g.ID); got != 2 {
		t.Errorf("member_count muốn 2, nhận %d", got)
	}
}

// Lỗi hạ tầng (ở đây: khoá ngoại vì user không tồn tại) phải trả ra, không bị nuốt, và chỗ đã giữ được
// trả lại (member_count không lệch).
func TestInviteMembers_LoiHaTang_KhongNuot_VaTraLaiCho(t *testing.T) {
	fx := newGroupFx(t)
	admin := fx.adminOwner()
	g := fx.group(admin, model.GroupPrivacyPrivate, 10)

	_, err := fx.svc.InviteMembers(t.Context(), admin, g.ID, []uuid.UUID{uuid.New()})
	if err == nil {
		t.Fatal("user không tồn tại: muốn lỗi trả ra, nhưng InviteMembers báo thành công")
	}
	if got := fx.count(g.ID); got != 1 {
		t.Errorf("ghi member lỗi phải trả lại chỗ đã giữ: member_count muốn 1, nhận %d", got)
	}
}

func TestJoinGroup_LoiCoSentinel(t *testing.T) {
	fx := newGroupFx(t)
	ctx := t.Context()
	owner := guardUser(t, fx.db, "owner")
	pub := fx.group(owner, model.GroupPrivacyPublic, 3)
	prv := fx.group(owner, model.GroupPrivacyPrivate, 2)

	_, err := fx.svc.JoinGroup(ctx, owner, pub.ID, nil)
	wantErr(t, err, ErrGroupAlreadyMember, "chủ nhóm join lại")

	banned := guardUser(t, fx.db, "banned")
	fx.member(pub.ID, banned, model.GroupRoleMember, model.GroupMemberBanned)
	_, err = fx.svc.JoinGroup(ctx, banned, pub.ID, nil)
	wantErr(t, err, ErrGroupBanned, "người bị cấm join")

	// Nhóm PRIVATE đã đầy (owner + 1 thành viên, max 2): không cho xin vào.
	fx.member(prv.ID, guardUser(t, fx.db, "m"), model.GroupRoleMember, model.GroupMemberActive)
	fx.db.Model(&model.Group{}).Where("id = ?", prv.ID).Update("member_count", 2)
	_, err = fx.svc.JoinGroup(ctx, guardUser(t, fx.db, "late"), prv.ID, nil)
	wantErr(t, err, ErrGroupFull, "xin vào nhóm private đã đầy")

	// Nhóm PUBLIC đầy.
	fx.member(pub.ID, guardUser(t, fx.db, "p1"), model.GroupRoleMember, model.GroupMemberActive)
	fx.member(pub.ID, guardUser(t, fx.db, "p2"), model.GroupRoleMember, model.GroupMemberActive)
	fx.db.Model(&model.Group{}).Where("id = ?", pub.ID).Update("member_count", 3)
	_, err = fx.svc.JoinGroup(ctx, guardUser(t, fx.db, "late2"), pub.ID, nil)
	wantErr(t, err, ErrGroupFull, "vào nhóm public đã đầy")

	// Yêu cầu đang chờ.
	open := fx.group(owner, model.GroupPrivacyPrivate, 10)
	u := guardUser(t, fx.db, "asker")
	if _, err := fx.svc.JoinGroup(ctx, u, open.ID, nil); err != nil {
		t.Fatal(err)
	}
	_, err = fx.svc.JoinGroup(ctx, u, open.ID, nil)
	wantErr(t, err, ErrGroupJoinRequestExists, "xin vào hai lần")
}

// Nhiều người vào nhóm PUBLIC cùng lúc khi chỉ còn 1 chỗ: đúng 1 người vào, member_count không vượt max.
// Thay TryReserveMemberSlot bằng kiểm-rồi-ghi hai bước thì test này đỏ (vài người cùng thấy còn chỗ).
func TestJoinGroup_DongThoi_KhongVuotMaxMembers(t *testing.T) {
	fx := newGroupFx(t)
	owner := guardUser(t, fx.db, "owner")
	g := fx.group(owner, model.GroupPrivacyPublic, 2) // còn đúng 1 chỗ
	const racers = 8
	users := make([]uuid.UUID, racers)
	for i := range users {
		users[i] = guardUser(t, fx.db, "racer")
	}
	start := make(chan struct{})
	var wg sync.WaitGroup
	errs := make([]error, racers)
	for i := range users {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			_, errs[i] = fx.svc.JoinGroup(t.Context(), users[i], g.ID, nil)
		}()
	}
	close(start)
	wg.Wait()

	joined, full := 0, 0
	for _, err := range errs {
		switch {
		case err == nil:
			joined++
		case errors.Is(err, ErrGroupFull):
			full++
		default:
			t.Errorf("lỗi ngoài dự kiến: %v", err)
		}
	}
	if joined != 1 || full != racers-1 {
		t.Errorf("muốn 1 người vào và %d người GROUP_FULL, nhận joined=%d full=%d", racers-1, joined, full)
	}
	if got := fx.count(g.ID); got != 2 {
		t.Errorf("member_count muốn 2 (= max), nhận %d", got)
	}
	var active int64
	fx.db.Model(&model.GroupMember{}).Where("group_id = ? AND status = ?", g.ID, model.GroupMemberActive).Count(&active)
	if active != 2 {
		t.Errorf("số thành viên ACTIVE muốn 2, nhận %d", active)
	}
}

func TestApproveRequest_NguoiDaTungRoi_DuyetDuoc_XinLaiSauKhiBiTuChoi(t *testing.T) {
	fx := newGroupFx(t)
	ctx := t.Context()
	owner := guardUser(t, fx.db, "owner")
	g := fx.group(owner, model.GroupPrivacyPrivate, 10)
	u := guardUser(t, fx.db, "asker")

	first := fx.request(ctx, u, g.ID, "lần 1")
	if err := fx.svc.ApproveRequest(ctx, owner, g.ID, first); err != nil {
		t.Fatalf("duyệt lần 1: %v", err)
	}
	if err := fx.svc.LeaveGroup(ctx, u, g.ID); err != nil {
		t.Fatal(err)
	}

	// Xin vào lại sau khi đã duyệt + rời: trước đây vi phạm unique group_join_requests và người dùng kẹt.
	second := fx.request(ctx, u, g.ID, "lần 2")
	if second != first {
		t.Errorf("xin lại phải dùng lại dòng cũ (unique theo cặp), id %v != %v", second, first)
	}
	if err := fx.svc.ApproveRequest(ctx, owner, g.ID, second); err != nil {
		t.Fatalf("duyệt người đã từng rời (trước đây lỗi unique group_members): %v", err)
	}
	if row := fx.row(g.ID, u); row.Status != model.GroupMemberActive || row.Role != model.GroupRoleMember {
		t.Errorf("muốn ACTIVE/MEMBER, nhận %+v", row)
	}
	if got := fx.count(g.ID); got != 2 {
		t.Errorf("member_count muốn 2, nhận %d", got)
	}
	if !fx.activeParticipant(g.ID, u) {
		t.Error("phải là participant hội thoại nhóm")
	}

	// Bị từ chối rồi xin lại.
	v := guardUser(t, fx.db, "rejected")
	r1 := fx.request(ctx, v, g.ID, "x")
	reason := "chưa đủ điều kiện"
	if err := fx.svc.RejectRequest(ctx, owner, g.ID, r1, &reason); err != nil {
		t.Fatal(err)
	}
	r2 := fx.request(ctx, v, g.ID, "xin lại")
	list, err := fx.svc.ListJoinRequests(ctx, owner, g.ID, 1, 20)
	if err != nil {
		t.Fatal(err)
	}
	var found *dto.JoinRequestResponse
	for i := range list.Requests {
		if list.Requests[i].UserID == v {
			found = &list.Requests[i]
		}
	}
	if r2 != r1 || found == nil || found.Status != "PENDING" || found.RejectionReason != nil || found.ReviewedBy != nil ||
		found.Message == nil || *found.Message != "xin lại" {
		t.Errorf("xin lại sau khi bị từ chối: muốn PENDING sạch dấu vết duyệt + message mới, nhận %+v (id %v vs %v)", found, r2, r1)
	}
}

// request gửi yêu cầu xin vào (nhóm không công khai) và trả id yêu cầu.
func (fx *groupFx) request(ctx context.Context, u, gid uuid.UUID, msg string) uuid.UUID {
	fx.t.Helper()
	res, err := fx.svc.JoinGroup(ctx, u, gid, &msg)
	if err != nil {
		fx.t.Fatalf("JoinGroup (xin vào): %v", err)
	}
	m, ok := res.(map[string]string)
	if !ok || m["status"] != "pending" {
		fx.t.Fatalf("muốn status pending, nhận %#v", res)
	}
	return uuid.MustParse(m["request_id"])
}

func TestApproveRequest_NhomDay_BanHoacDaLaThanhVien(t *testing.T) {
	fx := newGroupFx(t)
	ctx := t.Context()
	owner := guardUser(t, fx.db, "owner")
	g := fx.group(owner, model.GroupPrivacyPrivate, 3)
	a, b, c := guardUser(t, fx.db, "a"), guardUser(t, fx.db, "b"), guardUser(t, fx.db, "c")
	ra, rb, rc := fx.request(ctx, a, g.ID, "a"), fx.request(ctx, b, g.ID, "b"), fx.request(ctx, c, g.ID, "c")

	if err := fx.svc.ApproveRequest(ctx, owner, g.ID, ra); err != nil {
		t.Fatal(err)
	}
	if err := fx.svc.ApproveRequest(ctx, owner, g.ID, rb); err != nil {
		t.Fatal(err)
	}
	// Nhóm đã đủ 3 người: duyệt người thứ 4 phải báo đầy và GIỮ yêu cầu ở trạng thái chờ.
	err := fx.svc.ApproveRequest(ctx, owner, g.ID, rc)
	wantErr(t, err, ErrGroupFull, "duyệt khi nhóm đầy")
	var req model.GroupJoinRequest
	fx.db.First(&req, "id = ?", rc)
	if req.Status != model.JoinRequestPending {
		t.Errorf("duyệt thất bại thì yêu cầu phải còn PENDING, nhận %s", req.Status)
	}
	if fx.row(g.ID, c) != nil {
		t.Error("người bị từ chối vì nhóm đầy không được có dòng thành viên")
	}
	if got := fx.count(g.ID); got != 3 {
		t.Errorf("member_count muốn 3, nhận %d", got)
	}

	// Người xin vào bị cấm sau khi gửi yêu cầu: duyệt không được lách lệnh cấm.
	g2 := fx.group(owner, model.GroupPrivacyPrivate, 10)
	d := guardUser(t, fx.db, "d")
	rd := fx.request(ctx, d, g2.ID, "d")
	fx.member(g2.ID, d, model.GroupRoleMember, model.GroupMemberBanned)
	wantErr(t, fx.svc.ApproveRequest(ctx, owner, g2.ID, rd), ErrGroupBanned, "duyệt người đang bị cấm")
	if fx.row(g2.ID, d).Status != model.GroupMemberBanned {
		t.Error("duyệt không được gỡ cấm")
	}

	// Đã là thành viên (duyệt lại sau lần lỗi trước): chốt yêu cầu, không lỗi, không tăng member_count.
	e := guardUser(t, fx.db, "e")
	re := fx.request(ctx, e, g2.ID, "e")
	fx.member(g2.ID, e, model.GroupRoleMember, model.GroupMemberActive)
	before := fx.count(g2.ID)
	if err := fx.svc.ApproveRequest(ctx, owner, g2.ID, re); err != nil {
		t.Errorf("duyệt người đã là thành viên phải thành công: %v", err)
	}
	var settled model.GroupJoinRequest // biến mới: First trên struct đã có khoá chính sẽ thêm điều kiện id cũ
	if err := fx.db.First(&settled, "id = ?", re).Error; err != nil {
		t.Fatal(err)
	}
	if settled.Status != model.JoinRequestApproved {
		t.Errorf("yêu cầu phải được chốt APPROVED, nhận %s", settled.Status)
	}
	if fx.count(g2.ID) != before {
		t.Error("duyệt người đã là thành viên không được tăng member_count")
	}
}

func TestGetGroupBySlug_MyJoinRequest(t *testing.T) {
	fx := newGroupFx(t)
	ctx := t.Context()
	owner := guardUser(t, fx.db, "owner")
	g := fx.group(owner, model.GroupPrivacyPrivate, 10)
	asker, rejected, plain := guardUser(t, fx.db, "asker"), guardUser(t, fx.db, "rej"), guardUser(t, fx.db, "plain")
	reqID := fx.request(ctx, asker, g.ID, "xin")
	rid := fx.request(ctx, rejected, g.ID, "xin")
	if err := fx.svc.RejectRequest(ctx, owner, g.ID, rid, nil); err != nil {
		t.Fatal(err)
	}

	got, err := fx.svc.GetGroupBySlug(ctx, g.Slug, &asker)
	if err != nil {
		t.Fatal(err)
	}
	if got.MyJoinRequest == nil || got.MyJoinRequest.ID != reqID || got.MyJoinRequest.Status != "PENDING" {
		t.Errorf("người đã gửi yêu cầu phải thấy my_join_request PENDING, nhận %+v", got.MyJoinRequest)
	}
	if got.MyRole != nil {
		t.Errorf("người xin chưa là thành viên, my_role phải rỗng, nhận %v", *got.MyRole)
	}

	for who, viewer := range map[string]*uuid.UUID{"khách": nil, "người chưa xin": &plain, "người bị từ chối": &rejected, "chủ nhóm": &owner} {
		got, err := fx.svc.GetGroupBySlug(ctx, g.Slug, viewer)
		if err != nil {
			t.Fatalf("%s: %v", who, err)
		}
		if got.MyJoinRequest != nil {
			t.Errorf("%s không được có my_join_request, nhận %+v", who, got.MyJoinRequest)
		}
	}

	// Duyệt xong thì thành thành viên và my_join_request biến mất.
	if err := fx.svc.ApproveRequest(ctx, owner, g.ID, reqID); err != nil {
		t.Fatal(err)
	}
	got, _ = fx.svc.GetGroupBySlug(ctx, g.Slug, &asker)
	if got.MyJoinRequest != nil || got.MyRole == nil {
		t.Errorf("sau khi duyệt: muốn my_role có, my_join_request rỗng; nhận %+v / %v", got.MyJoinRequest, got.MyRole)
	}

	// Nhóm SECRET vẫn 404 với người ngoài, kể cả người đã có yêu cầu (không lộ qua my_join_request).
	sec := fx.group(owner, model.GroupPrivacySecret, 10)
	fx.db.Create(&model.GroupJoinRequest{GroupID: sec.ID, UserID: plain, Status: model.JoinRequestPending})
	if _, err := fx.svc.GetGroupBySlug(ctx, sec.Slug, &plain); !errors.Is(err, ErrGroupNotFound) {
		t.Errorf("SECRET với người ngoài phải ErrGroupNotFound, nhận %v", err)
	}
}

func TestListMembers_TheoTrangThai_VaQuyenXem(t *testing.T) {
	fx := newGroupFx(t)
	ctx := t.Context()
	owner := guardUser(t, fx.db, "owner")
	admin, mod, member, banned, stranger := guardUser(t, fx.db, "admin"), guardUser(t, fx.db, "mod"),
		guardUser(t, fx.db, "member"), guardUser(t, fx.db, "banned"), guardUser(t, fx.db, "stranger")
	g := fx.group(owner, model.GroupPrivacyPublic, 10)
	fx.member(g.ID, admin, model.GroupRoleAdmin, model.GroupMemberActive)
	fx.member(g.ID, mod, model.GroupRoleModerator, model.GroupMemberActive)
	fx.member(g.ID, member, model.GroupRoleMember, model.GroupMemberActive)
	fx.member(g.ID, banned, model.GroupRoleMember, model.GroupMemberBanned)

	// Mặc định và ACTIVE: không có người bị cấm.
	for _, st := range []string{"", "ACTIVE", "active"} {
		res, err := fx.svc.ListMembers(ctx, member, g.ID, st, 1, 50)
		if err != nil || res.TotalCount != 4 {
			t.Fatalf("status=%q: muốn 4 thành viên ACTIVE, nhận %+v (%v)", st, res, err)
		}
		for _, m := range res.Members {
			if m.Status != "ACTIVE" || m.UserID == banned {
				t.Errorf("status=%q lộ người bị cấm hoặc trạng thái sai: %+v", st, m)
			}
		}
	}

	// BANNED: chỉ OWNER/ADMIN.
	for who, id := range map[string]uuid.UUID{"chủ nhóm": owner, "admin nhóm": admin} {
		res, err := fx.svc.ListMembers(ctx, id, g.ID, "BANNED", 1, 50)
		if err != nil || res.TotalCount != 1 || res.Members[0].UserID != banned || res.Members[0].Status != "BANNED" {
			t.Errorf("%s xem danh sách cấm: %+v (%v)", who, res, err)
		}
	}
	for who, id := range map[string]uuid.UUID{"điều hành viên": mod, "thành viên": member, "người ngoài": stranger, "chính người bị cấm": banned} {
		_, err := fx.svc.ListMembers(ctx, id, g.ID, "BANNED", 1, 50)
		wantErr(t, err, ErrGroupForbidden, who+" xem danh sách cấm")
	}
	_, err := fx.svc.ListMembers(ctx, owner, g.ID, "LEFT", 1, 50)
	wantErr(t, err, ErrGroupInvalidMemberStatus, "status ngoài ACTIVE|BANNED")

	// Q10: nhóm PRIVATE chỉ thành viên xem danh sách; PUBLIC người ngoài xem được; SECRET 404.
	prv := fx.group(owner, model.GroupPrivacyPrivate, 10)
	fx.member(prv.ID, member, model.GroupRoleMember, model.GroupMemberActive)
	if _, err := fx.svc.ListMembers(ctx, member, prv.ID, "", 1, 20); err != nil {
		t.Errorf("thành viên xem nhóm private: %v", err)
	}
	_, err = fx.svc.ListMembers(ctx, stranger, prv.ID, "", 1, 20)
	wantErr(t, err, ErrGroupForbidden, "người ngoài xem thành viên nhóm PRIVATE")
	_, err = fx.svc.ListMembers(ctx, banned, prv.ID, "", 1, 20)
	wantErr(t, err, ErrGroupForbidden, "người từng bị cấm xem thành viên nhóm PRIVATE")
	if _, err := fx.svc.ListMembers(ctx, stranger, g.ID, "", 1, 20); err != nil {
		t.Errorf("người ngoài xem thành viên nhóm PUBLIC: %v", err)
	}
	sec := fx.group(owner, model.GroupPrivacySecret, 10)
	_, err = fx.svc.ListMembers(ctx, stranger, sec.ID, "", 1, 20)
	wantErr(t, err, ErrGroupNotFound, "người ngoài xem nhóm SECRET")
}
