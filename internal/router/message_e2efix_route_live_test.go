package router

// Test HTTP qua route THẬT cho các lỗi tìm thấy khi kiểm chứng đầu-cuối nhóm/bạn bè:
//   - F2: GET /conversations/:id của DM 1-1 có is_blocked (true khi giữa hai người có chặn, hai phía thấy như nhau);
//   - F3: người đã rời nhóm đọc tin nhận 404 + CONVERSATION_NOT_FOUND, y hệt hội thoại không tồn tại (trước đây 400);
//   - F4: tin nhắn có sender_full_name.

import (
	"context"
	"encoding/json"
	"io"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
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

func TestMessageRoutes_E2EFix_DMBlockedFlag_NotParticipant404_SenderFullName(t *testing.T) {
	db := pgtest.IsolatedSchema(t, database.Migrate)
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = rdb.Close() })
	cfg := &config.Config{JWTSecret: "msg-e2efix-secret", JWTAccessExpiration: 15 * time.Minute, JWTRefreshExpiration: 7 * 24 * time.Hour}

	ids, toks := map[string]uuid.UUID{}, map[string]string{}
	for _, name := range []string{"a", "b", "x"} {
		fullName := "Họ Tên " + strings.ToUpper(name)
		u := model.User{Email: name + "-" + uuid.NewString()[:6] + "@40study.test", PasswordHash: "x", UserName: name + uuid.NewString()[:6], FullName: &fullName, IsActive: true}
		if err := db.Create(&u).Error; err != nil {
			t.Fatal(err)
		}
		if err := rdb.Set(context.Background(), constants.KeyUserVersion(u.ID.String()), 1, 0).Err(); err != nil {
			t.Fatal(err)
		}
		tok, _, err := utils.GenerateTokens(cfg, u.ID, uuid.New(), "STUDENT", nil, 1)
		if err != nil {
			t.Fatal(err)
		}
		ids[name], toks[name] = u.ID, tok
	}
	now := time.Now()
	if err := db.Create(&model.Friendship{RequesterID: ids["a"], AddresseeID: ids["b"], Status: model.FriendshipStatusAccepted, RequestedAt: now, RespondedAt: &now}).Error; err != nil {
		t.Fatal(err)
	}

	convRepo := repository.NewConversationRepository(db)
	partRepo := repository.NewConversationParticipantRepository(db)
	convSvc := service.NewConversationService(
		convRepo, partRepo, repository.NewMessageRepository(db), repository.NewMessageReactionRepository(db),
		socket.NewNotifier(socket.NewHub()),
		repository.NewEnrollmentRepository(db), repository.NewParentStudentRepository(db), repository.NewUserSystemRoleRepository(db))
	convSvc.SetFriendshipChecker(service.NewFriendshipService(repository.NewFriendshipRepository(db), repository.NewUserBlockRepository(db)))
	app := fiber.New()
	SetupMessageRoutes(app.Group("/api"), cfg, handler.NewMessageHandler(convSvc), rdb)

	do := func(who, method, path, body string) (int, string, string) {
		req := httptest.NewRequest(method, path, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+toks[who])
		res, err := app.Test(req, -1)
		if err != nil {
			t.Fatal(err)
		}
		defer res.Body.Close()
		raw, _ := io.ReadAll(res.Body)
		var b struct {
			Code string `json:"code"`
		}
		_ = json.Unmarshal(raw, &b)
		return res.StatusCode, b.Code, string(raw)
	}
	type convBody struct {
		Data struct {
			ID        uuid.UUID `json:"id"`
			IsBlocked *bool     `json:"is_blocked"`
		} `json:"data"`
	}
	getConv := func(who string, id uuid.UUID) convBody {
		st, _, raw := do(who, "GET", "/api/conversations/"+id.String(), "")
		if st != 200 {
			t.Fatalf("%s GET conversation: %d %s", who, st, raw)
		}
		var out convBody
		_ = json.Unmarshal([]byte(raw), &out)
		return out
	}

	// --- F2: cờ is_blocked của DM ---
	st, _, raw := do("a", "POST", "/api/conversations/direct", `{"user_id":"`+ids["b"].String()+`"}`)
	if st != 201 {
		t.Fatalf("tạo DM: %d %s", st, raw)
	}
	var created convBody
	_ = json.Unmarshal([]byte(raw), &created)
	dm := created.Data.ID
	if created.Data.IsBlocked == nil || *created.Data.IsBlocked {
		t.Errorf("DM mới tạo, chưa chặn: muốn is_blocked=false, nhận %s", raw)
	}
	if c := getConv("a", dm); c.Data.IsBlocked == nil || *c.Data.IsBlocked {
		t.Errorf("GET DM chưa chặn: muốn is_blocked=false, nhận %v", c.Data.IsBlocked)
	}
	if err := db.Create(&model.UserBlock{BlockerID: ids["b"], BlockedID: ids["a"]}).Error; err != nil {
		t.Fatal(err)
	}
	for _, who := range []string{"a", "b"} {
		if c := getConv(who, dm); c.Data.IsBlocked == nil || !*c.Data.IsBlocked {
			t.Errorf("%s GET DM đã bị chặn: muốn is_blocked=true, nhận %v", who, c.Data.IsBlocked)
		}
	}
	_, _, raw = do("a", "GET", "/api/conversations/"+dm.String(), "")
	if s := strings.ToLower(raw); strings.Contains(s, "blocker") || strings.Contains(s, "blocked_by") {
		t.Errorf("response lộ ai chặn ai: %s", raw)
	}

	// --- F3 + F4: chat nhóm ---
	g := model.Group{Name: "E2EFIX route", Slug: "e2efix-" + uuid.NewString(), CreatedBy: ids["a"], Privacy: model.GroupPrivacyPublic, MaxMembers: 10}
	if err := db.Create(&g).Error; err != nil {
		t.Fatal(err)
	}
	gc := model.Conversation{Type: model.ConversationTypeGroup, GroupID: &g.ID}
	if err := db.Create(&gc).Error; err != nil {
		t.Fatal(err)
	}
	for _, u := range []string{"a", "b"} {
		if err := db.Create(&model.ConversationParticipant{ConversationID: gc.ID, UserID: ids[u]}).Error; err != nil {
			t.Fatal(err)
		}
	}
	if c := getConv("a", gc.ID); c.Data.IsBlocked != nil {
		t.Errorf("chat nhóm không được có is_blocked, nhận %v", *c.Data.IsBlocked)
	}
	path := "/api/conversations/" + gc.ID.String() + "/messages"
	st, _, raw = do("b", "POST", path, `{"content":"chào cả nhóm","type":"TEXT"}`)
	if st != 201 || !strings.Contains(raw, `"sender_full_name":"Họ Tên B"`) {
		t.Fatalf("gửi tin nhóm: muốn 201 kèm sender_full_name, nhận %d %s", st, raw)
	}
	if st, _, raw := do("a", "GET", path, ""); st != 200 || !strings.Contains(raw, `"sender_full_name":"Họ Tên B"`) {
		t.Errorf("danh sách tin: muốn sender_full_name, nhận %d %s", st, raw)
	}

	if err := partRepo.MarkLeft(context.Background(), gc.ID, ids["b"]); err != nil {
		t.Fatal(err)
	}
	stLeft, codeLeft, rawLeft := do("b", "GET", path, "")
	stNone, codeNone, rawNone := do("b", "GET", "/api/conversations/"+uuid.NewString()+"/messages", "")
	stOut, codeOut, _ := do("x", "GET", path, "")
	if stLeft != 404 || codeLeft != "CONVERSATION_NOT_FOUND" {
		t.Errorf("người đã rời đọc tin: muốn 404/CONVERSATION_NOT_FOUND, nhận %d %s", stLeft, rawLeft)
	}
	if stOut != 404 || codeOut != "CONVERSATION_NOT_FOUND" {
		t.Errorf("người ngoài đọc tin: muốn 404/CONVERSATION_NOT_FOUND, nhận %d %s", stOut, codeOut)
	}
	// Không lộ hội thoại có thật hay không: phản hồi y hệt hội thoại không tồn tại.
	if stLeft != stNone || codeLeft != codeNone || rawLeft != rawNone {
		t.Errorf("hội thoại có thật và không tồn tại phải cùng một phản hồi: %d %s vs %d %s", stLeft, rawLeft, stNone, rawNone)
	}
	// Chi tiết hội thoại cùng một phản hồi 404 (trước đây GET detail trả chuỗi tiếng Anh không mã, lệch với GET messages).
	stDLeft, codeDLeft, rawDLeft := do("b", "GET", "/api/conversations/"+gc.ID.String(), "")
	stDNone, _, rawDNone := do("b", "GET", "/api/conversations/"+uuid.NewString(), "")
	if stDLeft != 404 || codeDLeft != "CONVERSATION_NOT_FOUND" {
		t.Errorf("người đã rời xem chi tiết: muốn 404/CONVERSATION_NOT_FOUND, nhận %d %s", stDLeft, rawDLeft)
	}
	if stDLeft != stDNone || rawDLeft != rawDNone {
		t.Errorf("chi tiết: hội thoại có thật và không tồn tại phải cùng một phản hồi: %d %s vs %d %s", stDLeft, rawDLeft, stDNone, rawDNone)
	}
	// Người còn trong nhóm vẫn đọc được.
	if st, _, raw := do("a", "GET", path, ""); st != 200 {
		t.Errorf("thành viên còn lại phải đọc được: %d %s", st, raw)
	}
}
