package router

// QA2 ADM-L10: GET /system-roles và GET /permissions chỉ cần đăng nhập nên giảng viên đọc được toàn bộ danh mục
// vai trò (kể cả ORG_OWNER) và 28 permission. Mọi nơi gọi trong web (web/src) là trang quản trị: /admin (dashboard),
// /admin/roles, /admin/permissions, /admin/users[/:id] (ô lọc/gán vai trò), /admin/notifications (chọn đối tượng).
// Nay đọc danh mục cần MỘT trong ROLES_MANAGE_SYSTEM, USERS_VIEW_ALL, SYSTEM_SETTINGS_MANAGE (đúng các quyền gác
// những trang đó), nên admin có vai trò tuỳ biến chỉ giữ USERS_VIEW_ALL vẫn mở được /admin/users.
// permChecker THẬT (cùng harness với s5_admin_routes_authz_live_test.go): bỏ RequireAnyPermission khỏi route thì
// giảng viên nhận 200 và test ĐỎ.

import (
	"context"
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

type catalogueSystemRoleSvc struct {
	service.SystemRoleServiceInterface
	listed bool
}

func (s *catalogueSystemRoleSvc) GetAllSystemRoles(context.Context, int, int, string, string) (*dto.SystemRoleListResponseDTO, error) {
	s.listed = true
	return &dto.SystemRoleListResponseDTO{}, nil
}

type cataloguePermissionSvc struct {
	service.PermissionServiceInterface
	listed bool
}

func (s *cataloguePermissionSvc) GetAllPermissions(context.Context, int, int, string, string) (*dto.PermissionListResponseDTO, error) {
	s.listed = true
	return &dto.PermissionListResponseDTO{}, nil
}

func (s *cataloguePermissionSvc) GetPermissionByID(context.Context, uuid.UUID) (*dto.PermissionResponseDTO, error) {
	s.listed = true
	return &dto.PermissionResponseDTO{}, nil
}

// newCatalogueEnv: người gọi giữ đúng các permission trong granted (nil = không có gì, như giảng viên).
func newCatalogueEnv(t *testing.T, granted []string) (*fiber.App, string, *catalogueSystemRoleSvc, *cataloguePermissionSvc) {
	t.Helper()
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = rdb.Close() })
	cfg := &config.Config{JWTSecret: "catalogue-authz-test-secret", JWTAccessExpiration: 15 * time.Minute, JWTRefreshExpiration: 7 * 24 * time.Hour}

	userID, deviceID, sysRoleID := uuid.New(), uuid.New(), uuid.New()
	const userVer = int64(1)
	if err := rdb.Set(context.Background(), constants.KeyUserVersion(userID.String()), userVer, 0).Err(); err != nil {
		t.Fatal(err)
	}
	permChecker := middleware.NewPermissionChecker(&fakeUSRRepoCR{systemRoleID: sysRoleID}, &fakeSRRepoCR{perms: map[uuid.UUID][]string{sysRoleID: granted}}, nil, nil)
	token, _, err := utils.GenerateTokens(cfg, userID, deviceID, "TEACHER", nil, userVer)
	if err != nil {
		t.Fatal(err)
	}

	app := fiber.New()
	api := app.Group("/api")
	roles, perms := &catalogueSystemRoleSvc{}, &cataloguePermissionSvc{}
	SetupSystemRoleRoutes(api, cfg, handler.NewSystemRoleHandler(roles), rdb, permChecker, nil)
	SetupPermissionRoutes(api, cfg, handler.NewPermissionHandler(perms), rdb, permChecker, nil)
	return app, token, roles, perms
}

func TestCatalogue_TeacherWithoutAdminPermissionIsForbidden(t *testing.T) {
	app, token, roles, perms := newCatalogueEnv(t, []string{"COURSES_MANAGE_OWN"}) // quyền giảng viên, không có quyền quản trị nào
	for _, p := range []string{"/api/system-roles", "/api/permissions?page_size=100", "/api/permissions/" + uuid.NewString()} {
		if got := s5Do(t, app, token, "GET", p, ""); got != fiber.StatusForbidden {
			t.Errorf("GET %s (giảng viên): %d, muốn 403", p, got)
		}
	}
	if roles.listed || perms.listed {
		t.Error("service bị gọi dù permChecker phải chặn trước handler")
	}
}

func TestCatalogue_AdminSurfacesStillReadable(t *testing.T) {
	cases := map[string][]string{
		"SYSTEM_ADMIN (wildcard)":    {"*"},
		"chỉ ROLES_MANAGE_SYSTEM":    {"ROLES_MANAGE_SYSTEM"},
		"chỉ USERS_VIEW_ALL":         {"USERS_VIEW_ALL"},
		"chỉ SYSTEM_SETTINGS_MANAGE": {"SYSTEM_SETTINGS_MANAGE"},
		"quyền admin lẫn quyền khác": {"COURSES_MANAGE_OWN", "USERS_VIEW_ALL"},
	}
	for name, granted := range cases {
		app, token, roles, perms := newCatalogueEnv(t, granted)
		for _, p := range []string{"/api/system-roles", "/api/permissions?page_size=100", "/api/permissions/" + uuid.NewString()} {
			if got := s5Do(t, app, token, "GET", p, ""); got != fiber.StatusOK {
				t.Errorf("%s: GET %s = %d, muốn 200 (trang quản trị dùng endpoint này)", name, p, got)
			}
		}
		if !roles.listed || !perms.listed {
			t.Errorf("%s: service không được gọi", name)
		}
	}
}

func TestCatalogue_NoTokenIsStill401(t *testing.T) {
	app, _, _, _ := newCatalogueEnv(t, []string{"*"})
	for _, p := range []string{"/api/system-roles", "/api/permissions"} {
		if got := s5Do(t, app, "", "GET", p, ""); got != fiber.StatusUnauthorized {
			t.Errorf("GET %s không token: %d, muốn 401", p, got)
		}
	}
}

// RequireAnyPermission() không có đối số thất bại ĐÓNG (không ai qua): một route cấu hình thiếu danh sách quyền
// không được lặng lẽ thành route công khai.
func TestRequireAnyPermission_NoArgumentsFailsClosed(t *testing.T) {
	sysRoleID := uuid.New()
	pc := middleware.NewPermissionChecker(&fakeUSRRepoCR{systemRoleID: sysRoleID}, &fakeSRRepoCR{perms: map[uuid.UUID][]string{sysRoleID: {"COURSES_MANAGE_OWN"}}}, nil, nil)
	app := fiber.New()
	app.Get("/x", func(c *fiber.Ctx) error { c.Locals("user_id", uuid.New()); return c.Next() }, pc.RequireAnyPermission(), func(c *fiber.Ctx) error { return c.SendStatus(200) })
	if got := s5Do(t, app, "", "GET", "/x", ""); got != fiber.StatusForbidden {
		t.Errorf("RequireAnyPermission() không đối số: %d, muốn 403", got)
	}
}
