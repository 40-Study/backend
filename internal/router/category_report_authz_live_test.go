package router

// Test song cho A-P0-1 (report) va A-P0-2 (category/tag) — QA 260927.
//
// Truoc ban va: SetupCategoryRoutes/SetupReportRoutes chi gan `auth`, khong gan permChecker, nen
// BAT KY user da dang nhap nao (ke ca STUDENT) cung sua/xoa duoc danh muc/tag he thong va kiem
// duyet (liet ke tat ca/doi trang thai/xoa) report cua nguoi khac — xac nhan song bang curl trong
// verify-260927-student-admin.md (student1 PUT category seed -> 200, POST tag -> 201).
//
// Test nay dung THAT permChecker (khong mock RequirePermissions) voi fake repo toi thieu — chi
// cai dung 2 method ma resolvePermissions/hasWildcard can, cung pattern voi
// internal/middleware/my_permissions_test.go — de mutation "bo permChecker khoi router" hoac
// "doi permission name" deu lam test nay do (403 -> 200/201, hoac nguoc lai).

import (
	"context"
	"io"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"study.com/v1/internal/config"
	"study.com/v1/internal/constants"
	"study.com/v1/internal/dto"
	"study.com/v1/internal/handler"
	"study.com/v1/internal/middleware"
	"study.com/v1/internal/model"
	"study.com/v1/internal/repository"
	"study.com/v1/internal/utils"
)

const catRepAuthzTestSecret = "cat-rep-authz-live-test-secret"

// ---- fake repo toi thieu cho PermissionChecker (chi resolvePermissions/RequirePermissions can) ----

type fakeUSRRepoCR struct {
	repository.UserSystemRoleRepositoryInterface
	systemRoleID uuid.UUID
}

func (f *fakeUSRRepoCR) FindByUserID(ctx context.Context, userID uuid.UUID, status string) ([]model.UserSystemRole, error) {
	return []model.UserSystemRole{{SystemRoleID: f.systemRoleID}}, nil
}

type fakeSRRepoCR struct {
	repository.SystemRoleRepositoryInterface
	perms map[uuid.UUID][]string
}

func (f *fakeSRRepoCR) GetPermissionsBySystemRoleID(ctx context.Context, roleID uuid.UUID) ([]model.Permission, error) {
	var out []model.Permission
	for _, n := range f.perms[roleID] {
		out = append(out, model.Permission{Name: n})
	}
	return out, nil
}

// ---- fake service toi thieu — chi ghi nhan co bi goi khong ----

type fakeCategorySvcCR struct{ called bool }

func (f *fakeCategorySvcCR) CreateCategory(ctx context.Context, req dto.CreateCategoryDTO) (*dto.CategoryResponseDTO, error) {
	f.called = true
	return &dto.CategoryResponseDTO{}, nil
}
func (f *fakeCategorySvcCR) GetAllCategories(ctx context.Context, keyword string) (*dto.CategoryListResponseDTO, error) {
	return &dto.CategoryListResponseDTO{}, nil
}
func (f *fakeCategorySvcCR) GetCategoryByID(ctx context.Context, id uuid.UUID) (*dto.CategoryResponseDTO, error) {
	return &dto.CategoryResponseDTO{}, nil
}
func (f *fakeCategorySvcCR) UpdateCategory(ctx context.Context, id uuid.UUID, req dto.UpdateCategoryDTO) (*dto.CategoryResponseDTO, error) {
	f.called = true
	return &dto.CategoryResponseDTO{}, nil
}
func (f *fakeCategorySvcCR) DeleteCategory(ctx context.Context, id uuid.UUID) error {
	f.called = true
	return nil
}

type fakeTagSvcCR struct{ called bool }

func (f *fakeTagSvcCR) CreateTag(ctx context.Context, req dto.CreateTagDTO) (*dto.TagResponseDTO, error) {
	f.called = true
	return &dto.TagResponseDTO{}, nil
}
func (f *fakeTagSvcCR) GetAllTags(ctx context.Context, page, pageSize int, keyword string) (*dto.TagListResponseDTO, error) {
	return &dto.TagListResponseDTO{}, nil
}
func (f *fakeTagSvcCR) GetTagByID(ctx context.Context, id uuid.UUID) (*dto.TagResponseDTO, error) {
	return &dto.TagResponseDTO{}, nil
}
func (f *fakeTagSvcCR) UpdateTag(ctx context.Context, id uuid.UUID, req dto.UpdateTagDTO) (*dto.TagResponseDTO, error) {
	f.called = true
	return &dto.TagResponseDTO{}, nil
}
func (f *fakeTagSvcCR) DeleteTag(ctx context.Context, id uuid.UUID) error {
	f.called = true
	return nil
}

type fakeReportSvcCR struct{ called bool }

func (f *fakeReportSvcCR) CreateReport(ctx context.Context, reporterID uuid.UUID, req dto.CreateReportDTO) (*dto.ReportResponseDTO, error) {
	f.called = true
	return &dto.ReportResponseDTO{}, nil
}
func (f *fakeReportSvcCR) GetReportByID(ctx context.Context, id uuid.UUID) (*dto.ReportResponseDTO, error) {
	return &dto.ReportResponseDTO{}, nil
}
func (f *fakeReportSvcCR) ListReports(ctx context.Context, status, reportedType string, page, pageSize int) (*dto.ReportListDTO, error) {
	f.called = true
	return &dto.ReportListDTO{}, nil
}
func (f *fakeReportSvcCR) UpdateReportStatus(ctx context.Context, id, adminID uuid.UUID, req dto.UpdateReportStatusDTO) (*dto.ReportResponseDTO, error) {
	f.called = true
	return &dto.ReportResponseDTO{}, nil
}
func (f *fakeReportSvcCR) DeleteReport(ctx context.Context, id uuid.UUID) error {
	f.called = true
	return nil
}
func (f *fakeReportSvcCR) GetMyReports(ctx context.Context, reporterID uuid.UUID, page, pageSize int) (*dto.ReportListDTO, error) {
	f.called = true
	return &dto.ReportListDTO{}, nil
}

// ---- harness ----

type catRepAuthzEnv struct {
	app         *fiber.App
	token       string
	categorySvc *fakeCategorySvcCR
	tagSvc      *fakeTagSvcCR
	reportSvc   *fakeReportSvcCR
}

// newCatRepAuthzEnv dung mot mo hinh quyen: "STUDENT" (rong, khong co CATEGORIES_SYSTEM_MANAGE/
// REPORTS_MODERATE) hoac "SYSTEM_ADMIN" (wildcard "*", vuot qua moi permission — dung cach seeder
// that mo rong role nay, xem seeder.go SeedRoles).
func newCatRepAuthzEnv(t *testing.T, roleName string) *catRepAuthzEnv {
	t.Helper()

	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = rdb.Close() })

	cfg := &config.Config{
		JWTSecret:            catRepAuthzTestSecret,
		JWTAccessExpiration:  15 * time.Minute,
		JWTRefreshExpiration: 7 * 24 * time.Hour,
	}

	userID := uuid.New()
	deviceID := uuid.New()
	sysRoleID := uuid.New()
	const userVer = int64(1)

	ctx := context.Background()
	if err := rdb.Set(ctx, constants.KeyUserVersion(userID.String()), userVer, 0).Err(); err != nil {
		t.Fatalf("khong seed duoc user_version: %v", err)
	}

	perms := map[uuid.UUID][]string{}
	switch roleName {
	case "SYSTEM_ADMIN":
		perms[sysRoleID] = []string{"*"}
	case "STUDENT":
		perms[sysRoleID] = []string{}
	default:
		t.Fatalf("roleName khong ho tro trong test nay: %s", roleName)
	}

	permChecker := middleware.NewPermissionChecker(
		&fakeUSRRepoCR{systemRoleID: sysRoleID},
		&fakeSRRepoCR{perms: perms},
		nil, nil,
	)

	accessToken, _, err := utils.GenerateTokens(cfg, userID, deviceID, roleName, nil, userVer)
	if err != nil {
		t.Fatalf("khong sinh duoc access token: %v", err)
	}

	app := fiber.New()
	api := app.Group("/api")

	categorySvc := &fakeCategorySvcCR{}
	tagSvc := &fakeTagSvcCR{}
	reportSvc := &fakeReportSvcCR{}

	SetupCategoryRoutes(api, cfg, handler.NewCategoryHandler(categorySvc), handler.NewTagHandler(tagSvc), rdb, permChecker)
	SetupReportRoutes(api, cfg, handler.NewReportHandler(reportSvc), rdb, permChecker)

	return &catRepAuthzEnv{app: app, token: accessToken, categorySvc: categorySvc, tagSvc: tagSvc, reportSvc: reportSvc}
}

func (e *catRepAuthzEnv) do(t *testing.T, method, path, body string) (int, []byte) {
	t.Helper()
	var reader io.Reader
	if body != "" {
		reader = strings.NewReader(body)
	}
	req := httptest.NewRequest(method, path, reader)
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set("Authorization", "Bearer "+e.token)
	resp, err := e.app.Test(req)
	if err != nil {
		t.Fatalf("app.Test loi: %v", err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, raw
}

// ---- A-P0-2: category/tag CRUD chi admin ----

func TestCategoryTagWrite_Student_BiTuChoi403(t *testing.T) {
	env := newCatRepAuthzEnv(t, "STUDENT")

	cases := []struct {
		method, path, body string
	}{
		{"POST", "/api/categories/", `{"name":"QA-cat"}`},
		{"PUT", "/api/categories/" + uuid.New().String(), `{"name":"QA-cat"}`},
		{"DELETE", "/api/categories/" + uuid.New().String(), ""},
		{"POST", "/api/tags/", `{"name":"QA-tag"}`},
		{"PUT", "/api/tags/" + uuid.New().String(), `{"name":"QA-tag"}`},
		{"DELETE", "/api/tags/" + uuid.New().String(), ""},
	}
	for _, c := range cases {
		status, raw := env.do(t, c.method, c.path, c.body)
		if status != fiber.StatusForbidden {
			t.Errorf("%s %s: status = %d, muon 403 (STUDENT khong co CATEGORIES_SYSTEM_MANAGE), body: %s", c.method, c.path, status, raw)
		}
	}
	if env.categorySvc.called || env.tagSvc.called {
		t.Error("service Category/Tag bi goi du bi tu choi quyen — permChecker khong chan truoc handler")
	}
}

func TestCategoryTagWrite_SystemAdmin_DuocPhep(t *testing.T) {
	env := newCatRepAuthzEnv(t, "SYSTEM_ADMIN")

	status, raw := env.do(t, "POST", "/api/categories/", `{"name":"QA-cat"}`)
	if status == fiber.StatusForbidden {
		t.Fatalf("SYSTEM_ADMIN (wildcard *) van bi 403 tao category: %s", raw)
	}
	if !env.categorySvc.called {
		t.Error("service.CreateCategory khong duoc goi du SYSTEM_ADMIN da qua permChecker")
	}

	status, raw = env.do(t, "POST", "/api/tags/", `{"name":"QA-tag"}`)
	if status == fiber.StatusForbidden {
		t.Fatalf("SYSTEM_ADMIN (wildcard *) van bi 403 tao tag: %s", raw)
	}
	if !env.tagSvc.called {
		t.Error("service.CreateTag khong duoc goi du SYSTEM_ADMIN da qua permChecker")
	}
}

func TestCategoryRead_KhongCanAuth(t *testing.T) {
	env := newCatRepAuthzEnv(t, "STUDENT")
	req := httptest.NewRequest("GET", "/api/categories/", nil)
	// KHONG gan Authorization — GET phai van public.
	resp, err := env.app.Test(req)
	if err != nil {
		t.Fatalf("app.Test loi: %v", err)
	}
	if resp.StatusCode == fiber.StatusUnauthorized || resp.StatusCode == fiber.StatusForbidden {
		t.Fatalf("GET /categories khong con public sau ban va — status = %d", resp.StatusCode)
	}
}

// ---- A-P0-1: report — kiem duyet chi admin he thong ----

func TestReportModeration_Student_BiTuChoi403(t *testing.T) {
	env := newCatRepAuthzEnv(t, "STUDENT")

	cases := []struct {
		method, path, body string
	}{
		{"GET", "/api/reports/", ""},
		{"PUT", "/api/reports/" + uuid.New().String() + "/status", `{"status":"resolved"}`},
		{"DELETE", "/api/reports/" + uuid.New().String(), ""},
	}
	for _, c := range cases {
		status, raw := env.do(t, c.method, c.path, c.body)
		if status != fiber.StatusForbidden {
			t.Errorf("%s %s: status = %d, muon 403 (STUDENT khong co REPORTS_MODERATE), body: %s", c.method, c.path, status, raw)
		}
	}
	if env.reportSvc.called {
		t.Error("service Report (moderation) bi goi du bi tu choi quyen")
	}
}

// Tao report va xem report cua CHINH MINH van phai mo cho moi user da dang nhap — khong bi anh
// huong boi permChecker moi gan (chi gate ListReports/UpdateReportStatus/DeleteReport).
func TestReportCreateAndMy_Student_VanDuocPhep(t *testing.T) {
	env := newCatRepAuthzEnv(t, "STUDENT")

	status, raw := env.do(t, "POST", "/api/reports/", `{"reported_type":"course","reported_id":"`+uuid.New().String()+`","reason":"spam"}`)
	if status == fiber.StatusForbidden {
		t.Fatalf("POST /reports (tao report) bi 403 cho STUDENT — khong duoc gate: %s", raw)
	}

	status, raw = env.do(t, "GET", "/api/reports/my", "")
	if status == fiber.StatusForbidden {
		t.Fatalf("GET /reports/my bi 403 cho STUDENT — khong duoc gate: %s", raw)
	}
}

func TestReportModeration_SystemAdmin_DuocPhep(t *testing.T) {
	env := newCatRepAuthzEnv(t, "SYSTEM_ADMIN")

	status, raw := env.do(t, "GET", "/api/reports/", "")
	if status == fiber.StatusForbidden {
		t.Fatalf("SYSTEM_ADMIN (wildcard *) van bi 403 liet ke reports: %s", raw)
	}
	if !env.reportSvc.called {
		t.Error("service.ListReports khong duoc goi du SYSTEM_ADMIN da qua permChecker")
	}
}
