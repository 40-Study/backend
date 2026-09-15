package router

import (
	"context"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gofiber/fiber/v2"
	"study.com/v1/internal/config"
	"study.com/v1/internal/dto"
	"study.com/v1/internal/handler"
	"study.com/v1/internal/service"
)

// fakeAuthServiceForRouteTest — fake toi thieu cho AuthServiceInterface. Nhung interface de viec
// them method moi (vd SelectOrg) khong lam vo file test nay.
//
// Cac method khong duoc override se panic neu bi goi — do la chu y: bai test chi duoc phep cham
// toi SelectOrg cho cac case di qua duoc validation.
type fakeAuthServiceForRouteTest struct {
	service.AuthServiceInterface
	selectOrgCalls int
	lastReq        dto.SelectOrgRequestDto
}

func (f *fakeAuthServiceForRouteTest) SelectOrg(ctx context.Context, req dto.SelectOrgRequestDto) (*dto.SelectRoleResponseDto, error) {
	f.selectOrgCalls++
	f.lastReq = req
	return &dto.SelectRoleResponseDto{Completed: true}, nil
}

// Chi chu y ve CACH test trong file nay: SetupAuthRoutes gan authRateLimiter len select-org, va
// RateLimiter fail-CLOSED khi Redis loi (M-02, audit 260909). Voi redis=nil thi MOI request that
// qua nhom do tra 503 — dung thiet ke, nhung no lam request khong con do duoc hanh vi handler.
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

// TestAuthRoutes_SelectOrgRegistered — bug goc: web/src/lib/meet/auth.ts (dong 87, 95) goi
// POST /auth/select-org o buoc 3 cua luong dang nhap, nhung route chua bao gio duoc dang ky nen
// luon 404 va luong meet chet o buoc cuoi.
//
// Day la bai test dong khoang trong do: no pin SU TON TAI cua route, va se do ngay neu ai go
// dong dang ky ra.
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
		t.Fatal("POST /api/auth/select-org KHONG duoc dang ky — " +
			"web/src/lib/meet/auth.ts:87,95 se nhan 404 o buoc chon to chuc")
	}
}

// TestAuthRoutes_SelectRoleStillRegistered — chot chan hoi quy: select-org duoc them ngay canh
// select-role trong cung nhom route, nen phai khang dinh select-role khong bi dich ten hay go mat.
func TestAuthRoutes_SelectRoleStillRegistered(t *testing.T) {
	app := fiber.New()
	api := app.Group("/api")
	SetupAuthRoutes(api, &config.Config{}, handler.NewAuthHandler(&fakeAuthServiceForRouteTest{}), nil, nil)

	if !routeExists(app, "POST", "/api/auth/select-role") {
		t.Fatal("POST /api/auth/select-role KHONG duoc dang ky — route dang song bi go mat")
	}
}

// newSelectOrgOnlyApp dung mot app toi gian: chi select-org, khong rate limiter, khong auth.
// Nho vay request chay toi duoc handler de kiem tra hanh vi validate/parse.
func newSelectOrgOnlyApp(t *testing.T) (*fiber.App, *fakeAuthServiceForRouteTest) {
	t.Helper()
	fake := &fakeAuthServiceForRouteTest{}
	h := handler.NewAuthHandler(fake)
	app := fiber.New()
	app.Post("/api/auth/select-org", h.SelectOrg)
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

// TestSelectOrgHandler_RejectsMissingSessionToken — route phai validate TRUOC khi cham service:
// body thieu session_token (truong bat buoc trong SelectOrgRequestDto) phai bi tu choi 400 va
// service KHONG duoc goi. Neu ai bo mat buoc ValidateStruct, service se nhan session_token rong —
// bai test do o ca hai ve.
func TestSelectOrgHandler_RejectsMissingSessionToken(t *testing.T) {
	app, fake := newSelectOrgOnlyApp(t)

	status := postJSON(t, app, "/api/auth/select-org", `{}`)

	if status != fiber.StatusBadRequest {
		t.Errorf("body thieu session_token: mong doi 400, nhan %d", status)
	}
	if fake.selectOrgCalls != 0 {
		t.Errorf("service duoc goi %d lan voi body khong hop le, mong doi 0", fake.selectOrgCalls)
	}
}

// TestSelectOrgHandler_RejectsMalformedOrganizationID — organization_id phai la UUID hop le
// (validate:"omitempty,uuid"). Mot chuoi rac phai bi chan o tang handler chu khong xuong service,
// neu khong service se nhan gia tri khong parse duoc.
func TestSelectOrgHandler_RejectsMalformedOrganizationID(t *testing.T) {
	app, fake := newSelectOrgOnlyApp(t)

	status := postJSON(t, app, "/api/auth/select-org",
		`{"session_token":"sess-1","organization_id":"khong-phai-uuid"}`)

	if status != fiber.StatusBadRequest {
		t.Errorf("organization_id khong phai UUID: mong doi 400, nhan %d", status)
	}
	if fake.selectOrgCalls != 0 {
		t.Errorf("service duoc goi %d lan voi organization_id khong hop le, mong doi 0", fake.selectOrgCalls)
	}
}

// TestSelectOrgHandler_ReachesServiceWithParsedBody — chot chan hop dong body: web gui
// {session_token, organization_id} dung snake_case nhu SelectOrgRequestDto. Bai test khang dinh
// body duoc parse dung truoc khi xuong service, de mot lan doi json tag se do ngay.
func TestSelectOrgHandler_ReachesServiceWithParsedBody(t *testing.T) {
	app, fake := newSelectOrgOnlyApp(t)

	status := postJSON(t, app, "/api/auth/select-org",
		`{"session_token":"sess-1","organization_id":"11111111-1111-1111-1111-111111111111"}`)

	if status != fiber.StatusOK {
		t.Fatalf("mong doi 200, nhan %d", status)
	}
	if fake.selectOrgCalls != 1 {
		t.Fatalf("service duoc goi %d lan, mong doi 1", fake.selectOrgCalls)
	}
	if fake.lastReq.SessionToken != "sess-1" {
		t.Errorf("SessionToken = %q, mong doi %q", fake.lastReq.SessionToken, "sess-1")
	}
	if fake.lastReq.OrganizationID != "11111111-1111-1111-1111-111111111111" {
		t.Errorf("OrganizationID = %q, mong doi dung uuid da gui", fake.lastReq.OrganizationID)
	}
}

// TestSelectOrgHandler_AllowsEmptyOrganization — organization_id rong la HOP LE (DTO ghi ro:
// rong = che do "Doc lap", active_org=null), nen body chi co session_token phai xuong duoc service
// chu khong bi validation chan. Neu ai them `required` vao OrganizationID, bai test nay do —
// va luong dang nhap cua nguoi khong thuoc to chuc nao se ket thuc bang 400.
func TestSelectOrgHandler_AllowsEmptyOrganization(t *testing.T) {
	app, fake := newSelectOrgOnlyApp(t)

	status := postJSON(t, app, "/api/auth/select-org", `{"session_token":"sess-2"}`)

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
