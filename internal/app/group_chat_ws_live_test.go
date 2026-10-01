package app

// Test WebSocket THẬT cho chat nhóm (plan 260930, contract-api.md mục "Sự kiện WebSocket"): dựng đúng cách
// app.go nối route /api/ws (Hub + Notifier + authorizer + AuthMiddleware), mở kết nối gorilla thật và kiểm:
// thành viên nhận conversation_message, người ngoài bị subscribe_denied, người bị gỡ khỏi nhóm ngừng nhận.

import (
	"context"
	"encoding/json"
	"net"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"github.com/gorilla/websocket"
	"github.com/redis/go-redis/v9"
	"study.com/v1/internal/config"
	"study.com/v1/internal/constants"
	"study.com/v1/internal/database"
	"study.com/v1/internal/dto"
	"study.com/v1/internal/middleware"
	"study.com/v1/internal/model"
	"study.com/v1/internal/repository"
	"study.com/v1/internal/service"
	"study.com/v1/internal/socket"
	"study.com/v1/internal/testutil/pgtest"
	"study.com/v1/internal/utils"
)

type wsEnvelope struct {
	Event   string          `json:"event"`
	Payload json.RawMessage `json:"payload"`
}

func wsDial(t *testing.T, addr, token string) *websocket.Conn {
	t.Helper()
	h := map[string][]string{"Authorization": {"Bearer " + token}}
	c, _, err := websocket.DefaultDialer.Dial("ws://"+addr+"/api/ws", h)
	if err != nil {
		t.Fatalf("mở WebSocket: %v", err)
	}
	t.Cleanup(func() { _ = c.Close() })
	return c
}

func wsSubscribe(t *testing.T, c *websocket.Conn, channel string) {
	t.Helper()
	msg := map[string]any{"event": "subscribe", "payload": map[string]string{"channel": channel}}
	if err := c.WriteJSON(msg); err != nil {
		t.Fatal(err)
	}
}

// wsRead đọc envelope kế tiếp trong d; hết hạn thì trả ok=false (dùng cho khẳng định "KHÔNG nhận gì").
func wsRead(c *websocket.Conn, d time.Duration) (wsEnvelope, bool) {
	_ = c.SetReadDeadline(time.Now().Add(d))
	var e wsEnvelope
	if err := c.ReadJSON(&e); err != nil {
		return e, false
	}
	return e, true
}

// wsWaitEvent bỏ qua các envelope khác (pong, connected...) cho tới khi gặp event mong muốn hoặc hết hạn.
func wsWaitEvent(c *websocket.Conn, event string, d time.Duration) (wsEnvelope, bool) {
	deadline := time.Now().Add(d)
	for time.Now().Before(deadline) {
		e, ok := wsRead(c, time.Until(deadline))
		if !ok {
			return e, false
		}
		if e.Event == event {
			return e, true
		}
	}
	return wsEnvelope{}, false
}

func TestGroupChatWS_ThanhVienNhanTin_NguoiNgoaiBiChan_BiGoThiNgung(t *testing.T) {
	db := pgtest.IsolatedSchema(t, database.Migrate)
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = rdb.Close() })
	cfg := &config.Config{JWTSecret: "group-ws-secret", JWTAccessExpiration: 15 * time.Minute, JWTRefreshExpiration: 7 * 24 * time.Hour}

	ids := map[string]uuid.UUID{}
	toks := map[string]string{}
	for _, name := range []string{"owner", "member", "stranger"} {
		u := model.User{Email: name + "-" + uuid.NewString()[:6] + "@40study.test", PasswordHash: "x",
			UserName: name + uuid.NewString()[:6], IsActive: true}
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

	// Nối y như app.go: một Hub/Notifier dùng chung cho service và route /api/ws.
	hub := socket.NewHub()
	go hub.Run()
	notifier := socket.NewNotifier(hub)
	convRepo := repository.NewConversationRepository(db)
	partRepo := repository.NewConversationParticipantRepository(db)
	memberRepo := repository.NewGroupMemberRepository(db)
	convSvc := service.NewConversationService(convRepo, partRepo,
		repository.NewMessageRepository(db), repository.NewMessageReactionRepository(db), notifier,
		repository.NewEnrollmentRepository(db), repository.NewParentStudentRepository(db), repository.NewUserSystemRoleRepository(db))
	groupSvc := service.NewGroupService(repository.NewGroupRepository(db), memberRepo,
		repository.NewGroupJoinRequestRepository(db), convRepo, partRepo)
	groupSvc.SetChannelEvictor(notifier)

	fiberApp := fiber.New()
	sh := socket.NewHandler(hub, newWSChannelAuthorizer(partRepo, memberRepo))
	fiberApp.Get("/api/ws", middleware.AuthMiddleware(cfg, rdb), sh.HandleWebSocket)
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	go func() { _ = fiberApp.Listener(ln) }()
	t.Cleanup(func() { _ = fiberApp.Shutdown() })
	addr := ln.Addr().String()

	// Nhóm PUBLIC: member vào bằng JoinGroup (vào thẳng), người ngoài thì không.
	ctx := context.Background()
	g, err := groupSvc.CreateGroup(ctx, ids["owner"], dto.CreateGroupRequest{Name: "Nhom WS", Privacy: "PUBLIC"})
	if err != nil {
		t.Fatalf("tạo nhóm: %v", err)
	}
	if _, err := groupSvc.JoinGroup(ctx, ids["member"], g.ID, nil); err != nil {
		t.Fatalf("member vào nhóm: %v", err)
	}
	var conv model.Conversation
	if err := db.First(&conv, "group_id = ?", g.ID).Error; err != nil {
		t.Fatalf("hội thoại nhóm: %v", err)
	}
	channel := "conversation:" + conv.ID.String()

	memberWS, strangerWS, ownerWS := wsDial(t, addr, toks["member"]), wsDial(t, addr, toks["stranger"]), wsDial(t, addr, toks["owner"])
	wsSubscribe(t, memberWS, channel)
	wsSubscribe(t, ownerWS, channel)
	wsSubscribe(t, strangerWS, channel)

	// Người ngoài nhóm: bị từ chối subscribe, không nhận tin.
	if e, ok := wsWaitEvent(strangerWS, "error", 3*time.Second); !ok || !json.Valid(e.Payload) ||
		!contains2(string(e.Payload), "subscribe_denied") {
		t.Fatalf("người ngoài phải nhận error subscribe_denied, nhận %+v ok=%v", e, ok)
	}
	time.Sleep(300 * time.Millisecond) // chờ hub ghi nhận hai subscribe hợp lệ

	text := "xin chào cả nhóm"
	if _, err := convSvc.SendMessage(ctx, ids["owner"], conv.ID, dto.SendMessageRequest{Content: &text, Type: "TEXT"}); err != nil {
		t.Fatalf("gửi tin: %v", err)
	}
	e, ok := wsWaitEvent(memberWS, "conversation_message", 3*time.Second)
	if !ok {
		t.Fatal("thành viên phải nhận conversation_message realtime")
	}
	var got dto.MessageResponse
	if err := json.Unmarshal(e.Payload, &got); err != nil || got.ConversationID != conv.ID || got.Content == nil || *got.Content != text {
		t.Fatalf("payload conversation_message sai: err=%v %s", err, e.Payload)
	}
	if _, ok := wsWaitEvent(strangerWS, "conversation_message", 500*time.Millisecond); ok {
		t.Fatal("người ngoài nhóm không được nhận tin nhóm")
	}

	// Bị gỡ khỏi nhóm thì kết nối đang mở ngừng nhận (evict), dù vẫn còn kết nối.
	if err := groupSvc.RemoveMember(ctx, ids["owner"], g.ID, ids["member"]); err != nil {
		t.Fatalf("gỡ thành viên: %v", err)
	}
	text2 := "tin sau khi gỡ"
	if _, err := convSvc.SendMessage(ctx, ids["owner"], conv.ID, dto.SendMessageRequest{Content: &text2, Type: "TEXT"}); err != nil {
		t.Fatal(err)
	}
	if _, ok := wsWaitEvent(ownerWS, "conversation_message", 3*time.Second); !ok {
		t.Fatal("chủ nhóm (vẫn trong nhóm) phải nhận tin thứ hai — kiểm chứng kênh còn sống")
	}
	if e, ok := wsWaitEvent(memberWS, "conversation_message", 700*time.Millisecond); ok {
		t.Fatalf("thành viên đã bị gỡ vẫn nhận tin: %s", e.Payload)
	}
}

func contains2(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
