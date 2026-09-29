package router

// Lane S5: hai route "nội bộ" trước đây chỉ cần đăng nhập.
//  - POST /notifications/send: gửi thông báo tới user_ids TUỲ Ý (giả mạo, spam).
//  - POST /achievements/:id/unlock: ai cũng tự mở khoá mọi thành tựu, không kiểm điều kiện.
// Nay chỉ admin (SYSTEM_SETTINGS_MANAGE). Dùng permChecker THẬT (cùng harness với category_report_authz_live_test.go):
// bỏ permChecker khỏi router, hoặc đổi tên permission, thì STUDENT nhận 200/201 và test ĐỎ.

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
	"study.com/v1/internal/service"
	"study.com/v1/internal/utils"
)

type s5NotificationSvc struct {
	service.NotificationServiceInterface
	called bool
}

func (s *s5NotificationSvc) SendNotification(dto.CreateNotificationDTO) error {
	s.called = true
	return nil
}

func (s *s5NotificationSvc) GetUnreadCount(uuid.UUID) (int64, error) { return 0, nil }

type s5AchievementSvc struct {
	service.AchievementServiceInterface
	called bool
}

func (s *s5AchievementSvc) UnlockAchievement(_ context.Context, userID, achievementID uuid.UUID) (*dto.UnlockAchievementResponse, error) {
	s.called = true
	return &dto.UnlockAchievementResponse{AchievementID: achievementID, UserID: userID}, nil
}

func newS5AdminRoutesEnv(t *testing.T, roleName string) (*fiber.App, string, *s5NotificationSvc, *s5AchievementSvc) {
	t.Helper()
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = rdb.Close() })
	cfg := &config.Config{JWTSecret: "s5-admin-routes-test-secret", JWTAccessExpiration: 15 * time.Minute, JWTRefreshExpiration: 7 * 24 * time.Hour}

	userID, deviceID, sysRoleID := uuid.New(), uuid.New(), uuid.New()
	const userVer = int64(1)
	if err := rdb.Set(context.Background(), constants.KeyUserVersion(userID.String()), userVer, 0).Err(); err != nil {
		t.Fatal(err)
	}
	perms := map[uuid.UUID][]string{sysRoleID: {}}
	if roleName == "SYSTEM_ADMIN" {
		perms[sysRoleID] = []string{"*"}
	}
	permChecker := middleware.NewPermissionChecker(&fakeUSRRepoCR{systemRoleID: sysRoleID}, &fakeSRRepoCR{perms: perms}, nil, nil)
	token, _, err := utils.GenerateTokens(cfg, userID, deviceID, roleName, nil, userVer)
	if err != nil {
		t.Fatal(err)
	}

	app := fiber.New()
	api := app.Group("/api")
	notif, ach := &s5NotificationSvc{}, &s5AchievementSvc{}
	SetupNotificationRoutes(api, cfg, handler.NewNotificationHandler(notif), rdb, permChecker)
	SetupAchievementRoutes(api, cfg, handler.NewAchievementHandler(ach), rdb, permChecker)
	return app, token, notif, ach
}

func s5Do(t *testing.T, app *fiber.App, token, method, path, body string) int {
	t.Helper()
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	_, _ = io.ReadAll(resp.Body)
	return resp.StatusCode
}

const s5NotifBody = `{"title":"t","content":"c","notification_type":"system","user_ids":["` + "00000000-0000-4000-8000-000000000001" + `"]}`

func TestS5_NotificationSendVaAchievementUnlock_HocVienBiTuChoi403(t *testing.T) {
	app, token, notif, ach := newS5AdminRoutesEnv(t, "STUDENT")
	if got := s5Do(t, app, token, "POST", "/api/notifications/send", s5NotifBody); got != fiber.StatusForbidden {
		t.Errorf("POST /notifications/send (STUDENT): %d, muốn 403", got)
	}
	if got := s5Do(t, app, token, "POST", "/api/achievements/"+uuid.NewString()+"/unlock", `{}`); got != fiber.StatusForbidden {
		t.Errorf("POST /achievements/:id/unlock (STUDENT): %d, muốn 403", got)
	}
	if notif.called || ach.called {
		t.Error("service bị gọi dù permChecker phải chặn trước handler")
	}
}

func TestS5_NotificationSendVaAchievementUnlock_AdminDuocPhep(t *testing.T) {
	app, token, notif, ach := newS5AdminRoutesEnv(t, "SYSTEM_ADMIN")
	if got := s5Do(t, app, token, "POST", "/api/notifications/send", s5NotifBody); got != fiber.StatusCreated {
		t.Errorf("POST /notifications/send (admin): %d, muốn 201", got)
	}
	if got := s5Do(t, app, token, "POST", "/api/achievements/"+uuid.NewString()+"/unlock", `{}`); got != fiber.StatusCreated {
		t.Errorf("POST /achievements/:id/unlock (admin): %d, muốn 201", got)
	}
	if !notif.called || !ach.called {
		t.Error("service không được gọi dù admin đã qua permChecker")
	}
}

// Các route còn lại của nhóm giữ nguyên: học viên vẫn đọc thông báo và thành tựu của mình.
func TestS5_NotificationVaAchievement_RouteDocCuaHocVienConMo(t *testing.T) {
	app, token, _, _ := newS5AdminRoutesEnv(t, "STUDENT")
	for _, p := range []string{"/api/notifications/unread-count"} {
		if got := s5Do(t, app, token, "GET", p, ""); got == fiber.StatusForbidden || got == fiber.StatusUnauthorized {
			t.Errorf("GET %s bị chặn nhầm: %d", p, got)
		}
	}
}