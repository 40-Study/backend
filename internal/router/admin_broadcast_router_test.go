package router

// Phase 4: route THẬT + AuthMiddleware THẬT (miniredis) + permChecker THẬT; chỉ service giả.
//  - STUDENT bị 403 trước handler (SYSTEM_SETTINGS_MANAGE).
//  - POST /broadcast: tối đa 5 lần/giờ THEO ADMIN (lần 6 = 429), admin khác không bị ảnh hưởng.
//  - POST /broadcast/preview không bị giới hạn.

import (
	"context"
	"errors"
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
	"study.com/v1/internal/service"
	"study.com/v1/internal/utils"
)

type broadcastRouteSvc struct {
	sends, previews int
	sendErr         error // khác nil: Send trả lỗi này thay vì thành công
}

func (s *broadcastRouteSvc) Preview(context.Context, dto.BroadcastPreviewRequestDTO) (*dto.BroadcastPreviewDTO, error) {
	s.previews++
	return &dto.BroadcastPreviewDTO{RecipientCount: 1}, nil
}

func (s *broadcastRouteSvc) Send(context.Context, dto.BroadcastRequestDTO) (*dto.BroadcastResultDTO, error) {
	s.sends++
	if s.sendErr != nil {
		return nil, s.sendErr
	}
	return &dto.BroadcastResultDTO{RecipientCount: 1, Audience: "all", Roles: []string{}, NotificationType: "system"}, nil
}

type broadcastRouteEnv struct {
	app   *fiber.App
	svc   *broadcastRouteSvc
	audit *auditSpyRouter
	tok   func() string // token cho một user MỚI
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
	audit := &auditSpyRouter{}
	app := fiber.New()
	SetupAdminBroadcastRoutes(app.Group("/api"), cfg, handler.NewAdminBroadcastHandler(svc), rdb, permChecker, audit)
	return &broadcastRouteEnv{app: app, svc: svc, audit: audit, tok: tok}
}

const validBroadcastBody = `{"title":"a","content":"b","audience":"all"}`

func (e *broadcastRouteEnv) post(t *testing.T, token, path string) int {
	t.Helper()
	return e.postWith(t, token, path, validBroadcastBody, "").status
}

type broadcastRouteResp struct {
	status int
	body   string
	replay string
}

// postWith gửi body tuỳ ý, kèm Idempotency-Key nếu idemKey != "".
func (e *broadcastRouteEnv) postWith(t *testing.T, token, path, body, idemKey string) broadcastRouteResp {
	t.Helper()
	req := httptest.NewRequest("POST", path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	if idemKey != "" {
		req.Header.Set(middleware.IdempotencyKeyHeader, idemKey)
	}
	resp, err := e.app.Test(req, -1)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	return broadcastRouteResp{status: resp.StatusCode, body: string(raw), replay: resp.Header.Get(middleware.IdempotencyReplayHeader)}
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

const broadcastSendPath = "/api/admin/notifications/broadcast"

// M4 (review 261009): chỉ lần gửi THỰC SỰ giao >= 1 thông báo mới tiêu hạn mức 5/giờ. Lỗi validate, NO_RECIPIENTS và lỗi
// hạ tầng khi chưa giao ai (500 thường) không được khoá quản trị viên.
func TestBroadcastRoutes_RateLimit_OnlyDeliveringSendsConsumeQuota(t *testing.T) {
	env := newBroadcastRouteEnv(t, "SYSTEM_SETTINGS_MANAGE")
	admin := env.tok()
	badBodies := []string{
		`{"title":"","content":"b","audience":"all"}`, // 400 VALIDATION_FAILED
		`{"title":`, // JSON hỏng
	}
	for i := 0; i < adminBroadcastMaxPerHour+2; i++ {
		for _, body := range badBodies {
			if r := env.postWith(t, admin, broadcastSendPath, body, ""); r.status != fiber.StatusBadRequest {
				t.Fatalf("body lỗi = %d %s, muốn 400 (không bao giờ 429 khi chưa giao được lần nào)", r.status, r.body)
			}
		}
	}

	env.svc.sendErr = &service.BroadcastError{Status: 422, Code: "NO_RECIPIENTS", Message: "Không có người nhận"}
	for i := 0; i < adminBroadcastMaxPerHour+2; i++ {
		if got := env.post(t, admin, broadcastSendPath); got != fiber.StatusUnprocessableEntity {
			t.Fatalf("NO_RECIPIENTS lần %d = %d, muốn 422", i+1, got)
		}
	}
	env.svc.sendErr = errors.New("db down before first chunk") // 500 thường, chưa giao ai
	for i := 0; i < adminBroadcastMaxPerHour+2; i++ {
		if got := env.post(t, admin, broadcastSendPath); got != fiber.StatusInternalServerError {
			t.Fatalf("500 chưa giao lần %d = %d, muốn 500", i+1, got)
		}
	}

	env.svc.sendErr = nil
	for i := 1; i <= adminBroadcastMaxPerHour; i++ {
		if got := env.post(t, admin, broadcastSendPath); got != fiber.StatusCreated {
			t.Fatalf("lần gửi thật %d = %d, muốn 201 (mọi lần thất bại trước đó không được tiêu hạn mức)", i, got)
		}
	}
	if got := env.post(t, admin, broadcastSendPath); got != fiber.StatusTooManyRequests {
		t.Fatalf("lần gửi thật thứ %d = %d, muốn 429", adminBroadcastMaxPerHour+1, got)
	}
}

// Gửi dở dang (đã giao >= 1 rồi mới lỗi) VẪN tiêu hạn mức: thông báo đã đi không thu hồi được.
func TestBroadcastRoutes_RateLimit_PartialDeliveryConsumesQuota(t *testing.T) {
	env := newBroadcastRouteEnv(t, "SYSTEM_SETTINGS_MANAGE")
	admin := env.tok()
	env.svc.sendErr = &service.BroadcastPartialError{Delivered: 500, Cause: errors.New("db down")}
	for i := 1; i <= adminBroadcastMaxPerHour; i++ {
		if got := env.post(t, admin, broadcastSendPath); got != fiber.StatusInternalServerError {
			t.Fatalf("gửi dở dang lần %d = %d, muốn 500 BROADCAST_PARTIAL", i, got)
		}
	}
	if got := env.post(t, admin, broadcastSendPath); got != fiber.StatusTooManyRequests {
		t.Fatalf("sau %d lần dở dang đã giao, lần tiếp = %d, muốn 429", adminBroadcastMaxPerHour, got)
	}
}

// M2 (review 261009): Idempotency-Key. Gửi lại cùng key của cùng admin trả kết quả gốc, KHÔNG gửi lại, KHÔNG tiêu hạn
// mức, KHÔNG ghi nhật ký lần hai; thiếu header = hành vi cũ.
func TestBroadcastRoutes_IdempotencyKey_ReplayDoesNotResendOrConsumeQuota(t *testing.T) {
	env := newBroadcastRouteEnv(t, "SYSTEM_SETTINGS_MANAGE")
	adminA, adminB := env.tok(), env.tok()

	first := env.postWith(t, adminA, broadcastSendPath, validBroadcastBody, "send-1")
	if first.status != fiber.StatusCreated || first.replay != "" {
		t.Fatalf("lần đầu = %+v, muốn 201 không có Idempotent-Replay", first)
	}
	for i := 0; i < adminBroadcastMaxPerHour+3; i++ { // nhiều hơn hạn mức: bản phát lại không bị 429
		again := env.postWith(t, adminA, broadcastSendPath, validBroadcastBody, "send-1")
		if again.status != fiber.StatusCreated || again.body != first.body || again.replay != "true" {
			t.Fatalf("lặp lần %d = %+v, muốn đúng kết quả gốc %+v kèm Idempotent-Replay: true", i+1, again, first)
		}
	}
	if env.svc.sends != 1 {
		t.Fatalf("service được gọi %d lần, muốn 1 (lặp lại không được gửi lại)", env.svc.sends)
	}
	if n := len(env.audit.take()); n != 1 {
		t.Fatalf("nhật ký có %d dòng, muốn 1 (bản phát lại không ghi lần hai)", n)
	}
	// Bản phát lại không tiêu hạn mức: còn đủ 4 lần gửi thật cho tổng 5.
	for i := 1; i < adminBroadcastMaxPerHour; i++ {
		if got := env.post(t, adminA, broadcastSendPath); got != fiber.StatusCreated {
			t.Fatalf("lần gửi thật %d sau các bản phát lại = %d, muốn 201", i, got)
		}
	}
	if got := env.post(t, adminA, broadcastSendPath); got != fiber.StatusTooManyRequests {
		t.Fatalf("lần gửi thật thứ %d = %d, muốn 429", adminBroadcastMaxPerHour+1, got)
	}
	// Cùng key nhưng admin khác là yêu cầu khác.
	if r := env.postWith(t, adminB, broadcastSendPath, validBroadcastBody, "send-1"); r.status != fiber.StatusCreated || r.replay != "" {
		t.Fatalf("admin khác cùng key = %+v, muốn chạy thật (201, không phải phát lại)", r)
	}
}

// Lần đầu thất bại không giữ key: sửa lỗi rồi gửi lại cùng key thì chạy thật.
func TestBroadcastRoutes_IdempotencyKey_FailedAttemptCanRetrySameKey(t *testing.T) {
	env := newBroadcastRouteEnv(t, "SYSTEM_SETTINGS_MANAGE")
	admin := env.tok()
	env.svc.sendErr = &service.BroadcastError{Status: 422, Code: "NO_RECIPIENTS", Message: "Không có người nhận"}
	if r := env.postWith(t, admin, broadcastSendPath, validBroadcastBody, "send-1"); r.status != fiber.StatusUnprocessableEntity {
		t.Fatalf("lần đầu = %+v, muốn 422", r)
	}
	env.svc.sendErr = nil
	if r := env.postWith(t, admin, broadcastSendPath, validBroadcastBody, "send-1"); r.status != fiber.StatusCreated || r.replay != "" {
		t.Fatalf("thử lại cùng key sau thất bại = %+v, muốn chạy thật (201)", r)
	}
	if env.svc.sends != 2 {
		t.Fatalf("service được gọi %d lần, muốn 2", env.svc.sends)
	}
}

// Lần gửi dở dang đã giao cho một phần người nhận: gửi lại cùng key phát lại kết quả 500 gốc, KHÔNG gửi lại cho cả danh sách.
func TestBroadcastRoutes_IdempotencyKey_PartialDeliveryIsReplayedNotResent(t *testing.T) {
	env := newBroadcastRouteEnv(t, "SYSTEM_SETTINGS_MANAGE")
	admin := env.tok()
	env.svc.sendErr = &service.BroadcastPartialError{Delivered: 500, Cause: errors.New("db down")}
	first := env.postWith(t, admin, broadcastSendPath, validBroadcastBody, "send-1")
	again := env.postWith(t, admin, broadcastSendPath, validBroadcastBody, "send-1")
	if first.status != fiber.StatusInternalServerError || again.status != fiber.StatusInternalServerError ||
		again.body != first.body || again.replay != "true" || !strings.Contains(again.body, "BROADCAST_PARTIAL") {
		t.Fatalf("lần đầu = %+v, lần lặp = %+v; muốn phát lại 500 BROADCAST_PARTIAL gốc", first, again)
	}
	if env.svc.sends != 1 {
		t.Fatalf("service được gọi %d lần, muốn 1", env.svc.sends)
	}
}
