package router

// Phase 8 (plan 261008, risk "audit hook missing or doubled on a route"): TestAuditInstrumentation đi qua
// MỌI route quản trị được ghi nhật ký trong bảng của plan.
//
// Dựng router THẬT: SetupAllRoutes (gọi bằng reflection để chữ ký đổi không làm vỡ test) cộng hai Setup*
// mà app.go đăng ký riêng (withdrawal, broadcast), với AuthMiddleware THẬT (miniredis) + permChecker THẬT
// + recorder giả. Chỉ MỘT thứ bị thay: handler cuối của route (bằng stub trả status do header chỉ định),
// vì nghiệp vụ của handler không phải đối tượng test này. Mọi middleware trước đó, gồm cả middleware.Audit
// gắn trong router, chạy nguyên bản.
//
//   - 2xx  -> đúng 1 dòng với đúng action/target_type/target_id/actor.
//   - 4xx  -> 0 dòng.
//   - không token (401) -> 0 dòng.
// Bảng phải phủ toàn bộ model.AuditActions: thêm action mới mà quên route thì test đỏ.

import (
	"context"
	"net/http/httptest"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"study.com/v1/internal/config"
	"study.com/v1/internal/constants"
	"study.com/v1/internal/middleware"
	"study.com/v1/internal/model"
	"study.com/v1/internal/utils"
)

// auditSpyRouter ghi lại mọi entry; dùng chung cho các test router cần một AuditRecorder.
type auditSpyRouter struct {
	mu      sync.Mutex
	entries []model.AuditEntry
}

func (s *auditSpyRouter) Record(_ context.Context, e model.AuditEntry) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.entries = append(s.entries, e)
	return nil
}

func (s *auditSpyRouter) take() []model.AuditEntry {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := s.entries
	s.entries = nil
	return out
}

type auditRoute struct {
	method, pattern, action, targetType string
	// targetParam: tên URL param chứa id đích ("" = route không có đích trong URL).
	targetParam string
}

var auditRoutes = []auditRoute{
	{"PUT", "/api/users/:id/status", model.AuditActionUserLock, "user", "id"},
	{"POST", "/api/users/:user_id/system-roles", model.AuditActionUserRoleAssign, "user", "user_id"},
	{"DELETE", "/api/users/:user_id/system-roles/:system_role_id", model.AuditActionUserRoleRevoke, "user", "user_id"},
	{"POST", "/api/system-roles/", model.AuditActionSystemRoleCreate, "system_role", ""},
	{"PUT", "/api/system-roles/:id", model.AuditActionSystemRoleUpdate, "system_role", "id"},
	{"DELETE", "/api/system-roles/:id", model.AuditActionSystemRoleDelete, "system_role", "id"},
	{"PATCH", "/api/system-roles/:id/restore", model.AuditActionSystemRoleRestore, "system_role", "id"},
	{"POST", "/api/system-roles/:id/permissions", model.AuditActionSystemRolePermissions, "system_role", "id"},
	{"PUT", "/api/system-roles/:id/permissions", model.AuditActionSystemRolePermissions, "system_role", "id"},
	{"DELETE", "/api/system-roles/:id/permissions", model.AuditActionSystemRolePermissions, "system_role", "id"},
	{"PUT", "/api/permissions/:id", model.AuditActionPermissionUpdate, "permission", "id"},
	{"POST", "/api/admin/courses/:id/approve", model.AuditActionCourseApprove, "course", "id"},
	{"POST", "/api/admin/courses/:id/reject", model.AuditActionCourseReject, "course", "id"},
	{"POST", "/api/admin/teacher-applications/:userId/approve", model.AuditActionTeacherApplicationApprove, "user", "userId"},
	{"POST", "/api/admin/teacher-applications/:userId/reject", model.AuditActionTeacherApplicationReject, "user", "userId"},
	{"PUT", "/api/reports/:id/status", model.AuditActionReportStatusUpdate, "report", "id"},
	{"DELETE", "/api/reports/:id", model.AuditActionReportDelete, "report", "id"},
	{"POST", "/api/orders/admin/:id/refund", model.AuditActionOrderRefund, "order", "id"},
	{"POST", "/api/orders/admin/:id/late-refund", model.AuditActionOrderLateRefund, "order", "id"},
	{"POST", "/api/admin/withdrawals/:id/approve", model.AuditActionWithdrawalApprove, "withdrawal", "id"},
	{"POST", "/api/admin/withdrawals/:id/reject", model.AuditActionWithdrawalReject, "withdrawal", "id"},
	{"POST", "/api/admin/withdrawals/:id/mark-completed", model.AuditActionWithdrawalMarkCompleted, "withdrawal", "id"},
	{"PUT", "/api/admin/settings/platform-fee", model.AuditActionSettingPlatformFeeUpdate, "setting", ""},
	{"POST", "/api/admin/notifications/broadcast", model.AuditActionNotificationBroadcast, "", ""},
}

// auditRoutesNotInTable: action do HANDLER đặt (SetAuditAction), cùng route với một dòng bảng.
// user.unlock đi chung PUT /users/:id/status; được handler test phủ (TestUserAdminHandler_UnlockSetsAuditAction).
var auditActionsSetByHandler = map[string]bool{model.AuditActionUserUnlock: true}

const stubStatusHeader = "X-Audit-Stub-Status"

// every permission the instrumented routes demand (router files are the SSOT; this is a test fixture).
var auditRoutePerms = []string{
	"ROLES_MANAGE_SYSTEM", "USERS_BAN", "COURSES_APPROVE_ALL", "REPORTS_MODERATE",
	"PAYMENTS_MANAGE", "WALLET_WITHDRAWALS_MANAGE", "SYSTEM_SETTINGS_MANAGE",
}

type auditEnv struct {
	app *fiber.App
	rec *auditSpyRouter
	tok func() (string, uuid.UUID)
}

func newAuditEnv(t *testing.T) *auditEnv {
	t.Helper()
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = rdb.Close() })
	cfg := &config.Config{JWTSecret: "audit-instrumentation-test-secret", JWTAccessExpiration: 15 * time.Minute, JWTRefreshExpiration: time.Hour}
	roleID := uuid.New()
	pc := middleware.NewPermissionChecker(&fakeUSRRepoCR{systemRoleID: roleID}, &fakeSRRepoCR{perms: map[uuid.UUID][]string{roleID: auditRoutePerms}}, nil, nil)
	rec := &auditSpyRouter{}

	app := fiber.New()
	// SetupAllRoutes: mọi handler nil (method value trên con trỏ nil vẫn đăng ký được), redis/permChecker/recorder thật.
	fn := reflect.ValueOf(SetupAllRoutes)
	args := make([]reflect.Value, fn.Type().NumIn())
	for i := range args {
		args[i] = reflect.Zero(fn.Type().In(i))
		switch fn.Type().In(i) {
		case reflect.TypeOf(app):
			args[i] = reflect.ValueOf(app)
		case reflect.TypeOf(cfg):
			args[i] = reflect.ValueOf(cfg)
		case reflect.TypeOf(pc):
			args[i] = reflect.ValueOf(pc)
		case reflect.TypeOf(rdb):
			args[i] = reflect.ValueOf(rdb)
		case reflect.TypeOf((*middleware.AuditRecorder)(nil)).Elem():
			args[i] = reflect.ValueOf(rec)
		}
	}
	fn.Call(args)
	// Hai nhóm route app.go đăng ký ngoài SetupAllRoutes và nằm trong bảng audit.
	SetupWithdrawalRoutes(app.Group("/api"), cfg, nil, rdb, pc, rec)
	SetupAdminBroadcastRoutes(app.Group("/api"), cfg, nil, rdb, pc, rec)

	stub := func(c *fiber.Ctx) error {
		status, err := strconv.Atoi(c.Get(stubStatusHeader))
		if err != nil {
			status = fiber.StatusOK
		}
		return c.Status(status).SendString("stub")
	}
	for _, rt := range auditRoutes {
		var matches []*fiber.Route
		for _, r := range app.Stack()[methodIndex(rt.method)] {
			if r.Path == rt.pattern {
				matches = append(matches, r)
			}
		}
		if len(matches) != 1 {
			t.Fatalf("%s %s: %d route đã đăng ký, muốn đúng 1 (router đổi đường dẫn? cập nhật bảng)", rt.method, rt.pattern, len(matches))
		}
		h := matches[0].Handlers
		h[len(h)-1] = stub
	}

	tok := func() (string, uuid.UUID) {
		id := uuid.New()
		if err := rdb.Set(context.Background(), constants.KeyUserVersion(id.String()), int64(1), 0).Err(); err != nil {
			t.Fatal(err)
		}
		s, _, err := utils.GenerateTokens(cfg, id, uuid.New(), "SYSTEM_ADMIN", nil, 1)
		if err != nil {
			t.Fatal(err)
		}
		return s, id
	}
	return &auditEnv{app: app, rec: rec, tok: tok}
}

func methodIndex(m string) int {
	for i, v := range fiber.DefaultMethods {
		if v == m {
			return i
		}
	}
	panic("unknown method " + m)
}

// concrete thay :param bằng id thật và trả về map param -> giá trị.
func concrete(pattern string) (string, map[string]string) {
	params := map[string]string{}
	parts := strings.Split(pattern, "/")
	for i, p := range parts {
		if strings.HasPrefix(p, ":") {
			v := uuid.NewString()
			params[p[1:]] = v
			parts[i] = v
		}
	}
	return strings.Join(parts, "/"), params
}

func (e *auditEnv) call(t *testing.T, rt auditRoute, token string, status int) (int, string, map[string]string) {
	t.Helper()
	path, params := concrete(rt.pattern)
	req := httptest.NewRequest(rt.method, path, strings.NewReader("{}"))
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	req.Header.Set(stubStatusHeader, strconv.Itoa(status))
	resp, err := e.app.Test(req, -1)
	if err != nil {
		t.Fatalf("%s %s: %v", rt.method, path, err)
	}
	_ = resp.Body.Close()
	return resp.StatusCode, path, params
}

func TestAuditInstrumentation(t *testing.T) {
	env := newAuditEnv(t)

	for _, rt := range auditRoutes {
		t.Run(rt.method+" "+rt.pattern, func(t *testing.T) {
			token, actor := env.tok()
			got, path, params := env.call(t, rt, token, fiber.StatusOK)
			if got != fiber.StatusOK {
				t.Fatalf("2xx: status = %d, muốn 200 (chuỗi middleware chặn trước handler?)", got)
			}
			entries := env.rec.take()
			if len(entries) != 1 {
				t.Fatalf("2xx %s: %d dòng nhật ký, muốn đúng 1", path, len(entries))
			}
			e := entries[0]
			wantTarget := ""
			if rt.targetParam != "" {
				wantTarget = params[rt.targetParam]
			}
			if e.Action != rt.action || e.TargetType != rt.targetType || e.TargetID != wantTarget || e.ActorID != actor || e.StatusCode != 200 {
				t.Errorf("entry = %+v, muốn action=%q target=%s/%q actor=%s status=200", e, rt.action, rt.targetType, wantTarget, actor)
			}

			for _, bad := range []int{400, 403, 404, 409, 422} {
				token, _ := env.tok()
				if got, _, _ := env.call(t, rt, token, bad); got != bad {
					t.Fatalf("stub %d: status = %d", bad, got)
				}
				if n := len(env.rec.take()); n != 0 {
					t.Errorf("status %d phải ghi 0 dòng, có %d", bad, n)
				}
			}

			if got, _, _ := env.call(t, rt, "", fiber.StatusOK); got != fiber.StatusUnauthorized {
				t.Errorf("không token: status = %d, muốn 401", got)
			}
			if n := len(env.rec.take()); n != 0 {
				t.Errorf("401 phải ghi 0 dòng, có %d", n)
			}
		})
	}
}

// Bảng phải phủ đúng toàn bộ model.AuditActions: thiếu một dòng nghĩa là có action không route nào ghi
// (hoặc bảng có action lạ), cả hai đều là drift.
func TestAuditInstrumentation_TableCoversEveryAuditAction(t *testing.T) {
	covered := map[string]bool{}
	for _, rt := range auditRoutes {
		covered[rt.action] = true
	}
	for a := range auditActionsSetByHandler {
		covered[a] = true
	}
	for _, a := range model.AuditActions {
		if !covered[a] {
			t.Errorf("action %q có trong model.AuditActions nhưng không route nào trong bảng ghi nó", a)
		}
		delete(covered, a)
	}
	for a := range covered {
		t.Errorf("bảng có action %q không nằm trong model.AuditActions", a)
	}
}
