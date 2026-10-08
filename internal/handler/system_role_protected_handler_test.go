package handler

// Test cho QA A1 (261008), tầng handler: xoá vai trò hệ thống dựng sẵn -> 409 SYSTEM_ROLE_PROTECTED
// (không phải 400 chung, không phải 200).

import (
	"context"
	"encoding/json"
	"io"
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"study.com/v1/internal/service"
)

type stubSystemRoleDelete struct {
	service.SystemRoleServiceInterface
	err error
}

func (s stubSystemRoleDelete) DeleteSystemRole(ctx context.Context, id uuid.UUID, hardDelete bool) error {
	return s.err
}

func TestDeleteSystemRole_VaiTroDungSan_Tra409(t *testing.T) {
	h := NewSystemRoleHandler(stubSystemRoleDelete{err: service.ErrSystemRoleProtected})
	app := fiber.New()
	app.Delete("/system-roles/:id", h.DeleteSystemRole)
	resp, err := app.Test(httptest.NewRequest("DELETE", "/system-roles/"+uuid.NewString(), nil))
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != 409 {
		t.Fatalf("muốn 409, nhận %d", resp.StatusCode)
	}
	raw, _ := io.ReadAll(resp.Body)
	var body struct {
		Code string `json:"code"`
	}
	_ = json.Unmarshal(raw, &body)
	if body.Code != "SYSTEM_ROLE_PROTECTED" {
		t.Errorf("code=%q, body=%s", body.Code, raw)
	}
}

func TestDeleteSystemRole_ThanhCong_Tra200(t *testing.T) {
	h := NewSystemRoleHandler(stubSystemRoleDelete{})
	app := fiber.New()
	app.Delete("/system-roles/:id", h.DeleteSystemRole)
	resp, err := app.Test(httptest.NewRequest("DELETE", "/system-roles/"+uuid.NewString(), nil))
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != 200 {
		t.Fatalf("muốn 200, nhận %d", resp.StatusCode)
	}
}
