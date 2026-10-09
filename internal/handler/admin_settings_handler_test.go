package handler_test

// Phase 5 (contract C4): GET /api/admin/settings. Route THẬT (SetupAdminSettingsRoutes) →
// AuthMiddleware THẬT (miniredis) → PermissionChecker THẬT (repo quyền giả) → AdminSettingsHandler
// THẬT → PlatformSettingService + Repository THẬT trên schema Postgres tạm (pgtest.IsolatedSchema:
// mỗi test một schema riêng nên chạy song song với các lane khác không đụng nhau).
//
// Là package ngoài (handler_test) vì cần import router; router import handler nên package
// handler nội bộ không import ngược lại được.

import (
	"context"
	"encoding/json"
	"io"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"github.com/shopspring/decimal"
	"study.com/v1/internal/config"
	"study.com/v1/internal/constants"
	"study.com/v1/internal/database"
	"study.com/v1/internal/dto"
	"study.com/v1/internal/handler"
	"study.com/v1/internal/middleware"
	"study.com/v1/internal/model"
	"study.com/v1/internal/repository"
	"study.com/v1/internal/router"
	"study.com/v1/internal/service"
	"study.com/v1/internal/testutil/pgtest"
	"study.com/v1/internal/utils"
)

type asFakeSystemRoleRepo struct {
	repository.SystemRoleRepositoryInterface
	perms map[uuid.UUID][]string
}

func (f *asFakeSystemRoleRepo) GetPermissionsBySystemRoleID(_ context.Context, roleID uuid.UUID) ([]model.Permission, error) {
	var out []model.Permission
	for _, n := range f.perms[roleID] {
		out = append(out, model.Permission{Name: n})
	}
	return out, nil
}

type asFakeUserSystemRoleRepo struct {
	repository.UserSystemRoleRepositoryInterface
	byUser map[uuid.UUID][]model.UserSystemRole
}

func (f *asFakeUserSystemRoleRepo) FindByUserID(_ context.Context, userID uuid.UUID, _ string) ([]model.UserSystemRole, error) {
	return f.byUser[userID], nil
}

// asSpyService đếm số lần GetSettings được gọi — chứng minh người không có quyền bị chặn TRƯỚC handler.
type asSpyService struct {
	service.PlatformSettingServiceInterface
	getCalls int
}

func (s *asSpyService) GetSettings(_ context.Context) (*dto.AdminSettingsDTO, error) {
	s.getCalls++
	return &dto.AdminSettingsDTO{}, nil
}

type asEnv struct {
	app                  *fiber.App
	adminTok, teacherTok string
}

func newAdminSettingsEnv(t *testing.T, svc service.PlatformSettingServiceInterface) *asEnv {
	t.Helper()
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = rdb.Close() })
	cfg := &config.Config{JWTSecret: "admin-settings-route-test-secret", JWTAccessExpiration: 15 * time.Minute, JWTRefreshExpiration: time.Hour}

	adminID, teacherID := uuid.New(), uuid.New()
	adminRole, teacherRole := uuid.New(), uuid.New()
	sysRoles := &asFakeSystemRoleRepo{perms: map[uuid.UUID][]string{
		adminRole:   {"SYSTEM_SETTINGS_MANAGE"},
		teacherRole: {"COURSES_CREATE", "COURSES_UPDATE_OWN", "LESSONS_MANAGE"}, // quyền thật của TEACHER
	}}
	userRoles := &asFakeUserSystemRoleRepo{byUser: map[uuid.UUID][]model.UserSystemRole{
		adminID:   {{UserID: adminID, SystemRoleID: adminRole, Status: model.UserSystemRoleStatusActive}},
		teacherID: {{UserID: teacherID, SystemRoleID: teacherRole, Status: model.UserSystemRoleStatusActive}},
	}}
	pc := middleware.NewPermissionChecker(userRoles, sysRoles, nil, nil)

	deviceID := uuid.New()
	tok := func(id uuid.UUID, role string) string {
		if err := rdb.Set(context.Background(), constants.KeyUserVersion(id.String()), int64(1), 0).Err(); err != nil {
			t.Fatalf("seed user_version: %v", err)
		}
		s, _, err := utils.GenerateTokens(cfg, id, deviceID, role, nil, 1)
		if err != nil {
			t.Fatalf("GenerateTokens: %v", err)
		}
		return s
	}

	app := fiber.New()
	router.SetupAdminSettingsRoutes(app.Group("/api"), cfg, handler.NewAdminSettingsHandler(svc), rdb, pc)
	return &asEnv{app: app, adminTok: tok(adminID, "SYSTEM_ADMIN"), teacherTok: tok(teacherID, "TEACHER")}
}

func (e *asEnv) get(t *testing.T, token string) (int, map[string]interface{}) {
	t.Helper()
	req := httptest.NewRequest("GET", "/api/admin/settings", nil)
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := e.app.Test(req, -1)
	if err != nil {
		t.Fatalf("GET /api/admin/settings: %v", err)
	}
	raw, _ := io.ReadAll(resp.Body)
	var out map[string]interface{}
	_ = json.Unmarshal(raw, &out)
	return resp.StatusCode, out
}

func newRealSettingsService(t *testing.T) (service.PlatformSettingServiceInterface, func(name string, fullName *string) uuid.UUID) {
	t.Helper()
	db := pgtest.IsolatedSchema(t, database.Migrate)
	mkUser := func(userName string, fullName *string) uuid.UUID {
		s := uuid.NewString()
		u := model.User{Email: "settings-" + s + "@40study.test", PasswordHash: "x", UserName: userName, FullName: fullName}
		if err := db.Create(&u).Error; err != nil {
			t.Fatalf("tạo user: %v", err)
		}
		return u.ID
	}
	return service.NewPlatformSettingService(repository.NewPlatformSettingRepository(db)), mkUser
}

func dataOf(t *testing.T, body map[string]interface{}) map[string]interface{} {
	t.Helper()
	data, ok := body["data"].(map[string]interface{})
	if !ok {
		t.Fatalf("thiếu data trong body: %v", body)
	}
	return data
}

// Bảng platform_settings rỗng -> phí 0, updated_at/updated_by là null NHƯNG khoá vẫn có mặt
// (contract C4: không omitempty).
func TestAdminSettings_EmptyTableReturnsDefaults(t *testing.T) {
	svc, _ := newRealSettingsService(t)
	e := newAdminSettingsEnv(t, svc)

	code, body := e.get(t, e.adminTok)
	if code != fiber.StatusOK {
		t.Fatalf("status = %d %v, muốn 200", code, body)
	}
	data := dataOf(t, body)
	if fee, ok := data["platform_fee_percent"]; !ok || fee != "0" {
		t.Fatalf("platform_fee_percent = %#v, muốn \"0\" (cùng mã hoá GET platform-fee)", fee)
	}
	for _, k := range []string{"updated_at", "updated_by"} {
		v, present := data[k]
		if !present || v != nil {
			t.Fatalf("%s = %#v (present=%v), muốn null có mặt", k, v, present)
		}
	}
}

// Sau SetPlatformFeePercent: phí mới + thời điểm + TÊN người cập nhật (full_name; không có thì user_name).
func TestAdminSettings_AfterSetReturnsValuesAndUpdaterName(t *testing.T) {
	svc, mkUser := newRealSettingsService(t)
	e := newAdminSettingsEnv(t, svc)
	fullName := "Quản trị viên Lan"
	cases := []struct {
		label    string
		userName string
		fullName *string
		want     string
	}{
		{"có full_name", "lan_admin", &fullName, fullName},
		{"không full_name -> user_name", "root_admin", nil, "root_admin"},
	}
	for i, tc := range cases {
		actor := mkUser(tc.userName, tc.fullName)
		fee := decimal.NewFromFloat(12.5 + float64(i))
		before := time.Now().Add(-2 * time.Second)
		if err := svc.SetPlatformFeePercent(context.Background(), actor, fee); err != nil {
			t.Fatalf("%s: set: %v", tc.label, err)
		}

		code, body := e.get(t, e.adminTok)
		if code != fiber.StatusOK {
			t.Fatalf("%s: status = %d %v", tc.label, code, body)
		}
		raw, _ := json.Marshal(body["data"])
		var got dto.AdminSettingsDTO
		if err := json.Unmarshal(raw, &got); err != nil {
			t.Fatalf("%s: giải mã data: %v", tc.label, err)
		}
		if !got.PlatformFeePercent.Equal(fee) {
			t.Fatalf("%s: phí = %s, muốn %s", tc.label, got.PlatformFeePercent, fee)
		}
		if got.UpdatedAt == nil || got.UpdatedAt.Before(before) {
			t.Fatalf("%s: updated_at = %v, muốn gần đây", tc.label, got.UpdatedAt)
		}
		if got.UpdatedBy == nil || got.UpdatedBy.ID != actor || got.UpdatedBy.Name != tc.want {
			t.Fatalf("%s: updated_by = %+v, muốn {%s %q}", tc.label, got.UpdatedBy, actor, tc.want)
		}
	}
}

// Người không có SYSTEM_SETTINGS_MANAGE: 403 và handler/service KHÔNG được gọi; không token: 401.
func TestAdminSettings_NonAdminForbidden(t *testing.T) {
	spy := &asSpyService{}
	e := newAdminSettingsEnv(t, spy)

	if code, body := e.get(t, e.teacherTok); code != fiber.StatusForbidden {
		t.Fatalf("giáo viên = %d %v, muốn 403", code, body)
	}
	if code, _ := e.get(t, ""); code != fiber.StatusUnauthorized {
		t.Fatalf("không token = %d, muốn 401", code)
	}
	if spy.getCalls != 0 {
		t.Fatalf("service bị gọi %d lần dù bị chặn quyền", spy.getCalls)
	}
	if code, _ := e.get(t, e.adminTok); code != fiber.StatusOK || spy.getCalls != 1 {
		t.Fatalf("admin = %d, getCalls=%d, muốn 200 và 1", code, spy.getCalls)
	}
}
