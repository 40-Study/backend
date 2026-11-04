package handler

// QA3-22: tên vai trò tổ chức trùng và gán lại cùng vai trò đều phải là 409 có mã ổn định + thông báo tiếng Việt
// (trước: 400 "duplicated key not allowed" và 409 tiếng Anh thô).

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"study.com/v1/internal/dto"
	"study.com/v1/internal/service"
)

type roleConflictStub struct {
	service.RoleServiceInterface
	err error
}

func (s roleConflictStub) CreateRole(context.Context, *uuid.UUID, bool, dto.CreateRoleDTO) (*dto.RoleResponseDTO, error) {
	return nil, s.err
}

func (s roleConflictStub) UpdateRole(context.Context, uuid.UUID, *uuid.UUID, bool, dto.UpdateRoleDTO) (*dto.RoleResponseDTO, error) {
	return nil, s.err
}

type assignConflictStub struct {
	service.UserOrganizationRoleServiceInterface
	err error
}

func (s assignConflictStub) AssignOrgRolesToUser(context.Context, uuid.UUID, dto.AssignOrgRolesToUserDTO, uuid.UUID, *uuid.UUID, bool) ([]dto.UserOrgRoleResponseDTO, error) {
	return nil, s.err
}

func conflictCall(t *testing.T, app *fiber.App, method, path, body string) (int, map[string]any) {
	t.Helper()
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp, err := app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := io.ReadAll(resp.Body)
	var out map[string]any
	_ = json.Unmarshal(raw, &out)
	return resp.StatusCode, out
}

func TestRole_DuplicateName_Returns409WithCode(t *testing.T) {
	h := NewRoleHandler(roleConflictStub{err: service.ErrRoleNameTaken}, nil)
	app := fiber.New()
	app.Use(func(c *fiber.Ctx) error { c.Locals("user_id", uuid.New()); return c.Next() })
	app.Post("/org-roles", h.CreateRole)
	app.Put("/org-roles/:id", h.UpdateRole)

	for _, tc := range []struct{ method, path, body string }{
		{"POST", "/org-roles", fmt.Sprintf(`{"name":"Trợ giảng","organization_id":"%s"}`, uuid.New())},
		{"PUT", "/org-roles/" + uuid.NewString(), `{"name":"Trợ giảng"}`},
	} {
		status, body := conflictCall(t, app, tc.method, tc.path, tc.body)
		if status != 409 || body["code"] != "ROLE_NAME_TAKEN" {
			t.Errorf("%s: status=%d body=%v, muốn 409 ROLE_NAME_TAKEN", tc.method, status, body)
		}
		if msg, _ := body["message"].(string); !strings.Contains(msg, "Tên vai trò") || strings.Contains(msg, "duplicated") {
			t.Errorf("%s: message %q phải tiếng Việt", tc.method, msg)
		}
	}
}

func TestAssignOrgRole_AlreadyAssigned_Returns409Vietnamese(t *testing.T) {
	wrapped := fmt.Errorf("%w: %s", service.ErrOrgRoleAlreadyAssigned, "Trợ giảng")
	h := NewUserOrganizationRoleHandler(assignConflictStub{err: wrapped}, nil)
	app := fiber.New()
	app.Use(func(c *fiber.Ctx) error { c.Locals("user_id", uuid.New()); return c.Next() })
	app.Post("/users/:user_id/org-roles", h.AssignOrgRolesToUser)

	body := fmt.Sprintf(`{"role_ids":["%s"],"organization_id":"%s"}`, uuid.New(), uuid.New())
	status, out := conflictCall(t, app, "POST", "/users/"+uuid.NewString()+"/org-roles", body)
	if status != 409 || out["code"] != "ORG_ROLE_ALREADY_ASSIGNED" {
		t.Fatalf("status=%d body=%v, muốn 409 ORG_ROLE_ALREADY_ASSIGNED", status, out)
	}
	for _, field := range []string{"message", "error"} {
		if msg, _ := out[field].(string); !strings.Contains(msg, "Vai trò đã được gán") || !strings.Contains(msg, "Trợ giảng") {
			t.Errorf("%s = %q, muốn tiếng Việt kèm tên vai trò", field, msg)
		}
	}
}
