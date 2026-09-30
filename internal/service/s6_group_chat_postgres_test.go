package service

// Lane S6 (Postgres thật), theo từng vai — chat nhóm và nhóm SECRET.
//  1. Rời / bị kick / bị cấm khỏi nhóm phải gỡ người đó khỏi hội thoại nhóm (left_at): trước đây họ vẫn
//     đọc (GET messages) và gửi (POST messages) được vì chỉ group_members.status đổi.
//  2. Vào lại nhóm phải khôi phục đúng dòng participant (chỉ mục duy nhất), không nhân đôi.
//  3. Backfill: người đã rời/bị cấm từ trước (participant còn left_at NULL) bị gỡ khi chạy post-migration.
//  4. GET /groups/:slug: nhóm SECRET 404 với khách và người ngoài; nhóm PRIVATE hiện tên nhưng KHÔNG kèm id
//     hội thoại; thành viên thấy id hội thoại. ?privacy=SECRET không liệt kê được nhóm bí mật.
// Bỏ MarkLeft ở một đường nào thì test tương ứng ĐỎ.

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"study.com/v1/internal/database"
	"study.com/v1/internal/dto"
	"study.com/v1/internal/model"
	"study.com/v1/internal/repository"
	"study.com/v1/internal/socket"
)

type s6GroupEnv struct {
	*s2Fixture
	groups *GroupService
	convs  *ConversationService
	hub    *socket.Hub
}

func newS6GroupEnv(t *testing.T) *s6GroupEnv {
	t.Helper()
	f := newS2Fixture(t)
	hub := socket.NewHub()
	notifier := socket.NewNotifier(hub)
	convRepo := repository.NewConversationRepository(f.db)
	partRepo := repository.NewConversationParticipantRepository(f.db)
	groups := NewGroupService(repository.NewGroupRepository(f.db), repository.NewGroupMemberRepository(f.db),
		repository.NewGroupJoinRequestRepository(f.db), convRepo, partRepo)
	groups.SetChannelEvictor(notifier)
	convs := NewConversationService(convRepo, partRepo, repository.NewMessageRepository(f.db), repository.NewMessageReactionRepository(f.db),
		notifier, repository.NewEnrollmentRepository(f.db), repository.NewParentStudentRepository(f.db), repository.NewUserSystemRoleRepository(f.db))
	return &s6GroupEnv{s2Fixture: f, groups: groups, convs: convs, hub: hub}
}

// newGroup tạo nhóm qua service thật (có hội thoại + participant chủ nhóm) rồi thêm các thành viên ACTIVE
// theo đúng đường của GroupService (addToGroupConversation), không ghi thẳng DB.
func (e *s6GroupEnv) newGroup(owner model.User, privacy string, members ...model.User) (*dto.GroupResponse, uuid.UUID) {
	e.t.Helper()
	ctx := context.Background()
	g, err := e.groups.CreateGroup(ctx, owner.ID, dto.CreateGroupRequest{Name: "S6 " + uuid.NewString()[:6], Privacy: privacy})
	if err != nil {
		e.t.Fatalf("tạo nhóm: %v", err)
	}
	for _, m := range members {
		e.addMember(g.ID, m)
	}
	return g, g.Conversation.ID
}

func (e *s6GroupEnv) addMember(groupID uuid.UUID, u model.User) {
	e.t.Helper()
	now := e.db.NowFunc()
	if err := e.db.Create(&model.GroupMember{GroupID: groupID, UserID: u.ID, Role: model.GroupRoleMember, Status: model.GroupMemberActive, JoinedAt: &now}).Error; err != nil {
		e.t.Fatal(err)
	}
	e.groups.addToGroupConversation(context.Background(), groupID, u.ID)
}

func (e *s6GroupEnv) canRead(u model.User, conv uuid.UUID) bool {
	_, err := e.convs.ListMessages(context.Background(), u.ID, conv, 1, 20)
	return err == nil
}

func (e *s6GroupEnv) canSend(u model.User, conv uuid.UUID) bool {
	text := "xin chao"
	_, err := e.convs.SendMessage(context.Background(), u.ID, conv, dto.SendMessageRequest{Content: &text})
	return err == nil
}

func TestS6_GroupChat_RoiKickBanThiKhongConDocGuiDuoc(t *testing.T) {
	e := newS6GroupEnv(t)
	ctx := context.Background()
	owner, leaver, kicked, banned, stayer := e.user("owner"), e.user("leaver"), e.user("kicked"), e.user("banned"), e.user("stayer")
	g, conv := e.newGroup(owner, "PRIVATE", leaver, kicked, banned, stayer)

	for _, u := range []model.User{owner, leaver, kicked, banned, stayer} {
		if !e.canRead(u, conv) || !e.canSend(u, conv) {
			t.Fatalf("thành viên %s phải đọc và gửi được trước khi bị gỡ", u.UserName)
		}
	}

	if err := e.groups.LeaveGroup(ctx, leaver.ID, g.ID); err != nil {
		t.Fatal(err)
	}
	if err := e.groups.RemoveMember(ctx, owner.ID, g.ID, kicked.ID); err != nil {
		t.Fatal(err)
	}
	if err := e.groups.BanMember(ctx, owner.ID, g.ID, banned.ID); err != nil {
		t.Fatal(err)
	}

	for name, u := range map[string]model.User{"người rời": leaver, "người bị kick": kicked, "người bị cấm": banned} {
		if e.canRead(u, conv) {
			t.Errorf("%s vẫn ĐỌC được tin nhóm", name)
		}
		if e.canSend(u, conv) {
			t.Errorf("%s vẫn GỬI được tin nhóm", name)
		}
		if got, err := e.convs.GetConversation(ctx, u.ID, conv); err == nil || got != nil {
			t.Errorf("%s vẫn xem được hội thoại", name)
		}
	}
	for _, u := range []model.User{owner, stayer} {
		if !e.canRead(u, conv) || !e.canSend(u, conv) {
			t.Errorf("thành viên còn lại %s bị chặn nhầm", u.UserName)
		}
	}
	// người bị gỡ không còn nhận tin chưa đọc/hội thoại trong danh sách của họ
	if list, err := e.convs.ListConversations(ctx, kicked.ID, 1, 20); err != nil || list.TotalCount != 0 {
		t.Errorf("hội thoại nhóm vẫn nằm trong danh sách của người bị kick: err=%v %+v", err, list)
	}
	// Tin nhắn người bị gỡ gửi khi còn là thành viên vẫn lưu, chỉ mất quyền truy cập.
	var n int64
	e.db.Model(&model.Message{}).Where("conversation_id = ?", conv).Count(&n)
	if n != 5+2 {
		t.Errorf("số tin trong hội thoại = %d, muốn 7 (5 tin đầu + owner + stayer)", n)
	}
}

func TestS6_GroupChat_VaoLaiNhomKhoiPhucParticipant(t *testing.T) {
	e := newS6GroupEnv(t)
	ctx := context.Background()
	owner, member := e.user("owner"), e.user("member")
	g, conv := e.newGroup(owner, "PUBLIC", member)

	if err := e.groups.LeaveGroup(ctx, member.ID, g.ID); err != nil {
		t.Fatal(err)
	}
	if e.canRead(member, conv) {
		t.Fatal("đã rời mà vẫn đọc được")
	}
	if _, err := e.groups.JoinGroup(ctx, member.ID, g.ID, nil); err != nil {
		t.Fatalf("vào lại nhóm PUBLIC: %v", err)
	}
	if !e.canRead(member, conv) || !e.canSend(member, conv) {
		t.Error("vào lại nhóm mà không đọc/gửi được")
	}
	var rows int64
	e.db.Model(&model.ConversationParticipant{}).Where("conversation_id = ? AND user_id = ?", conv, member.ID).Count(&rows)
	if rows != 1 {
		t.Errorf("có %d dòng participant, muốn đúng 1 (khôi phục dòng cũ)", rows)
	}
}

func TestS6_GroupChat_BackfillNguoiDaRoiTruocDay(t *testing.T) {
	e := newS6GroupEnv(t)
	owner, active, leftBefore, bannedBefore := e.user("owner"), e.user("active"), e.user("leftbefore"), e.user("bannedbefore")
	g, conv := e.newGroup(owner, "PRIVATE", active, leftBefore, bannedBefore)

	// Trạng thái CŨ: đổi group_members nhưng KHÔNG đụng conversation_participants (đúng lỗi trước S6).
	if err := e.db.Model(&model.GroupMember{}).Where("group_id = ? AND user_id = ?", g.ID, leftBefore.ID).Update("status", model.GroupMemberLeft).Error; err != nil {
		t.Fatal(err)
	}
	if err := e.db.Model(&model.GroupMember{}).Where("group_id = ? AND user_id = ?", g.ID, bannedBefore.ID).Update("status", model.GroupMemberBanned).Error; err != nil {
		t.Fatal(err)
	}
	if !e.canRead(leftBefore, conv) || !e.canRead(bannedBefore, conv) {
		t.Fatal("tiền điều kiện: dữ liệu cũ phải còn đọc được trước backfill")
	}

	if err := database.RunPostMigrations(e.db); err != nil {
		t.Fatal(err)
	}

	if e.canRead(leftBefore, conv) || e.canSend(leftBefore, conv) {
		t.Error("người đã rời từ trước vẫn truy cập sau backfill")
	}
	if e.canRead(bannedBefore, conv) || e.canSend(bannedBefore, conv) {
		t.Error("người bị cấm từ trước vẫn truy cập sau backfill")
	}
	if !e.canRead(owner, conv) || !e.canRead(active, conv) {
		t.Error("backfill gỡ nhầm thành viên còn hiệu lực")
	}
	// Chạy lại không đổi gì (idempotent).
	if err := database.RunPostMigrations(e.db); err != nil {
		t.Fatal(err)
	}
	if !e.canRead(owner, conv) || !e.canRead(active, conv) {
		t.Error("lần backfill thứ hai gỡ nhầm thành viên còn hiệu lực")
	}
}

func TestS6_Group_SecretVaPrivateTheoTungVai(t *testing.T) {
	e := newS6GroupEnv(t)
	ctx := context.Background()
	owner, member, stranger := e.user("owner"), e.user("member"), e.user("stranger")
	secret, secretConv := e.newGroup(owner, "SECRET", member)
	private, privateConv := e.newGroup(owner, "PRIVATE", member)
	public, _ := e.newGroup(owner, "PUBLIC", member)

	bySlug := func(g *dto.GroupResponse, viewer *uuid.UUID) (*dto.GroupResponse, error) {
		return e.groups.GetGroupBySlug(ctx, g.Slug, viewer)
	}

	// SECRET: khách và người ngoài 404; thành viên thấy, kèm id hội thoại.
	for name, viewer := range map[string]*uuid.UUID{"khách": nil, "người ngoài": &stranger.ID} {
		if got, err := bySlug(secret, viewer); !errors.Is(err, ErrGroupNotFound) || got != nil {
			t.Errorf("SECRET / %s: err=%v got=%v, muốn ErrGroupNotFound", name, err, got)
		}
	}
	for name, viewer := range map[string]uuid.UUID{"chủ nhóm": owner.ID, "thành viên": member.ID} {
		v := viewer
		got, err := bySlug(secret, &v)
		if err != nil || got == nil || got.Conversation == nil || got.Conversation.ID != secretConv {
			t.Errorf("SECRET / %s: err=%v got=%+v, muốn thấy nhóm kèm id hội thoại", name, err, got)
		}
	}

	// PRIVATE: mọi người thấy tên và mô tả để xin vào, nhưng chỉ thành viên có id hội thoại.
	for name, viewer := range map[string]*uuid.UUID{"khách": nil, "người ngoài": &stranger.ID} {
		got, err := bySlug(private, viewer)
		if err != nil || got == nil || got.Name != private.Name {
			t.Errorf("PRIVATE / %s: err=%v got=%+v, muốn thấy tên nhóm", name, err, got)
			continue
		}
		if got.Conversation != nil {
			t.Errorf("PRIVATE / %s: lộ id hội thoại %v", name, got.Conversation.ID)
		}
	}
	if got, err := bySlug(private, &member.ID); err != nil || got.Conversation == nil || got.Conversation.ID != privateConv {
		t.Errorf("PRIVATE / thành viên: err=%v got=%+v, muốn kèm id hội thoại", err, got)
	}
	// PUBLIC: khách thấy nhóm, không có id hội thoại.
	if got, err := bySlug(public, nil); err != nil || got.Conversation != nil {
		t.Errorf("PUBLIC / khách: err=%v got=%+v", err, got)
	}

	// Không liệt kê nhóm SECRET, kể cả khi xin đích danh ?privacy=SECRET.
	for _, privacy := range []string{"", "SECRET"} {
		list, err := e.groups.ListGroups(ctx, "", privacy, 1, 50)
		if err != nil {
			t.Fatal(err)
		}
		for _, g := range list.Groups {
			if g.ID == secret.ID {
				t.Errorf("ListGroups(privacy=%q) lộ nhóm SECRET", privacy)
			}
		}
	}

	// Xin vào nhóm SECRET bằng id: người ngoài 404, không tạo yêu cầu.
	if _, err := e.groups.JoinGroup(ctx, stranger.ID, secret.ID, nil); !errors.Is(err, ErrGroupNotFound) {
		t.Errorf("người ngoài xin vào SECRET: err=%v, muốn ErrGroupNotFound", err)
	}
	var reqs int64
	e.db.Model(&model.GroupJoinRequest{}).Where("group_id = ?", secret.ID).Count(&reqs)
	if reqs != 0 {
		t.Errorf("có %d yêu cầu vào nhóm SECRET từ người ngoài", reqs)
	}
	// PRIVATE vẫn cho xin vào.
	if res, err := e.groups.JoinGroup(ctx, stranger.ID, private.ID, nil); err != nil || res == nil {
		t.Errorf("xin vào nhóm PRIVATE: err=%v", err)
	}
}

// Kick/ban ngừng phát kênh WebSocket đang mở: hub không còn giữ kết nối của người bị gỡ trong kênh hội thoại.
func TestS6_GroupChat_KickNgungPhatKenhWebSocket(t *testing.T) {
	e := newS6GroupEnv(t)
	ctx := context.Background()
	owner, kicked := e.user("owner"), e.user("kicked")
	g, conv := e.newGroup(owner, "PRIVATE", kicked)

	channel := "conversation:" + conv.String()
	client := &socket.Client{ID: uuid.New(), UserID: kicked.ID}
	e.hub.SubscribeToChannel(client, channel)
	if members := e.hub.GetChannelMembers(channel); len(members) != 1 {
		t.Fatalf("tiền điều kiện: kênh có %d thành viên, muốn 1", len(members))
	}
	if err := e.groups.RemoveMember(ctx, owner.ID, g.ID, kicked.ID); err != nil {
		t.Fatal(err)
	}
	if members := e.hub.GetChannelMembers(channel); len(members) != 0 {
		t.Errorf("người bị kick vẫn nằm trong kênh WebSocket: %v", members)
	}
}
