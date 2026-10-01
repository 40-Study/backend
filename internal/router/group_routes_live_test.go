package router

// Test HTTP qua route THẬT cho phần nhóm của plan 260930 (phase 02): mã lỗi `code`, quy tắc 403/200 của lời
// mời, my_join_request, members?status=, và quyền xem theo từng vai (chủ nhóm, admin nhóm, điều hành viên,
// thành viên, người ngoài, người bị cấm, khách).

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"gorm.io/gorm"
	"study.com/v1/internal/config"
	"study.com/v1/internal/constants"
	"study.com/v1/internal/database"
	"study.com/v1/internal/handler"
	"study.com/v1/internal/model"
	"study.com/v1/internal/repository"
	"study.com/v1/internal/service"
	"study.com/v1/internal/socket"
	"study.com/v1/internal/testutil/pgtest"
	"study.com/v1/internal/utils"
)

type groupLiveEnv struct {
	t      *testing.T
	app    *fiber.App
	db     *gorm.DB
	tok    map[string]string
	ids    map[string]uuid.UUID
	roleID map[string]uuid.UUID
}

func newGroupLiveEnv(t *testing.T) *groupLiveEnv {
	t.Helper()
	db := pgtest.IsolatedSchema(t, database.Migrate)
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = rdb.Close() })
	cfg := &config.Config{JWTSecret: "group-live-secret", JWTAccessExpiration: 15 * time.Minute, JWTRefreshExpiration: 7 * 24 * time.Hour}
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	e := &groupLiveEnv{t: t, db: db, tok: map[string]string{}, ids: map[string]uuid.UUID{}, roleID: map[string]uuid.UUID{}}
	for _, name := range []string{"owner", "sysadmin", "gadmin", "mod", "member", "stranger", "banned", "friend", "asker"} {
		full := "Ho Ten " + name
		u := model.User{Email: name + "-" + uuid.NewString()[:6] + "@40study.test", PasswordHash: "x",
			UserName: name + uuid.NewString()[:6], FullName: &full, IsActive: true}
		must(db.Create(&u).Error)
		must(rdb.Set(context.Background(), constants.KeyUserVersion(u.ID.String()), 1, 0).Err())
		tok, _, err := utils.GenerateTokens(cfg, u.ID, uuid.New(), "STUDENT", nil, 1)
		must(err)
		e.tok[name], e.ids[name] = tok, u.ID
	}
	sys := model.SystemRole{Name: "SYSTEM_ADMIN", Status: "active"}
	must(db.Create(&sys).Error)
	must(db.Create(&model.UserSystemRole{UserID: e.ids["sysadmin"], SystemRoleID: sys.ID, Status: model.UserSystemRoleStatusActive}).Error)

	convSvc := service.NewConversationService(
		repository.NewConversationRepository(db), repository.NewConversationParticipantRepository(db),
		repository.NewMessageRepository(db), repository.NewMessageReactionRepository(db),
		socket.NewNotifier(socket.NewHub()),
		repository.NewEnrollmentRepository(db), repository.NewParentStudentRepository(db), repository.NewUserSystemRoleRepository(db))
	groupSvc := service.NewGroupService(
		repository.NewGroupRepository(db), repository.NewGroupMemberRepository(db), repository.NewGroupJoinRequestRepository(db),
		repository.NewConversationRepository(db), repository.NewConversationParticipantRepository(db))
	groupSvc.SetInviteGuard(convSvc)

	app := fiber.New()
	SetupGroupRoutes(app.Group("/api"), cfg, handler.NewGroupHandler(groupSvc), rdb)
	e.app = app
	return e
}

type groupResp struct {
	Status int
	Raw    string
	Body   struct {
		Message string          `json:"message"`
		Code    string          `json:"code"`
		Data    json.RawMessage `json:"data"`
	}
}

func (e *groupLiveEnv) do(who, method, path, body string) groupResp {
	e.t.Helper()
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	if who != "" {
		req.Header.Set("Authorization", "Bearer "+e.tok[who])
	}
	res, err := e.app.Test(req, -1)
	if err != nil {
		e.t.Fatal(err)
	}
	defer res.Body.Close()
	raw, _ := io.ReadAll(res.Body)
	out := groupResp{Status: res.StatusCode, Raw: string(raw)}
	_ = json.Unmarshal(raw, &out.Body)
	return out
}

func (e *groupLiveEnv) expect(r groupResp, status int, code, what string) {
	e.t.Helper()
	if r.Status != status || (code != "" && r.Body.Code != code) {
		e.t.Errorf("%s: muốn %d/%s, nhận %d/%s — %s", what, status, code, r.Status, r.Body.Code, r.Raw)
	}
}

// group tạo nhóm cùng hội thoại, chủ nhóm là `owner`.
func (e *groupLiveEnv) group(privacy model.GroupPrivacy, maxMembers int) *model.Group {
	e.t.Helper()
	g := model.Group{Name: "Nhom QA", Slug: "qa-" + uuid.NewString(), CreatedBy: e.ids["owner"], Privacy: privacy, MaxMembers: maxMembers, MemberCount: 1}
	if err := e.db.Create(&g).Error; err != nil {
		e.t.Fatal(err)
	}
	e.member(g.ID, "owner", model.GroupRoleOwner, model.GroupMemberActive)
	conv := model.Conversation{Type: model.ConversationTypeGroup, GroupID: &g.ID}
	if err := e.db.Create(&conv).Error; err != nil {
		e.t.Fatal(err)
	}
	if err := e.db.Create(&model.ConversationParticipant{ConversationID: conv.ID, UserID: e.ids["owner"], JoinedAt: time.Now()}).Error; err != nil {
		e.t.Fatal(err)
	}
	return &g
}

func (e *groupLiveEnv) member(gid uuid.UUID, who string, role model.GroupMemberRole, status model.GroupMemberStatus) {
	e.t.Helper()
	if err := e.db.Create(&model.GroupMember{GroupID: gid, UserID: e.ids[who], Role: role, Status: status}).Error; err != nil {
		e.t.Fatal(err)
	}
}

func (e *groupLiveEnv) invite(who string, gid uuid.UUID, targets ...string) groupResp {
	ids := make([]string, len(targets))
	for i, t := range targets {
		ids[i] = fmt.Sprintf("%q", e.ids[t])
	}
	return e.do(who, "POST", fmt.Sprintf("/api/groups/%s/members/invite", gid), fmt.Sprintf(`{"user_ids":[%s]}`, strings.Join(ids, ",")))
}

type inviteData struct {
	Invited  []string `json:"invited"`
	Rejected []struct {
		UserID string `json:"user_id"`
		Code   string `json:"code"`
	} `json:"rejected"`
}

func (e *groupLiveEnv) inviteData(r groupResp) inviteData {
	var d inviteData
	if err := json.Unmarshal(r.Body.Data, &d); err != nil {
		e.t.Fatalf("data không phải InviteMembersResult: %s", r.Raw)
	}
	return d
}

// Quy tắc HTTP của lời mời (contract §2): 403 CHỈ khi không ai được mời VÀ mọi lý do là
// GROUP_INVITE_NOT_ALLOWED; còn lại 200 kèm rejected[].
func TestGroupRoutes_Moi_QuyTacHttpVaMaLyDo(t *testing.T) {
	e := newGroupLiveEnv(t)

	// (a) Chủ nhóm thường mời người lạ: chỉ có NOT_ALLOWED -> 403 + code + data.
	g := e.group(model.GroupPrivacyPrivate, 10)
	r := e.invite("owner", g.ID, "stranger")
	e.expect(r, 403, "GROUP_INVITE_NOT_ALLOWED", "mời người lạ")
	if d := e.inviteData(r); len(d.Invited) != 0 || len(d.Rejected) != 1 || d.Rejected[0].Code != "GROUP_INVITE_NOT_ALLOWED" {
		t.Errorf("data 403 sai: %s", r.Raw)
	}

	// (b) Admin hệ thống mời người đã là thành viên và người bị cấm: lý do KHÁC -> 200, invited rỗng.
	g2 := e.group(model.GroupPrivacyPrivate, 10)
	e.member(g2.ID, "sysadmin", model.GroupRoleOwner, model.GroupMemberActive)
	e.member(g2.ID, "member", model.GroupRoleMember, model.GroupMemberActive)
	e.member(g2.ID, "banned", model.GroupRoleMember, model.GroupMemberBanned)
	r = e.invite("sysadmin", g2.ID, "member", "banned")
	e.expect(r, 200, "", "mời toàn người không thể thêm vì lý do khác")
	d := e.inviteData(r)
	codes := map[string]string{}
	for _, rej := range d.Rejected {
		codes[rej.UserID] = rej.Code
	}
	if len(d.Invited) != 0 || codes[e.ids["member"].String()] != "GROUP_ALREADY_MEMBER" || codes[e.ids["banned"].String()] != "GROUP_MEMBER_BANNED" {
		t.Errorf("rejected sai: %s", r.Raw)
	}

	// (c) Thêm một phần: một người mới + một đã là thành viên -> 200.
	r = e.invite("sysadmin", g2.ID, "friend", "member")
	e.expect(r, 200, "", "mời một phần")
	if d := e.inviteData(r); len(d.Invited) != 1 || d.Invited[0] != e.ids["friend"].String() || len(d.Rejected) != 1 {
		t.Errorf("mời một phần sai: %s", r.Raw)
	}

	// (d) Guard quan hệ chạy TRƯỚC kiểm tra thành viên: người mời không có quan hệ với một thành viên sẵn có
	// vẫn nhận NOT_ALLOWED (không lộ "người đó đã ở trong nhóm") -> toàn NOT_ALLOWED nên 403.
	g3 := e.group(model.GroupPrivacyPrivate, 10)
	e.member(g3.ID, "gadmin", model.GroupRoleAdmin, model.GroupMemberActive)
	r = e.invite("owner", g3.ID, "stranger", "gadmin")
	e.expect(r, 403, "GROUP_INVITE_NOT_ALLOWED", "guard chạy trước, không lộ trạng thái thành viên")
	if d := e.inviteData(r); len(d.Rejected) != 2 || d.Rejected[0].Code != "GROUP_INVITE_NOT_ALLOWED" || d.Rejected[1].Code != "GROUP_INVITE_NOT_ALLOWED" {
		t.Errorf("cả hai phải là NOT_ALLOWED: %s", r.Raw)
	}

	// (e) Nhóm đầy: GROUP_FULL -> 200.
	g4 := e.group(model.GroupPrivacyPrivate, 1)
	e.member(g4.ID, "sysadmin", model.GroupRoleOwner, model.GroupMemberActive)
	r = e.invite("sysadmin", g4.ID, "friend")
	e.expect(r, 200, "", "nhóm đầy")
	if d := e.inviteData(r); len(d.Rejected) != 1 || d.Rejected[0].Code != "GROUP_FULL" {
		t.Errorf("nhóm đầy: muốn GROUP_FULL, nhận %s", r.Raw)
	}

	// Vai trò mời: thành viên thường và người ngoài không mời được (không phải lỗi quan hệ).
	for _, who := range []string{"member", "stranger", "banned"} {
		rr := e.invite(who, g2.ID, "friend")
		if rr.Status == 200 || rr.Status == 403 && rr.Body.Code == "GROUP_INVITE_NOT_ALLOWED" {
			t.Errorf("%s không được mời: %d %s", who, rr.Status, rr.Raw)
		}
	}
	e.expect(e.invite("", g2.ID, "friend"), 401, "", "khách mời")
}

func TestGroupRoutes_JoinVaApprove_CoMaLoi(t *testing.T) {
	e := newGroupLiveEnv(t)
	pub := e.group(model.GroupPrivacyPublic, 2)
	path := func(g *model.Group) string { return fmt.Sprintf("/api/groups/%s/join", g.ID) }

	e.expect(e.do("owner", "POST", path(pub), ""), 400, "GROUP_ALREADY_MEMBER", "đã là thành viên")
	e.member(pub.ID, "banned", model.GroupRoleMember, model.GroupMemberBanned)
	e.expect(e.do("banned", "POST", path(pub), ""), 400, "GROUP_BANNED", "bị cấm")
	e.expect(e.do("member", "POST", path(pub), ""), 200, "", "join nhóm public")
	e.expect(e.do("friend", "POST", path(pub), ""), 400, "GROUP_FULL", "nhóm đầy")

	prv := e.group(model.GroupPrivacyPrivate, 10)
	e.expect(e.do("asker", "POST", path(prv), `{"message":"cho em vào"}`), 200, "", "xin vào nhóm private")
	e.expect(e.do("asker", "POST", path(prv), ""), 400, "GROUP_JOIN_REQUEST_EXISTS", "xin hai lần")
	// Thông điệp giữ nguyên câu cũ để web khớp theo câu ở những nơi chưa đọc code.
	if r := e.do("asker", "POST", path(prv), ""); r.Body.Message != "you already have a pending join request" {
		t.Errorf("message đổi: %q", r.Body.Message)
	}

	// Duyệt khi nhóm đã đầy.
	full := e.group(model.GroupPrivacyPrivate, 2)
	e.member(full.ID, "member", model.GroupRoleMember, model.GroupMemberActive)
	e.db.Model(&model.Group{}).Where("id = ?", full.ID).Update("member_count", 1) // giả lập đếm lệch khi xin
	e.expect(e.do("friend", "POST", path(full), ""), 200, "", "xin vào")
	e.db.Model(&model.Group{}).Where("id = ?", full.ID).Update("member_count", 2)
	var req model.GroupJoinRequest
	e.db.First(&req, "group_id = ? AND user_id = ?", full.ID, e.ids["friend"])
	e.expect(e.do("owner", "POST", fmt.Sprintf("/api/groups/%s/requests/%s/approve", full.ID, req.ID), ""), 400, "GROUP_FULL", "duyệt khi đầy")
}

func TestGroupRoutes_MyJoinRequest_VaThanhVienTheoVai(t *testing.T) {
	e := newGroupLiveEnv(t)
	g := e.group(model.GroupPrivacyPrivate, 10)
	e.member(g.ID, "gadmin", model.GroupRoleAdmin, model.GroupMemberActive)
	e.member(g.ID, "mod", model.GroupRoleModerator, model.GroupMemberActive)
	e.member(g.ID, "member", model.GroupRoleMember, model.GroupMemberActive)
	e.member(g.ID, "banned", model.GroupRoleMember, model.GroupMemberBanned)

	e.expect(e.do("asker", "POST", fmt.Sprintf("/api/groups/%s/join", g.ID), `{"message":"x"}`), 200, "", "xin vào")
	r := e.do("asker", "GET", "/api/groups/"+g.Slug, "")
	e.expect(r, 200, "", "GET /groups/:slug của người xin")
	if !strings.Contains(r.Raw, `"my_join_request":{"id":"`) || !strings.Contains(r.Raw, `"status":"PENDING"`) {
		t.Errorf("thiếu my_join_request: %s", r.Raw)
	}
	for _, who := range []string{"", "stranger", "owner"} {
		r := e.do(who, "GET", "/api/groups/"+g.Slug, "")
		e.expect(r, 200, "", "GET /groups/:slug "+who)
		if strings.Contains(r.Raw, "my_join_request") {
			t.Errorf("%q không được có my_join_request: %s", who, r.Raw)
		}
	}

	members := fmt.Sprintf("/api/groups/%s/members", g.ID)
	for who, want := range map[string]int{"owner": 200, "gadmin": 200, "mod": 200, "member": 200, "stranger": 403, "banned": 403, "asker": 403, "": 401} {
		e.expect(e.do(who, "GET", members, ""), want, "", "danh sách thành viên nhóm PRIVATE bởi "+who)
	}
	if r := e.do("stranger", "GET", members, ""); r.Body.Code != "ERR_FORBIDDEN" {
		t.Errorf("403 danh sách thành viên cần code ERR_FORBIDDEN: %s", r.Raw)
	}

	// status=BANNED chỉ OWNER/ADMIN.
	for who, want := range map[string]int{"owner": 200, "gadmin": 200, "mod": 403, "member": 403, "stranger": 403, "banned": 403} {
		e.expect(e.do(who, "GET", members+"?status=BANNED", ""), want, "", "danh sách bị cấm bởi "+who)
	}
	r = e.do("owner", "GET", members+"?status=BANNED", "")
	if !strings.Contains(r.Raw, e.ids["banned"].String()) || strings.Contains(r.Raw, e.ids["member"].String()) {
		t.Errorf("status=BANNED chỉ được chứa người bị cấm: %s", r.Raw)
	}
	e.expect(e.do("owner", "GET", members+"?status=LEFT", ""), 400, "ERR_VALIDATION", "status lạ")
	if r := e.do("owner", "GET", members, ""); strings.Contains(r.Raw, e.ids["banned"].String()) {
		t.Errorf("danh sách mặc định không được chứa người bị cấm: %s", r.Raw)
	}
}
