package app

// Test WebSocket THẬT cho chặn trong DM 1-1 (L3, sau e2e 01/10):
//   - "đang gõ" không được phát cho phía bên kia khi hai người chặn nhau (bất kỳ chiều nào), nhưng vẫn phát
//     bình thường khi không chặn và ở chat nhóm;
//   - chặn/bỏ chặn phát `conversation_blocked_changed {conversation_id, is_blocked}` tới CẢ HAI người, cùng một
//     payload, không nói ai chặn ai, không tới người thứ ba, và chỉ phát khi trạng thái khoá hiệu lực đổi.
// Bỏ typingAllowed khỏi handler, bỏ SetDirectBlockChecker/SetDirectBlockPublisher, hoặc phát khi trạng thái
// không đổi thì test ĐỎ.

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
	"study.com/v1/internal/middleware"
	"study.com/v1/internal/model"
	"study.com/v1/internal/repository"
	"study.com/v1/internal/service"
	"study.com/v1/internal/socket"
	"study.com/v1/internal/testutil/pgtest"
	"study.com/v1/internal/utils"
)

type blockWSFx struct {
	t       *testing.T
	friends *service.FriendshipService
	ids     map[string]uuid.UUID
	conns   map[string]*websocket.Conn
	inbox   map[string]chan wsEnvelope
	dm      uuid.UUID
	group   uuid.UUID
}

func newBlockWSFx(t *testing.T) *blockWSFx {
	t.Helper()
	db := pgtest.IsolatedSchema(t, database.Migrate)
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = rdb.Close() })
	cfg := &config.Config{JWTSecret: "dm-block-ws-secret", JWTAccessExpiration: 15 * time.Minute, JWTRefreshExpiration: 7 * 24 * time.Hour}

	fx := &blockWSFx{t: t, ids: map[string]uuid.UUID{}, conns: map[string]*websocket.Conn{}, inbox: map[string]chan wsEnvelope{}}
	toks := map[string]string{}
	for _, name := range []string{"a", "b", "c"} {
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
		fx.ids[name], toks[name] = u.ID, tok
	}

	// DM giữa a và b; chat nhóm (không phải DM) cũng giữa a và b để chứng minh khoá chỉ áp dụng cho DM.
	mkConv := func(typ model.ConversationType) uuid.UUID {
		c := model.Conversation{Type: typ}
		if err := db.Create(&c).Error; err != nil {
			t.Fatal(err)
		}
		for _, n := range []string{"a", "b"} {
			if err := db.Create(&model.ConversationParticipant{ConversationID: c.ID, UserID: fx.ids[n]}).Error; err != nil {
				t.Fatal(err)
			}
		}
		return c.ID
	}
	fx.dm, fx.group = mkConv(model.ConversationTypeDirect), mkConv(model.ConversationTypeGroup)

	// Nối y như app.go: Hub/Notifier dùng chung, authorizer + services nối checker/publisher.
	hub := socket.NewHub()
	go hub.Run()
	notifier := socket.NewNotifier(hub)
	convRepo := repository.NewConversationRepository(db)
	partRepo := repository.NewConversationParticipantRepository(db)
	memberRepo := repository.NewGroupMemberRepository(db)
	convSvc := service.NewConversationService(convRepo, partRepo,
		repository.NewMessageRepository(db), repository.NewMessageReactionRepository(db), notifier,
		repository.NewEnrollmentRepository(db), repository.NewParentStudentRepository(db), repository.NewUserSystemRoleRepository(db))
	fx.friends = service.NewFriendshipService(repository.NewFriendshipRepository(db), repository.NewUserBlockRepository(db))
	convSvc.SetFriendshipChecker(fx.friends)
	authz := newWSChannelAuthorizer(partRepo, memberRepo)
	// Dây nối thật của app (không tự Set từng cái): bỏ dòng nào trong wireDirectBlockRealtime thì test đỏ.
	wireDirectBlockRealtime(&Services{Conversation: convSvc, Friendship: fx.friends}, authz)

	fiberApp := fiber.New()
	sh := socket.NewHandler(hub, authz)
	fiberApp.Get("/api/ws", middleware.AuthMiddleware(cfg, rdb), sh.HandleWebSocket)
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	go func() { _ = fiberApp.Listener(ln) }()
	t.Cleanup(func() { _ = fiberApp.Shutdown() })

	for _, n := range []string{"a", "b", "c"} {
		fx.conns[n] = wsDial(t, ln.Addr().String(), toks[n])
		// Một goroutine đọc liên tục vào hộp thư: gorilla hỏng vĩnh viễn sau lần đọc hết hạn nên không thể
		// dùng wsWaitEvent (đọc có deadline) cho các khẳng định "KHÔNG nhận gì" rồi đọc tiếp.
		inbox := make(chan wsEnvelope, 256)
		fx.inbox[n] = inbox
		go func(c *websocket.Conn) {
			for {
				var e wsEnvelope
				if err := c.ReadJSON(&e); err != nil {
					return
				}
				inbox <- e
			}
		}(fx.conns[n])
	}
	for _, n := range []string{"a", "b"} {
		wsSubscribe(t, fx.conns[n], "conversation:"+fx.dm.String())
		wsSubscribe(t, fx.conns[n], "conversation:"+fx.group.String())
	}
	time.Sleep(500 * time.Millisecond) // chờ hub ghi nhận các subscribe
	return fx
}

func (fx *blockWSFx) type_(from string, conv uuid.UUID) {
	fx.t.Helper()
	msg := map[string]any{"event": "conversation_typing", "payload": map[string]any{
		"conversation_id": conv.String(), "user_name": from, "is_typing": true}}
	if err := fx.conns[from].WriteJSON(msg); err != nil {
		fx.t.Fatal(err)
	}
}

// waitEvent chờ envelope có event mong muốn trong hộp thư của `who` (bỏ qua envelope khác); hết hạn thì ok=false.
func (fx *blockWSFx) waitEvent(who, event string, d time.Duration) (json.RawMessage, bool) {
	timer := time.NewTimer(d)
	defer timer.Stop()
	for {
		select {
		case e := <-fx.inbox[who]:
			if e.Event == event {
				return e.Payload, true
			}
		case <-timer.C:
			return nil, false
		}
	}
}

// gotTyping: `to` có nhận conversation_typing trong d không.
func (fx *blockWSFx) gotTyping(to string, d time.Duration) bool {
	_, ok := fx.waitEvent(to, "conversation_typing", d)
	return ok
}

// blockedEvent chờ conversation_blocked_changed của `who`; trả payload thô.
func (fx *blockWSFx) blockedEvent(who string, d time.Duration) (json.RawMessage, bool) {
	return fx.waitEvent(who, "conversation_blocked_changed", d)
}

func (fx *blockWSFx) wantBlockedEvent(want bool, why string) {
	fx.t.Helper()
	pa, okA := fx.blockedEvent("a", 3*time.Second)
	pb, okB := fx.blockedEvent("b", 3*time.Second)
	if !okA || !okB {
		fx.t.Fatalf("%s: cả hai phải nhận conversation_blocked_changed (a=%v b=%v)", why, okA, okB)
	}
	if string(pa) != string(pb) {
		fx.t.Errorf("%s: hai phía phải nhận payload giống hệt nhau, a=%s b=%s", why, pa, pb)
	}
	var m map[string]any
	if err := json.Unmarshal(pa, &m); err != nil {
		fx.t.Fatal(err)
	}
	if len(m) != 2 || m["conversation_id"] != fx.dm.String() || m["is_blocked"] != want {
		fx.t.Errorf("%s: payload phải đúng {conversation_id, is_blocked=%v} và không có field nào khác (không nói ai chặn ai): %s", why, want, pa)
	}
}

func (fx *blockWSFx) wantNoBlockedEvent(why string) {
	fx.t.Helper()
	for _, n := range []string{"a", "b"} {
		if p, ok := fx.blockedEvent(n, 700*time.Millisecond); ok {
			fx.t.Errorf("%s: %s không được nhận sự kiện: %s", why, n, p)
		}
	}
}

func TestDMBlockWS_DangGoVaSuKienChan(t *testing.T) {
	fx := newBlockWSFx(t)
	ctx := context.Background()
	a, b := fx.ids["a"], fx.ids["b"]

	// Đối chứng: chưa chặn thì "đang gõ" tới phía bên kia (cả DM lẫn chat nhóm).
	fx.type_("a", fx.dm)
	if !fx.gotTyping("b", 3*time.Second) {
		t.Fatal("chưa chặn: b phải nhận conversation_typing của a")
	}

	// a chặn b: cả hai nhận sự kiện khoá; người thứ ba (c) không nhận gì.
	if err := fx.friends.Block(ctx, a, b); err != nil {
		t.Fatalf("Block: %v", err)
	}
	fx.wantBlockedEvent(true, "a chặn b")
	if p, ok := fx.blockedEvent("c", 500*time.Millisecond); ok {
		t.Errorf("người ngoài DM không được nhận sự kiện chặn: %s", p)
	}

	// Khoá "đang gõ" hai chiều.
	fx.type_("a", fx.dm)
	if fx.gotTyping("b", 700*time.Millisecond) {
		t.Error("a chặn b: b không được nhận 'đang gõ' của a")
	}
	fx.type_("b", fx.dm)
	if fx.gotTyping("a", 700*time.Millisecond) {
		t.Error("a chặn b: a không được nhận 'đang gõ' của b (khoá cả chiều người bị chặn)")
	}
	// Chat nhóm không bị ảnh hưởng.
	fx.type_("a", fx.group)
	if !fx.gotTyping("b", 3*time.Second) {
		t.Error("chat nhóm: 'đang gõ' vẫn phải tới b dù a đã chặn b")
	}

	// b chặn lại a: trạng thái khoá hiệu lực không đổi nên KHÔNG phát gì.
	if err := fx.friends.Block(ctx, b, a); err != nil {
		t.Fatal(err)
	}
	fx.wantNoBlockedEvent("b chặn lại khi đã bị chặn")
	// a bỏ chặn nhưng b vẫn chặn a: vẫn khoá, KHÔNG phát (không tiết lộ b còn chặn).
	if err := fx.friends.Unblock(ctx, a, b); err != nil {
		t.Fatal(err)
	}
	fx.wantNoBlockedEvent("a bỏ chặn khi b vẫn chặn")
	fx.type_("a", fx.dm)
	if fx.gotTyping("b", 700*time.Millisecond) {
		t.Error("b vẫn chặn a: 'đang gõ' vẫn phải bị khoá")
	}

	// b bỏ chặn: hết chặn mọi chiều, cả hai nhận is_blocked=false và "đang gõ" chạy lại.
	if err := fx.friends.Unblock(ctx, b, a); err != nil {
		t.Fatal(err)
	}
	fx.wantBlockedEvent(false, "hết chặn mọi chiều")
	fx.type_("a", fx.dm)
	if !fx.gotTyping("b", 3*time.Second) {
		t.Error("hết chặn: 'đang gõ' phải chạy lại")
	}

	// Bỏ chặn khi chưa chặn (idempotent): không phát.
	if err := fx.friends.Unblock(ctx, a, b); err != nil {
		t.Fatal(err)
	}
	fx.wantNoBlockedEvent("bỏ chặn khi chưa chặn")
}
