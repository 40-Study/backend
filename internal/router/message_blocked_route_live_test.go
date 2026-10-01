package router

// Test HTTP qua route THẬT: DM 1-1 bị chặn -> gửi/sửa/xoá trả 403 + code ERR_CONVERSATION_BLOCKED cho CẢ HAI
// phía, đọc lịch sử vẫn 200, bỏ chặn thì gửi lại 201 (quyết định chủ dự án sau review đối kháng #102 M2).

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

func TestMessageRoutes_DMBiChan_KhoaGuiHaiChieu(t *testing.T) {
	db := pgtest.IsolatedSchema(t, database.Migrate)
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = rdb.Close() })
	cfg := &config.Config{JWTSecret: "msg-block-secret", JWTAccessExpiration: 15 * time.Minute, JWTRefreshExpiration: 7 * 24 * time.Hour}

	ids, toks := map[string]uuid.UUID{}, map[string]string{}
	for _, name := range []string{"a", "b"} {
		u := model.User{Email: name + "-" + uuid.NewString()[:6] + "@40study.test", PasswordHash: "x", UserName: name + uuid.NewString()[:6], IsActive: true}
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

	convSvc := service.NewConversationService(
		repository.NewConversationRepository(db), repository.NewConversationParticipantRepository(db),
		repository.NewMessageRepository(db), repository.NewMessageReactionRepository(db),
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
			Code string          `json:"code"`
			Data json.RawMessage `json:"data"`
		}
		_ = json.Unmarshal(raw, &b)
		return res.StatusCode, b.Code, string(raw)
	}

	st, _, raw := do("a", "POST", "/api/conversations/direct", `{"user_id":"`+ids["b"].String()+`"}`)
	if st != 201 {
		t.Fatalf("tạo DM: %d %s", st, raw)
	}
	var created struct {
		Data struct {
			ID uuid.UUID `json:"id"`
		} `json:"data"`
	}
	_ = json.Unmarshal([]byte(raw), &created)
	conv := created.Data.ID.String()
	st, _, raw = do("a", "POST", "/api/conversations/"+conv+"/messages", `{"content":"trước khi chặn","type":"TEXT"}`)
	if st != 201 {
		t.Fatalf("tiền đề gửi khi chưa chặn: %d %s", st, raw)
	}
	var sent struct {
		Data struct {
			ID uuid.UUID `json:"id"`
		} `json:"data"`
	}
	_ = json.Unmarshal([]byte(raw), &sent)
	msg := sent.Data.ID.String()

	// b chặn a: cả hai phía bị khoá gửi/sửa/xoá, lịch sử vẫn đọc được.
	if err := db.Create(&model.UserBlock{BlockerID: ids["b"], BlockedID: ids["a"]}).Error; err != nil {
		t.Fatal(err)
	}
	for _, who := range []string{"a", "b"} {
		st, code, raw := do(who, "POST", "/api/conversations/"+conv+"/messages", `{"content":"sau khi chặn","type":"TEXT"}`)
		if st != 403 || code != "ERR_CONVERSATION_BLOCKED" {
			t.Errorf("%s gửi: muốn 403/ERR_CONVERSATION_BLOCKED, nhận %d %s", who, st, raw)
		}
		if !strings.Contains(raw, "Không thể gửi tin nhắn trong cuộc trò chuyện này") {
			t.Errorf("%s: thông điệp tiếng Việt cho web: %s", who, raw)
		}
		if st, _, raw := do(who, "GET", "/api/conversations/"+conv+"/messages", ""); st != 200 || !strings.Contains(raw, "trước khi chặn") {
			t.Errorf("%s đọc lịch sử phải 200 và còn tin cũ: %d %s", who, st, raw)
		}
	}
	if st, code, raw := do("a", "PUT", "/api/conversations/"+conv+"/messages/"+msg, `{"content":"sửa"}`); st != 403 || code != "ERR_CONVERSATION_BLOCKED" {
		t.Errorf("sửa: muốn 403/ERR_CONVERSATION_BLOCKED, nhận %d %s", st, raw)
	}
	if st, code, raw := do("a", "DELETE", "/api/conversations/"+conv+"/messages/"+msg, ""); st != 403 || code != "ERR_CONVERSATION_BLOCKED" {
		t.Errorf("xoá: muốn 403/ERR_CONVERSATION_BLOCKED, nhận %d %s", st, raw)
	}

	// Bỏ chặn: gửi lại bình thường.
	db.Where("blocker_id = ?", ids["b"]).Delete(&model.UserBlock{})
	if st, _, raw := do("a", "POST", "/api/conversations/"+conv+"/messages", `{"content":"sau khi bỏ chặn","type":"TEXT"}`); st != 201 {
		t.Errorf("bỏ chặn phải gửi lại được: %d %s", st, raw)
	}
}
