package router

// Phase 4: route THẬT + AuthMiddleware THẬT (miniredis) + permChecker THẬT; chỉ service giả.
//  - STUDENT bị 403 trước handler (SYSTEM_SETTINGS_MANAGE).
//  - POST /broadcast: tối đa 5 lần/giờ THEO ADMIN (lần 6 = 429), admin khác không bị ảnh hưởng.
//  - POST /broadcast/preview không bị giới hạn.

import (
	"context"
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
	"study.com/v1/internal/dto"
	"study.com/v1/internal/handler"
	"study.com/v1/internal/middleware"
	"study.com/v1/internal/utils"
)

type broadcastRouteSvc struct{ sends, previews int }

func (s *broadcastRouteSvc) Preview(context.Context, dto.BroadcastPreviewRequestDTO) (*dto.BroadcastPreviewDTO, error) {
	s.previews++
	return &dto.BroadcastPreviewDTO{RecipientCount: 1}, nil
}

func (s *broadcastRouteSvc) Send(context.Context, dto.BroadcastRequestDTO) (*dto.BroadcastResultDTO, error) {
	s.sends++
	return &dto.BroadcastResultDTO{RecipientCount: 1, Audience: "all", Roles: []string{}, NotificationType: "system"}, nil
}

type broadcastRouteEnv struct {
	app *fiber.App
	svc *broadcastRouteSvc
	tok func() string // token cho một user MỚI
}

// perms: quyền của mọi user trong env này (fake repo trả cùng một vai trò cho mọi user).
func newBroadcastRouteEnv(t *testing.T, perms ...string) *broadcastRouteEnv {
	t.Helper()
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = rdb.Close() })
	cfg := &config.Config{JWTSecret: "admin-broadcast-route-test-secret", JWTAccessExpiration: 15 * time.Minute, JWTRefreshExpiration: time.Hour}

	roleID := uuid.New()
	permChecker := middleware.NewPermissionChecker(&fakeUSRRepoCR{systemRoleID: roleID}, &fakeSRRepoCR{perms: map[uuid.UUID][]string{roleID: perms}}, nil, nil)
	tok := func() string {
		id := uuid.New()
		if err := rdb.Set(context.Background(), constants.KeyUserVersion(id.String()), int64(1), 0).Err(); err != nil {
			t.Fatal(err)
		}
		s, _, err := utils.GenerateTokens(cfg, id, uuid.New(), "SYSTEM_ADMIN", nil, 1)
		if err != nil {
			t.Fatal(err)
		}
		return s
	}

	svc := &broadcastRouteSvc{}
	app := fiber.New()
	SetupAdminBroadcastRoutes(app.Group("/api"), cfg, handler.NewAdminBroadcastHandler(svc), rdb, permChecker)
	return &broadcastRouteEnv{app: app, svc: svc, tok: tok}
}

func (e *broadcastRouteEnv) post(t *testing.T, token, path string) int {
	t.Helper()
	body := `{"title":"a","content":"b","audience":"all"}`
	req := httptest.NewRequest("POST", path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := e.app.Test(req, -1)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	_, _ = io.ReadAll(resp.Body)
	return resp.StatusCode
}

func TestBroadcastRoutes_RateLimit5MoiGioTheoAdmin(t *testing.T) {
	env := newBroadcastRouteEnv(t, "SYSTEM_SETTINGS_MANAGE")
	adminA, adminB := env.tok(), env.tok()

	for i := 1; i <= adminBroadcastMaxPerHour; i++ {
		if got := env.post(t, adminA, "/api/admin/notifications/broadcast"); got != fiber.StatusCreated {
			t.Fatalf("lần gửi %d = %d, muốn 201", i, got)
		}
	}
	if got := env.post(t, adminA, "/api/admin/notifications/broadcast"); got != fiber.StatusTooManyRequests {
		t.Fatalf("lần gửi thứ %d = %d, muốn 429", adminBroadcastMaxPerHour+1, got)
	}
	if env.svc.sends != adminBroadcastMaxPerHour {
		t.Fatalf("service được gọi %d lần, muốn %d (lần bị 429 không được chạm service)", env.svc.sends, adminBroadcastMaxPerHour)
	}
	// Khoá theo user id chứ không theo IP: app.Test dùng chung một IP giả lập cho mọi request.
	if got := env.post(t, adminB, "/api/admin/notifications/broadcast"); got != fiber.StatusCreated {
		t.Fatalf("admin khác sau khi admin A bị chặn = %d, muốn 201 (hạn mức phải theo user id)", got)
	}
	// Xem trước không bị giới hạn.
	for i := 0; i < adminBroadcastMaxPerHour+2; i++ {
		if got := env.post(t, adminA, "/api/admin/notifications/broadcast/preview"); got != fiber.StatusOK {
			t.Fatalf("preview lần %d = %d, muốn 200", i+1, got)
		}
	}
}

func TestBroadcastRoutes_KhongCoQuyenBiTuChoi403(t *testing.T) {
	env := newBroadcastRouteEnv(t) // không có quyền nào
	tok := env.tok()
	for _, path := range []string{"/api/admin/notifications/broadcast", "/api/admin/notifications/broadcast/preview"} {
		if got := env.post(t, tok, path); got != fiber.StatusForbidden {
			t.Errorf("POST %s thiếu SYSTEM_SETTINGS_MANAGE = %d, muốn 403", path, got)
		}
	}
	if env.svc.sends != 0 || env.svc.previews != 0 {
		t.Error("service bị gọi dù permChecker phải chặn trước handler")
	}
	if got := env.post(t, "not-a-token", "/api/admin/notifications/broadcast"); got != fiber.StatusUnauthorized {
		t.Errorf("không đăng nhập = %d, muốn 401", got)
	}
}
