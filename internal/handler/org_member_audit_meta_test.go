package handler

// QA4-A02: dòng nhật ký gán / thu hồi vai trò tổ chức phải mang metadata do service dựng (trước đây metadata: null).
// Handler THẬT sau middleware.Audit THẬT; service là stub. Mutation đã kiểm: bỏ setOrgMemberAuditMeta ở một trong
// hai handler làm nhánh đó ĐỎ.

import (
	"context"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"study.com/v1/internal/dto"
	"study.com/v1/internal/middleware"
	"study.com/v1/internal/model"
	"study.com/v1/internal/service"
)

type orgMemberAuditStub struct {
	service.UserOrganizationRoleServiceInterface
}

func (orgMemberAuditStub) AssignOrgRolesToUser(context.Context, uuid.UUID, dto.AssignOrgRolesToUserDTO, uuid.UUID, *uuid.UUID, bool) ([]dto.UserOrgRoleResponseDTO, error) {
	return nil, nil
}
func (orgMemberAuditStub) RevokeOrgRoleFromUser(context.Context, uuid.UUID, uuid.UUID, uuid.UUID, *uuid.UUID, bool) error {
	return nil
}
func (orgMemberAuditStub) OrgMemberAuditMeta(context.Context, uuid.UUID, uuid.UUID, []uuid.UUID) (map[string]any, error) {
	return map[string]any{"role_name": "gán"}, nil
}
func (orgMemberAuditStub) OrgAssignmentAuditMeta(context.Context, uuid.UUID) (map[string]any, error) {
	return map[string]any{"role_name": "thu hồi"}, nil
}

func TestOrgMemberAudit_HandlersAttachMeta(t *testing.T) {
	h := NewUserOrganizationRoleHandler(orgMemberAuditStub{}, nil)
	actor, target := uuid.New(), uuid.New()
	body := `{"organization_id":"` + uuid.NewString() + `","role_ids":["` + uuid.NewString() + `"]}`
	for _, tc := range []struct {
		name, method, path, body, want string
		handler                        fiber.Handler
	}{
		{"gán", "POST", "/users/" + target.String() + "/org-roles", body, "gán", h.AssignOrgRolesToUser},
		{"thu hồi", "DELETE", "/users/" + target.String() + "/org-roles/" + uuid.NewString(), "", "thu hồi", h.RevokeOrgRoleFromUser},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rec := &auditTargetSpy{}
			app := fiber.New()
			route := "/users/:user_id/org-roles"
			if tc.method == "DELETE" {
				route += "/:org_role_id"
			}
			app.Add(tc.method, route, func(c *fiber.Ctx) error { c.Locals("user_id", actor); return c.Next() },
				middleware.Audit(rec, model.AuditActionUserOrgRoleAssign, "user", "user_id"), tc.handler)
			req := httptest.NewRequest(tc.method, tc.path, strings.NewReader(tc.body))
			req.Header.Set("Content-Type", "application/json")
			resp, err := app.Test(req)
			if err != nil {
				t.Fatal(err)
			}
			if resp.StatusCode != fiber.StatusOK || len(rec.entries) != 1 {
				t.Fatalf("status=%d entries=%d, muốn 200 và 1 dòng", resp.StatusCode, len(rec.entries))
			}
			if got := rec.entries[0].Metadata["role_name"]; got != tc.want {
				t.Errorf("metadata = %v, muốn role_name=%q", rec.entries[0].Metadata, tc.want)
			}
		})
	}
}
