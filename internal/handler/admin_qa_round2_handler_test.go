package handler

// QA vòng 2, Lane G:
//   - G2 (N-01): tạo/sửa tổ chức tên rỗng hoặc toàn khoảng trắng phải bị chặn 400 TRƯỚC khi tới
//     service. Trước đây DTO dùng tag `binding:` (validator không đọc) và handler không gọi
//     ValidateStruct -> service nhận tên "   " và tạo tổ chức rỗng.
//   - G3 (N-02/A-P1-1): GET /system-roles/:id/users mặc định chỉ trả lượt gán đang hiệu lực;
//     trước đây không lọc nên số user theo vai trò cộng cả lượt đã thu hồi.

import (
	"context"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"study.com/v1/internal/dto"
	"study.com/v1/internal/service"
)

type orgServiceSpy struct {
	service.OrganizationServiceInterface
	createCalls []dto.CreateOrganizationDTO
	updateCalls []dto.UpdateOrganizationDTO
}

func (s *orgServiceSpy) CreateOrganization(ctx context.Context, creatorUserID uuid.UUID, req dto.CreateOrganizationDTO) (*dto.OrganizationResponseDTO, error) {
	s.createCalls = append(s.createCalls, req)
	return &dto.OrganizationResponseDTO{ID: uuid.New(), Name: req.Name}, nil
}

func (s *orgServiceSpy) UpdateOrganization(ctx context.Context, id uuid.UUID, req dto.UpdateOrganizationDTO) (*dto.OrganizationResponseDTO, error) {
	s.updateCalls = append(s.updateCalls, req)
	return &dto.OrganizationResponseDTO{ID: id}, nil
}

func newOrgTestApp(spy *orgServiceSpy) *fiber.App {
	h := NewOrganizationHandler(spy)
	app := fiber.New()
	app.Use(func(c *fiber.Ctx) error {
		c.Locals("user_id", uuid.New())
		return c.Next()
	})
	app.Post("/organizations", h.CreateOrganization)
	app.Put("/organizations/:id", h.UpdateOrganization)
	return app
}

func sendJSON(t *testing.T, app *fiber.App, method, path, body string) int {
	t.Helper()
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("app.Test: %v", err)
	}
	return resp.StatusCode
}

func TestCreateOrganization_TenRongHoacKhoangTrang_400(t *testing.T) {
	for _, name := range []string{"", "   ", " a "} {
		spy := &orgServiceSpy{}
		status := sendJSON(t, newOrgTestApp(spy), "POST", "/organizations", `{"name":"`+name+`"}`)
		if status != fiber.StatusBadRequest {
			t.Fatalf("tên %q: status = %d, muốn 400", name, status)
		}
		if len(spy.createCalls) != 0 {
			t.Fatalf("tên %q: service vẫn bị gọi với %+v — phải chặn ở handler", name, spy.createCalls)
		}
	}
}

func TestCreateOrganization_TenHopLe_DuocTrim(t *testing.T) {
	spy := &orgServiceSpy{}
	status := sendJSON(t, newOrgTestApp(spy), "POST", "/organizations", `{"name":"  ForteX  "}`)
	if status != fiber.StatusCreated {
		t.Fatalf("status = %d, muốn 201", status)
	}
	if len(spy.createCalls) != 1 || spy.createCalls[0].Name != "ForteX" {
		t.Fatalf("service nhận %+v, muốn đúng 1 lần với tên đã trim \"ForteX\"", spy.createCalls)
	}
}

func TestUpdateOrganization_DoiTenThanhKhoangTrang_400(t *testing.T) {
	spy := &orgServiceSpy{}
	status := sendJSON(t, newOrgTestApp(spy), "PUT", "/organizations/"+uuid.NewString(), `{"name":"    "}`)
	if status != fiber.StatusBadRequest {
		t.Fatalf("status = %d, muốn 400", status)
	}
	if len(spy.updateCalls) != 0 {
		t.Fatalf("service vẫn bị gọi với %+v", spy.updateCalls)
	}
}

type userSystemRoleServiceSpy struct {
	service.UserSystemRoleServiceInterface
	statuses []string
}

func (s *userSystemRoleServiceSpy) GetUsersBySystemRole(ctx context.Context, systemRoleID uuid.UUID, page, pageSize int, status string) (*dto.UserSystemRoleListResponseDTO, error) {
	s.statuses = append(s.statuses, status)
	return &dto.UserSystemRoleListResponseDTO{}, nil
}

func TestGetUsersBySystemRole_MacDinhChiActive(t *testing.T) {
	cases := []struct{ query, want string }{
		{"", "active"},
		{"?status=all", ""},
		{"?status=inactive", "inactive"},
	}
	for _, tc := range cases {
		spy := &userSystemRoleServiceSpy{}
		h := NewUserSystemRoleHandler(spy)
		app := fiber.New()
		app.Get("/system-roles/:system_role_id/users", h.GetUsersBySystemRole)

		status := sendJSON(t, app, "GET", "/system-roles/"+uuid.NewString()+"/users"+tc.query, "")
		if status != fiber.StatusOK {
			t.Fatalf("query %q: status = %d, muốn 200", tc.query, status)
		}
		if len(spy.statuses) != 1 || spy.statuses[0] != tc.want {
			t.Fatalf("query %q: service nhận status %v, muốn %q", tc.query, spy.statuses, tc.want)
		}
	}
}
