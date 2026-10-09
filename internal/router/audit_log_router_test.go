package router

// Route nhật ký quản trị (contract C2, plan D4): Fiber app riêng, AuthMiddleware THẬT (miniredis),
// PermissionChecker THẬT với repo giả, service giả. Chỉ SYSTEM_SETTINGS_MANAGE được xem.

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
	"study.com/v1/internal/config"
	"study.com/v1/internal/constants"
	"study.com/v1/internal/dto"
	"study.com/v1/internal/handler"
	"study.com/v1/internal/middleware"
	"study.com/v1/internal/model"
	"study.com/v1/internal/repository"
	"study.com/v1/internal/service"
	"study.com/v1/internal/utils"
)

type auditRouteUserRoles struct {
	repository.UserSystemRoleRepositoryInterface
	roleByUser map[uuid.UUID]uuid.UUID
}

func (f *auditRouteUserRoles) FindByUserID(_ context.Context, id uuid.UUID, _ string) ([]model.UserSystemRole, error) {
	if r, ok := f.roleByUser[id]; ok {
		return []model.UserSystemRole{{SystemRoleID: r}}, nil
	}
	return nil, nil
}

type auditRouteRoles struct {
	repository.SystemRoleRepositoryInterface
	perms map[uuid.UUID][]string
}

func (f *auditRouteRoles) GetPermissionsBySystemRoleID(_ context.Context, id uuid.UUID) ([]model.Permission, error) {
	var out []model.Permission
	for _, n := range f.perms[id] {
		out = append(out, model.Permission{Name: n})
	}
	return out, nil
}

type auditRouteService struct {
	service.AuditLogServiceInterface
	lastFilter dto.AuditLogFilterDTO
}

func (f *auditRouteService) List(_ context.Context, q dto.AuditLogFilterDTO) (dto.AuditLogListDTO, error) {
	f.lastFilter = q
	if q.ActorID == "bad" {
		return dto.AuditLogListDTO{}, &service.InvalidAuditFilterError{Field: "actor_id"}
	}
	return dto.NewAuditLogListDTO(nil, 0, 1, 20), nil
}

func (f *auditRouteService) Actions() []string { return []string{"user.lock"} }

func TestAuditRoutes_PermissionGateAndResponses(t *testing.T) {
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = rdb.Close() })
	cfg := &config.Config{JWTSecret: "audit-route-test-secret", JWTAccessExpiration: 15 * time.Minute, JWTRefreshExpiration: time.Hour}

	adminRole, studentRole := uuid.New(), uuid.New()
	admin, student := uuid.New(), uuid.New()
	pc := middleware.NewPermissionChecker(
		&auditRouteUserRoles{roleByUser: map[uuid.UUID]uuid.UUID{admin: adminRole, student: studentRole}},
		&auditRouteRoles{perms: map[uuid.UUID][]string{adminRole: {"SYSTEM_SETTINGS_MANAGE"}, studentRole: {"COURSE_VIEW", "PAYMENTS_MANAGE"}}},
		nil, nil)
	token := func(id uuid.UUID) string {
		if err := rdb.Set(context.Background(), constants.KeyUserVersion(id.String()), int64(1), 0).Err(); err != nil {
			t.Fatal(err)
		}
		s, _, err := utils.GenerateTokens(cfg, id, uuid.New(), "ADMIN", nil, 1)
		if err != nil {
			t.Fatal(err)
		}
		return s
	}

	svc := &auditRouteService{}
	app := fiber.New()
	SetupAuditLogRoutes(app.Group("/api"), cfg, handler.NewAuditLogHandler(svc), rdb, pc)
	do := func(path, tok string) (int, map[string]any) {
		req := httptest.NewRequest("GET", path, nil)
		if tok != "" {
			req.Header.Set("Authorization", "Bearer "+tok)
		}
		resp, err := app.Test(req, -1)
		if err != nil {
			t.Fatal(err)
		}
		raw, _ := io.ReadAll(resp.Body)
		var body map[string]any
		_ = json.Unmarshal(raw, &body)
		return resp.StatusCode, body
	}

	for _, path := range []string{"/api/admin/audit-logs", "/api/admin/audit-logs/actions"} {
		if code, _ := do(path, ""); code != fiber.StatusUnauthorized {
			t.Errorf("%s không token = %d, muốn 401", path, code)
		}
		if code, _ := do(path, token(student)); code != fiber.StatusForbidden {
			t.Errorf("%s thiếu SYSTEM_SETTINGS_MANAGE = %d, muốn 403", path, code)
		}
	}

	code, body := do("/api/admin/audit-logs?page=2&action=user.lock", token(admin))
	data, _ := body["data"].(map[string]any)
	if code != 200 || body["message"] != "success" || data["page_size"] != float64(20) || svc.lastFilter.Page != 2 || svc.lastFilter.Action != "user.lock" {
		t.Errorf("list = %d %v filter=%+v", code, body, svc.lastFilter)
	}
	if items, ok := data["items"].([]any); !ok || len(items) != 0 {
		t.Errorf("items phải là [] chứ không null: %v", data["items"])
	}

	code, body = do("/api/admin/audit-logs?actor_id=bad", token(admin))
	if code != 400 || body["code"] != "INVALID_FILTER" || body["message"] == "" {
		t.Errorf("filter sai = %d %v, muốn 400 INVALID_FILTER", code, body)
	}

	code, body = do("/api/admin/audit-logs/actions", token(admin))
	if acts, _ := body["data"].([]any); code != 200 || len(acts) != 1 || acts[0] != "user.lock" {
		t.Errorf("actions = %d %v", code, body)
	}
}
