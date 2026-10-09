package middleware

// Test middleware.Audit (contract C2, plan D5/D6): chỉ ghi khi handler thành công, không bao giờ
// ghi sau Go error, lỗi recorder không đổi response.

import (
	"context"
	"errors"
	"io"
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"study.com/v1/internal/model"
)

type auditSpy struct {
	entries []model.AuditEntry
	err     error
	boom    bool
}

func (s *auditSpy) Record(_ context.Context, e model.AuditEntry) error {
	if s.boom {
		panic("recorder exploded")
	}
	s.entries = append(s.entries, e)
	return s.err
}

// auditApp dựng route giả lập chuỗi thật: (gán actor) -> Audit -> handler.
func auditApp(rec AuditRecorder, actor uuid.UUID, handler fiber.Handler) *fiber.App {
	app := fiber.New()
	setActor := func(c *fiber.Ctx) error {
		if actor != uuid.Nil {
			c.Locals("user_id", actor)
		}
		return c.Next()
	}
	app.Post("/things/:id/act", setActor, Audit(rec, "thing.act", "thing", "id"), handler)
	return app
}

func auditCall(t *testing.T, app *fiber.App) (int, string) {
	t.Helper()
	resp, err := app.Test(httptest.NewRequest("POST", "/things/abc-123/act", nil), -1)
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, string(body)
}

func respond(status int) fiber.Handler {
	return func(c *fiber.Ctx) error { return c.Status(status).SendString("body") }
}

func TestAudit_RecordsOnSuccess(t *testing.T) {
	for _, status := range []int{200, 201, 204} {
		spy, actor := &auditSpy{}, uuid.New()
		app := auditApp(spy, actor, respond(status))
		if got, _ := auditCall(t, app); got != status {
			t.Fatalf("status = %d, muốn %d", got, status)
		}
		if len(spy.entries) != 1 {
			t.Fatalf("status %d: %d dòng, muốn đúng 1", status, len(spy.entries))
		}
		e := spy.entries[0]
		if e.ActorID != actor || e.Action != "thing.act" || e.TargetType != "thing" || e.TargetID != "abc-123" || e.StatusCode != status || e.IP == "" {
			t.Errorf("entry sai: %+v", e)
		}
		if e.Metadata != nil {
			t.Errorf("không SetAuditMeta thì metadata phải nil, có %v", e.Metadata)
		}
	}
}

func TestAudit_NoRecordOnNon2xx(t *testing.T) {
	for _, status := range []int{301, 400, 401, 403, 404, 409, 422, 429, 500, 503} {
		spy := &auditSpy{}
		app := auditApp(spy, uuid.New(), respond(status))
		if got, _ := auditCall(t, app); got != status {
			t.Fatalf("status = %d, muốn %d", got, status)
		}
		if len(spy.entries) != 0 {
			t.Errorf("status %d không được ghi nhật ký, có %d dòng", status, len(spy.entries))
		}
	}
}

// Ca mà việc đọc status sẽ làm sai: handler trả Go error thì status trên response vẫn là 200 mặc
// định (Fiber chưa chạy error handler) -> bản đọc status sẽ ghi nhầm một hành động đã THẤT BẠI.
func TestAudit_NoRecordWhenHandlerReturnsGoError(t *testing.T) {
	for name, h := range map[string]fiber.Handler{
		"plain error":   func(c *fiber.Ctx) error { return errors.New("db down") },
		"fiber 4xx":     func(c *fiber.Ctx) error { return fiber.NewError(fiber.StatusBadRequest, "bad") },
		"after 2xx set": func(c *fiber.Ctx) error { _ = c.Status(200).SendString("partial"); return errors.New("late failure") },
	} {
		spy := &auditSpy{}
		app := auditApp(spy, uuid.New(), h)
		if status, _ := auditCall(t, app); status < 400 {
			t.Fatalf("%s: status = %d, muốn lỗi", name, status)
		}
		if len(spy.entries) != 0 {
			t.Errorf("%s: handler trả Go error nhưng vẫn ghi %d dòng", name, len(spy.entries))
		}
	}
}

func TestAudit_RecorderFailureLeavesResponseUnchanged(t *testing.T) {
	cases := map[string]*auditSpy{
		"error": {err: errors.New("insert failed")},
		"panic": {boom: true},
	}
	for name, spy := range cases {
		app := auditApp(spy, uuid.New(), func(c *fiber.Ctx) error { return c.Status(201).SendString("created") })
		status, body := auditCall(t, app)
		if status != 201 || body != "created" {
			t.Errorf("%s: response đổi thành %d %q, muốn 201 \"created\"", name, status, body)
		}
	}
}

func TestAudit_NoActorMeansNoRecordAndNoFailure(t *testing.T) {
	spy := &auditSpy{}
	app := auditApp(spy, uuid.Nil, respond(200))
	if status, _ := auditCall(t, app); status != 200 {
		t.Fatalf("status = %d", status)
	}
	if len(spy.entries) != 0 {
		t.Fatalf("không có actor thì không thể ghi, có %d dòng", len(spy.entries))
	}
}

func TestAudit_HandlerCanOverrideActionTargetAndMergeMeta(t *testing.T) {
	spy := &auditSpy{}
	app := auditApp(spy, uuid.New(), func(c *fiber.Ctx) error {
		SetAuditAction(c, "thing.undo")
		SetAuditMeta(c, map[string]any{"a": 1})
		SetAuditMeta(c, map[string]any{"b": "two", "a": 3})
		SetAuditTarget(c, "from-body")
		return c.SendStatus(200)
	})
	auditCall(t, app)
	if len(spy.entries) != 1 {
		t.Fatalf("%d dòng", len(spy.entries))
	}
	e := spy.entries[0]
	if e.Action != "thing.undo" || e.TargetID != "from-body" || e.Metadata["a"] != 3 || e.Metadata["b"] != "two" || len(e.Metadata) != 2 {
		t.Errorf("entry = %+v", e)
	}
}

func TestAudit_StateDoesNotLeakBetweenRequests(t *testing.T) {
	spy := &auditSpy{}
	calls := 0
	app := auditApp(spy, uuid.New(), func(c *fiber.Ctx) error {
		calls++
		if calls == 1 {
			SetAuditAction(c, "thing.first")
			SetAuditMeta(c, map[string]any{"k": "v"})
		}
		return c.SendStatus(200)
	})
	auditCall(t, app)
	auditCall(t, app)
	if len(spy.entries) != 2 || spy.entries[1].Action != "thing.act" || spy.entries[1].Metadata != nil {
		t.Fatalf("request thứ hai thừa hưởng trạng thái của request đầu: %+v", spy.entries)
	}
}

func TestAudit_SettersAreNoOpWithoutMiddleware(t *testing.T) {
	app := fiber.New()
	app.Get("/x", func(c *fiber.Ctx) error {
		SetAuditAction(c, "a")
		SetAuditMeta(c, map[string]any{"k": 1})
		SetAuditTarget(c, "t")
		return c.SendStatus(204)
	})
	resp, err := app.Test(httptest.NewRequest("GET", "/x", nil))
	if err != nil || resp.StatusCode != 204 {
		t.Fatalf("resp=%v err=%v", resp, err)
	}
}

func TestAudit_RouteWithoutTargetParamHasNoTargetID(t *testing.T) {
	spy := &auditSpy{}
	app := fiber.New()
	app.Post("/broadcast", func(c *fiber.Ctx) error { c.Locals("user_id", uuid.New()); return c.Next() },
		Audit(spy, "notification.broadcast", "", ""), respond(201))
	if resp, err := app.Test(httptest.NewRequest("POST", "/broadcast", nil)); err != nil || resp.StatusCode != 201 {
		t.Fatalf("resp=%v err=%v", resp, err)
	}
	if len(spy.entries) != 1 || spy.entries[0].TargetID != "" || spy.entries[0].TargetType != "" {
		t.Fatalf("entries = %+v", spy.entries)
	}
}
