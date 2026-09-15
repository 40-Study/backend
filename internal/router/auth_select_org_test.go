package router

import (
	"context"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"study.com/v1/internal/config"
	"study.com/v1/internal/dto"
	"study.com/v1/internal/handler"
	"study.com/v1/internal/service"
)

// fakeAuthServiceForRouteTest — fake toi thieu cho AuthServiceInterface. Nhung interface de viec
// them method moi khong lam vo file test nay.
//
// Cac method khong duoc override se panic neu bi goi — do la chu y: bai test chi duoc phep cham
// toi SelectOrg cho cac case di qua duoc validation.
//
// GIOI HAN CAN BIET: fake nay chung minh duoc "handler goi service dung tham so", KHONG chung
// minh duoc "service tra loi duoc". Dung cai lo hong BLOCKER-1 nam sau lop fake nay — xem
// select_org_live_test.go (di het duong that: route -> middleware -> handler -> service -> redis).
type fakeAuthServiceForRouteTest struct {
	service.AuthServiceInterface
	selectOrgCalls int
	lastReq        dto.SelectOrgRequestDto
	lastUserID     uuid.UUID
	lastDeviceID   uuid.UUID
	lastActiveRole string
	err            error
}

func (f *fakeAuthServiceForRouteTest) SelectOrg(
	ctx context.Context,
	userID uuid.UUID,
	deviceID uuid.UUID,
	activeRole string,
	req dto.SelectOrgRequestDto,
) (*dto.SelectRoleResponseDto, error) {
	f.selectOrgCalls++
	f.lastReq = req
	f.lastUserID = userID
	f.lastDeviceID = deviceID
	f.lastActiveRole = activeRole
	if f.err != nil {
		return nil, f.err
	}
	return &dto.SelectRoleResponseDto{Completed: true}, nil
}

// Chi chu y ve CACH test trong file nay: SetupAuthRoutes gan authRateLimiter len nhom route cong
// khai, va RateLimiter fail-CLOSED khi Redis loi (M-02, audit 260909). Voi redis=nil thi MOI
// request that qua nhom do tra 503 — dung thiet ke, nhung no lam request khong con do duoc hanh
// vi handler.
//
// Vi vay tach doi hai thu can khang dinh:
//  1. Route CO duoc dang ky khong  -> doc bang route (app.GetRoutes()), khong chay middleware.
//  2. Handler lam dung viec gi     -> dung mot app toi gian chi co select-org.
//
// Khong dung mot chiec app "that" roi ky vong no chay: se chi khang dinh duoc hanh vi cua
// rate limiter, khong phai cua thay doi nay.

// routeExists bao cao route METHOD PATH co duoc dang ky trong app hay khong.
func routeExists(app *fiber.App, method, path string) bool {
	for _, r := range app.GetRoutes() {
		if r.Method == method && r.Path == path {
			return true
		}
	}
	return false
}

// TestAuthRoutes_SelectOrgRegistered — select-org phai ton tai trong bang route.
//
// BLOCKER-1 (review 260915): ban dau route duoc doc lap dang ky nhung KHONG BAO GIO chay duoc
// (SelectRole xoa pending key truoc do). Bai test nay chi pin SU TON TAI; ve "chay duoc" nam o
// select_org_live_test.go.
func TestAuthRoutes_SelectOrgRegistered(t *testing.T) {
	app := fiber.New()
	api := app.Group("/api")
	SetupAuthRoutes(api, &config.Config{}, handler.NewAuthHandler(&fakeAuthServiceForRouteTest{}), nil, nil)

	// Self-test cua phep kiem: routeExists PHAI tra false cho mot path chac chan khong ton tai.
	// Khong co dong nay thi mot helper luon-tra-true se cho ra green vo nghia.
	if routeExists(app, "POST", "/api/auth/khong-ton-tai-dau") {
		t.Fatal("routeExists tra true cho path khong ton tai — phep kiem khong phan biet duoc gi")
	}

	if !routeExists(app, "POST", "/api/auth/select-org") {
		t.Fatal("POST /api/auth/select-org KHONG duoc dang ky — web se nhan 404 khi doi to chuc")
	}
}

// TestAuthRoutes_SelectRoleStillRegistered — chot chan hoi quy: select-org nam ngay canh
// select-role trong cung nhom route, nen phai khang dinh select-role khong bi dich ten hay go mat.
func TestAuthRoutes_SelectRoleStillRegistered(t *testing.T) {
	app := fiber.New()
	api := app.Group("/api")
	SetupAuthRoutes(api, &config.Config{}, handler.NewAuthHandler(&fakeAuthServiceForRouteTest{}), nil, nil)

	if !routeExists(app, "POST", "/api/auth/select-role") {
		t.Fatal("POST /api/auth/select-role KHONG duoc dang ky — route dang song bi go mat")
	}
}

// withFakeAuthLocals gia lap dung 4 gia tri ma middleware.AuthMiddleware that set vao Locals.
// Day KHONG phai ban sao cua AuthMiddleware: no chi bom san gia tri de test duoc tang handler.
// Duong di that (qua middleware that) nam o select_org_live_test.go.
func withFakeAuthLocals(userID, deviceID uuid.UUID, activeRole string) fiber.Handler {
	return func(c *fiber.Ctx) error {
		c.Locals("user_id", userID)
		c.Locals("device_id", deviceID)
		c.Locals("active_role", activeRole)
		return c.Next()
	}
}

// newSelectOrgOnlyApp dung mot app toi gian: chi select-org + gia lap Locals, khong rate limiter.
// Nho vay request chay toi duoc handler de kiem tra hanh vi validate/parse.
func newSelectOrgOnlyApp(t *testing.T) (*fiber.App, *fakeAuthServiceForRouteTest) {
	t.Helper()
	fake := &fakeAuthServiceForRouteTest{}
	h := handler.NewAuthHandler(fake)
	app := fiber.New()
	app.Post("/api/auth/select-org",
		withFakeAuthLocals(uuid.New(), uuid.New(), "STUDENT"),
		h.SelectOrg)
	return app, fake
}

func postJSON(t *testing.T, app *fiber.App, path, body string) int {
	t.Helper()
	req := httptest.NewRequest("POST", path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("app.Test loi: %v", err)
	}
	return resp.StatusCode
}

// TestSelectOrgHandler_RejectsMissingAuthLocals — thieu danh tinh trong Locals (tuc la request
// khong di qua AuthMiddleware, hoac middleware bi go) PHAI la 401, va service KHONG duoc goi.
// Day la chot chan "khong the doi org khi chua dang nhap".
func TestSelectOrgHandler_RejectsMissingAuthLocals(t *testing.T) {
	fake := &fakeAuthServiceForRouteTest{}
	h := handler.NewAuthHandler(fake)
	app := fiber.New()
	// KHONG gan withFakeAuthLocals — gia lap middleware bi go.
	app.Post("/api/auth/select-org", h.SelectOrg)

	status := postJSON(t, app, "/api/auth/select-org", `{}`)

	if status != fiber.StatusUnauthorized {
		t.Errorf("thieu user_id/device_id trong Locals: mong doi 401, nhan %d", status)
	}
	if fake.selectOrgCalls != 0 {
		t.Errorf("service duoc goi %d lan khi chua xac thuc, mong doi 0", fake.selectOrgCalls)
	}
}

// TestSelectOrgHandler_RejectsMalformedOrganizationID — organization_id phai la UUID hop le
// (validate:"omitempty,uuid"). Mot chuoi rac phai bi chan o tang handler chu khong xuong service,
// neu khong service se nhan gia tri khong parse duoc.
func TestSelectOrgHandler_RejectsMalformedOrganizationID(t *testing.T) {
	app, fake := newSelectOrgOnlyApp(t)

	status := postJSON(t, app, "/api/auth/select-org", `{"organization_id":"khong-phai-uuid"}`)

	if status != fiber.StatusBadRequest {
		t.Errorf("organization_id khong phai UUID: mong doi 400, nhan %d", status)
	}
	if fake.selectOrgCalls != 0 {
		t.Errorf("service duoc goi %d lan voi organization_id khong hop le, mong doi 0", fake.selectOrgCalls)
	}
}

// TestSelectOrgHandler_ReachesServiceWithParsedBody — chot chan hop dong body: body dung
// snake_case nhu SelectOrgRequestDto, va danh tinh (user_id/device_id/active_role) duoc lay TU
// TOKEN chu KHONG tu body. Bai test khang dinh ca hai ve: body parse dung, va service nhan dung
// gia tri Locals — mot lan doi nguon danh tinh se do ngay.
func TestSelectOrgHandler_ReachesServiceWithParsedBody(t *testing.T) {
	app, fake := newSelectOrgOnlyApp(t)

	status := postJSON(t, app, "/api/auth/select-org",
		`{"organization_id":"11111111-1111-1111-1111-111111111111"}`)

	if status != fiber.StatusOK {
		t.Fatalf("mong doi 200, nhan %d", status)
	}
	if fake.selectOrgCalls != 1 {
		t.Fatalf("service duoc goi %d lan, mong doi 1", fake.selectOrgCalls)
	}
	if fake.lastReq.OrganizationID != "11111111-1111-1111-1111-111111111111" {
		t.Errorf("OrganizationID = %q, mong doi dung uuid da gui", fake.lastReq.OrganizationID)
	}
	if fake.lastUserID == uuid.Nil {
		t.Error("userID truyen xuong service la uuid.Nil — danh tinh khong den tu token Locals")
	}
	if fake.lastDeviceID == uuid.Nil {
		t.Error("deviceID truyen xuong service la uuid.Nil — token thieu device_id")
	}
	if fake.lastActiveRole != "STUDENT" {
		t.Errorf("activeRole = %q, mong doi %q — role phai lay tu claim trong token", fake.lastActiveRole, "STUDENT")
	}
}

// TestSelectOrgHandler_AllowsEmptyOrganization — organization_id rong la HOP LE (DTO ghi ro:
// rong = che do "Doc lap", active_org=null), nen body rong phai xuong duoc service chu khong bi
// validation chan. Neu ai them `required` vao OrganizationID, bai test nay do — va nguoi khong
// thuoc to chuc nao se khong the quay ve che do Doc lap.
func TestSelectOrgHandler_AllowsEmptyOrganization(t *testing.T) {
	app, fake := newSelectOrgOnlyApp(t)

	status := postJSON(t, app, "/api/auth/select-org", `{}`)

	if status != fiber.StatusOK {
		t.Fatalf("mong doi 200 (che do Doc lap), nhan %d", status)
	}
	if fake.selectOrgCalls != 1 {
		t.Errorf("service duoc goi %d lan, mong doi 1", fake.selectOrgCalls)
	}
	if fake.lastReq.OrganizationID != "" {
		t.Errorf("OrganizationID = %q, mong doi rong", fake.lastReq.OrganizationID)
	}
}

// TestSelectOrgHandler_ClassifiesErrors — LOW-7 (review 260915): org khong thuoc user la loi
// QUYEN (403), khong duoc tra 400 nhu loi nhap lieu; loi ha tang (Redis/DB) phai la 500 de client
// va monitoring phan biet duoc backend hong voi body sai.
func TestSelectOrgHandler_ClassifiesErrors(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want int
	}{
		{"org khong thuoc user", service.ErrOrgNotBelongToUser, fiber.StatusForbidden},
		{"organization_id sai dinh dang", service.ErrInvalidOrgIDFormat, fiber.StatusBadRequest},
		{"khong tim thay user", service.ErrUserNotFound, fiber.StatusUnauthorized},
		{"loi ha tang (redis/db)", context.DeadlineExceeded, fiber.StatusInternalServerError},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			fake := &fakeAuthServiceForRouteTest{err: tc.err}
			h := handler.NewAuthHandler(fake)
			app := fiber.New()
			app.Post("/api/auth/select-org",
				withFakeAuthLocals(uuid.New(), uuid.New(), "STUDENT"),
				h.SelectOrg)

			status := postJSON(t, app, "/api/auth/select-org", `{}`)
			if status != tc.want {
				t.Errorf("mong doi %d, nhan %d", tc.want, status)
			}
		})
	}
}
