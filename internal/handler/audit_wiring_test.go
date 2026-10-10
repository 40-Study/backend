package handler

// Phase 8 (plan 261008): các "handler one-liner" của nhật ký hoạt động — phần middleware.Audit KHÔNG tự suy ra
// được từ URL. Handler THẬT + middleware.Audit THẬT + service giả, recorder giả.
//   - mở khoá  -> action user.unlock (khoá -> user.lock mặc định)
//   - tạo system role -> target là id trong response
//   - đổi phí nền tảng -> target platform_fee, metadata {old,new}
//   - broadcast -> metadata {audience,roles,recipient_count,title}; gửi dở dang (500) VẪN được ghi, kèm số đã gửi

import (
	"context"
	"errors"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"study.com/v1/internal/dto"
	"study.com/v1/internal/middleware"
	"study.com/v1/internal/model"
	"study.com/v1/internal/service"
)

type auditWiringSpy struct{ entries []model.AuditEntry }

func (s *auditWiringSpy) Record(_ context.Context, e model.AuditEntry) error {
	s.entries = append(s.entries, e)
	return nil
}

// auditWiringCall: method path, chuỗi (gán actor) -> Audit(action,targetType,"") -> handler.
func auditWiringCall(t *testing.T, method, route, url, body string, action, targetType, targetParam string, h fiber.Handler) (*auditWiringSpy, uuid.UUID, int) {
	t.Helper()
	spy, actor := &auditWiringSpy{}, uuid.New()
	app := fiber.New()
	setActor := func(c *fiber.Ctx) error { c.Locals("user_id", actor); return c.Next() }
	app.Add(method, route, setActor, middleware.Audit(spy, action, targetType, targetParam), h)
	req := httptest.NewRequest(method, url, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp, err := app.Test(req, -1)
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	return spy, actor, resp.StatusCode
}

type auditUserAdminSvc struct{ service.UserAdminServiceInterface }

func (auditUserAdminSvc) UpdateUserStatus(context.Context, uuid.UUID, uuid.UUID, bool, *string) (*dto.AdminUserDetailDTO, error) {
	return &dto.AdminUserDetailDTO{}, nil
}

func TestUserAdminHandler_LockAndUnlockSetAuditAction(t *testing.T) {
	h := NewUserAdminHandler(auditUserAdminSvc{})
	id := uuid.New().String()
	for _, tc := range []struct{ body, want string }{
		{`{"is_active":false,"reason":"spam"}`, model.AuditActionUserLock},
		{`{"is_active":true}`, model.AuditActionUserUnlock},
	} {
		spy, _, status := auditWiringCall(t, "PUT", "/users/:id/status", "/users/"+id+"/status", tc.body,
			model.AuditActionUserLock, "user", "id", h.UpdateUserStatus)
		if status != 200 || len(spy.entries) != 1 || spy.entries[0].Action != tc.want || spy.entries[0].TargetID != id {
			t.Errorf("body %s: status=%d entries=%+v, muốn 1 dòng action=%s target=%s", tc.body, status, spy.entries, tc.want, id)
		}
	}
}

type auditSystemRoleSvc struct {
	service.SystemRoleServiceInterface
	id uuid.UUID
}

func (s auditSystemRoleSvc) CreateSystemRole(context.Context, dto.CreateSystemRoleDTO) (*dto.SystemRoleResponseDTO, error) {
	return &dto.SystemRoleResponseDTO{ID: s.id}, nil
}

func TestSystemRoleHandler_CreateSetsAuditTargetFromResponse(t *testing.T) {
	id := uuid.New()
	h := NewSystemRoleHandler(auditSystemRoleSvc{id: id})
	spy, _, status := auditWiringCall(t, "POST", "/system-roles", "/system-roles", `{"name":"X"}`,
		model.AuditActionSystemRoleCreate, "system_role", "", h.CreateSystemRole)
	if status != 201 || len(spy.entries) != 1 || spy.entries[0].TargetID != id.String() {
		t.Errorf("status=%d entries=%+v, muốn 1 dòng target=%s", status, spy.entries, id)
	}
}

type auditFeeSvc struct {
	service.PlatformSettingServiceInterface
	old    decimal.Decimal
	getErr error
}

func (s auditFeeSvc) GetPlatformFeePercent(context.Context) (decimal.Decimal, error) { return s.old, s.getErr }
func (s auditFeeSvc) SetPlatformFeePercent(context.Context, uuid.UUID, decimal.Decimal) error {
	return nil
}

func TestAdminOrderHandler_PlatformFeeUpdateAuditsOldAndNew(t *testing.T) {
	call := func(svc auditFeeSvc) model.AuditEntry {
		h := NewAdminOrderHandler(nil, nil, svc)
		spy, _, status := auditWiringCall(t, "PUT", "/fee", "/fee", `{"platform_fee_percent":12.5}`,
			model.AuditActionSettingPlatformFeeUpdate, "setting", "", h.UpdatePlatformFeeSetting)
		if status != 200 || len(spy.entries) != 1 {
			t.Fatalf("status=%d entries=%+v", status, spy.entries)
		}
		return spy.entries[0]
	}
	e := call(auditFeeSvc{old: decimal.RequireFromString("10")})
	if e.TargetType != "setting" || e.TargetID != "platform_fee" || e.Metadata["old"] != "10" || e.Metadata["new"] != "12.5" {
		t.Errorf("entry = %+v, muốn target setting/platform_fee meta old=10 new=12.5", e)
	}
	// Đọc giá trị cũ lỗi: việc đổi phí vẫn thành công, nhật ký có "new" và KHÔNG có "old" bịa.
	e = call(auditFeeSvc{getErr: errors.New("db down")})
	if _, has := e.Metadata["old"]; has || e.Metadata["new"] != "12.5" {
		t.Errorf("khi đọc old lỗi, meta = %v, muốn chỉ có new=12.5", e.Metadata)
	}
}

func TestBroadcastHandler_AuditsSuccessAndPartialFailure(t *testing.T) {
	send := func(svc *fakeBroadcastService) (*auditWiringSpy, int) {
		h := NewAdminBroadcastHandler(svc)
		spy, _, status := auditWiringCall(t, "POST", "/b", "/b", `{"title":"Bảo trì","content":"c","audience":"roles","roles":["STUDENT"]}`,
			model.AuditActionNotificationBroadcast, "", "", h.Send)
		return spy, status
	}

	spy, status := send(&fakeBroadcastService{sendRes: &dto.BroadcastResultDTO{RecipientCount: 42, Audience: "roles", Roles: []string{"STUDENT"}, NotificationType: "system"}})
	if status != 201 || len(spy.entries) != 1 {
		t.Fatalf("thành công: status=%d entries=%+v", status, spy.entries)
	}
	m := spy.entries[0].Metadata
	if m["audience"] != "roles" || m["recipient_count"] != int64(42) || m["title"] != "Bảo trì" || m["partial"] != nil {
		t.Errorf("meta thành công = %v", m)
	}

	// Gửi dở dang: 500 BROADCAST_PARTIAL vẫn phải có đúng 1 dòng nhật ký với số đã gửi (quyết định điều phối, ngoại lệ hẹp của D6).
	spy, status = send(&fakeBroadcastService{err: &service.BroadcastPartialError{Delivered: 1500, Cause: errors.New("db")}})
	if status != 500 || len(spy.entries) != 1 {
		t.Fatalf("dở dang: status=%d entries=%+v, muốn 500 và đúng 1 dòng", status, spy.entries)
	}
	e := spy.entries[0]
	if e.Action != model.AuditActionNotificationBroadcast || e.StatusCode != 500 || e.Metadata["recipient_count"] != int64(1500) || e.Metadata["partial"] != true {
		t.Errorf("entry dở dang = %+v", e)
	}

	// Lỗi khác (vd 422 NO_RECIPIENTS, 500 hạ tầng thuần) KHÔNG được ghi.
	for _, err := range []error{
		&service.BroadcastError{Status: 422, Code: "NO_RECIPIENTS", Message: "x"},
		errors.New("infra"),
	} {
		spy, _ = send(&fakeBroadcastService{err: err})
		if len(spy.entries) != 0 {
			t.Errorf("lỗi %v không được ghi nhật ký, có %+v", err, spy.entries)
		}
	}
}
