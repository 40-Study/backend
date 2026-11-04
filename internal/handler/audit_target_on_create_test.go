package handler

// QA3-06: tổ chức, danh mục, voucher và vai trò tổ chức được thêm vào nhật ký quản trị. Id của đối tượng vừa TẠO chỉ
// có trong response (không ở URL), nên handler phải đặt đích bằng SetAuditTarget; thiếu thì dòng nhật ký mất target_id.
// (Bảng router/audit_instrumentation_test.go thay handler cuối bằng stub nên không phủ phần này.)

import (
	"context"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"study.com/v1/internal/dto"
	"study.com/v1/internal/middleware"
	"study.com/v1/internal/model"
	"study.com/v1/internal/service"
)

type auditTargetSpy struct {
	mu      sync.Mutex
	entries []model.AuditEntry
}

func (s *auditTargetSpy) Record(_ context.Context, e model.AuditEntry) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.entries = append(s.entries, e)
	return nil
}

type orgCreateStub struct {
	service.OrganizationServiceInterface
	id uuid.UUID
}

func (s orgCreateStub) CreateOrganization(context.Context, uuid.UUID, dto.CreateOrganizationDTO) (*dto.OrganizationResponseDTO, error) {
	return &dto.OrganizationResponseDTO{ID: s.id, Name: "x"}, nil
}

type categoryCreateStub struct {
	service.CategoryServiceInterface
	id uuid.UUID
}

func (s categoryCreateStub) CreateCategory(context.Context, dto.CreateCategoryDTO) (*dto.CategoryResponseDTO, error) {
	return &dto.CategoryResponseDTO{ID: s.id, Name: "x"}, nil
}

type voucherCreateStub struct {
	service.VoucherServiceInterface
	id uuid.UUID
}

func (s voucherCreateStub) CreateVoucher(context.Context, *dto.CreateVoucherRequest) (*model.Voucher, error) {
	return &model.Voucher{ID: s.id}, nil
}

type roleCreateStub struct {
	service.RoleServiceInterface
	id uuid.UUID
}

func (s roleCreateStub) CreateRole(context.Context, *uuid.UUID, bool, dto.CreateRoleDTO) (*dto.RoleResponseDTO, error) {
	return &dto.RoleResponseDTO{ID: s.id, Name: "x"}, nil
}

func TestAuditTarget_CreateHandlersReportNewObjectID(t *testing.T) {
	actor, newID := uuid.New(), uuid.New()
	cases := []struct {
		name, action, targetType, body string
		handler                        fiber.Handler
		wantID                         func() string
	}{
		{"organization", model.AuditActionOrganizationCreate, "organization", `{"name":"Tổ chức mới"}`,
			NewOrganizationHandler(orgCreateStub{id: newID}).CreateOrganization, func() string { return newID.String() }},
		{"category", model.AuditActionCategoryCreate, "category", `{"name":"Danh mục mới"}`,
			NewCategoryHandler(categoryCreateStub{id: newID}).CreateCategory, func() string { return newID.String() }},
		{"voucher", model.AuditActionVoucherCreate, "voucher",
			`{"code":"NEW10","name":"Voucher mới","discount_unit":"MONEY","discount_method":"FIXED","discount_amount_money":1000}`,
			NewVoucherHandler(voucherCreateStub{id: newID}).CreateVoucher, func() string { return newID.String() }},
		{"org_role", model.AuditActionOrgRoleCreate, "org_role", `{"name":"Vai trò mới","organization_id":"` + uuid.NewString() + `"}`,
			NewRoleHandler(roleCreateStub{id: newID}, nil).CreateRole, func() string { return newID.String() }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rec := &auditTargetSpy{}
			app := fiber.New()
			app.Post("/things", func(c *fiber.Ctx) error { c.Locals("user_id", actor); return c.Next() },
				middleware.Audit(rec, tc.action, tc.targetType, ""), tc.handler)

			req := httptest.NewRequest("POST", "/things", strings.NewReader(tc.body))
			req.Header.Set("Content-Type", "application/json")
			resp, err := app.Test(req)
			if err != nil {
				t.Fatal(err)
			}
			if resp.StatusCode != fiber.StatusCreated {
				t.Fatalf("status %d, muốn 201", resp.StatusCode)
			}
			if len(rec.entries) != 1 {
				t.Fatalf("%d dòng nhật ký, muốn đúng 1", len(rec.entries))
			}
			e := rec.entries[0]
			want := tc.wantID()
			if e.Action != tc.action || e.TargetType != tc.targetType || e.TargetID != want || e.ActorID != actor {
				t.Errorf("entry = %+v, muốn action=%q target=%s/%s actor=%s", e, tc.action, tc.targetType, want, actor)
			}
		})
	}
}
