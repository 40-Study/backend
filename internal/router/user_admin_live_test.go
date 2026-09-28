package router

// Phase 1 quản lý người dùng (2026-09-28) — test "route SỐNG", cùng khuôn với
// select_org_live_test.go: Fiber route THẬT (SetupUserAdminRoutes/SetupAuthRoutes) →
// AuthMiddleware THẬT → PermissionChecker THẬT → UserAdminHandler/AuthHandler THẬT →
// UserAdminService/AuthService THẬT → Redis THẬT (miniredis). Chỉ repository được fake (biên DB).
//
// Bao phủ: (1) 2 quyền USERS_VIEW_ALL/USERS_BAN được kiểm RIÊNG (403 nếu thiếu 1 trong 2);
// (2) khoá bắt buộc reason (400); (3) không tự khoá chính mình (400); (4) khoá thành công set
// đủ 3 field (locked_reason/locked_at/locked_by) VÀ user_version tăng thật trong Redis; (5)
// AuthMiddleware phân biệt "Tài khoản đã bị khoá" (code ACCOUNT_LOCKED) với "All sessions
// revoked" chung chung; (6) Login trả message/code riêng cho tài khoản bị khoá, khác "sai mật
// khẩu"; (7) mở khoá xong đăng nhập lại được (token MỚI được middleware chấp nhận).

import (
	"context"
	"encoding/json"
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
	"study.com/v1/internal/handler"
	"study.com/v1/internal/middleware"
	"study.com/v1/internal/model"
	"study.com/v1/internal/repository"
	"study.com/v1/internal/service"
	"study.com/v1/internal/utils"
)

const adminLiveJWTSecret = "user-admin-live-test-secret"

// ── Repository fakes (biên DB) ──────────────────────────────────────────────

type adminLiveUserRepo struct {
	repository.UserRepositoryInterface
	byID    map[uuid.UUID]*model.User
	byEmail map[string]*model.User
	// order + rolesByUser: chỉ dùng cho AdminListUsers (mô phỏng lọc/phân trang trong bộ nhớ,
	// KHÔNG phải Postgres thật — xem ghi chú ở LockOrUnlockUser).
	order       []uuid.UUID
	rolesByUser map[uuid.UUID][]string
}

func (f *adminLiveUserRepo) FindUserByID(ctx context.Context, id uuid.UUID) (*model.User, error) {
	return f.byID[id], nil
}

func (f *adminLiveUserRepo) FindUserByEmail(ctx context.Context, email string) (*model.User, error) {
	return f.byEmail[email], nil
}

func (f *adminLiveUserRepo) ActiveSystemRoleNamesByUserIDs(ctx context.Context, ids []uuid.UUID) (map[uuid.UUID][]string, error) {
	result := make(map[uuid.UUID][]string, len(ids))
	for _, id := range ids {
		result[id] = f.rolesByUser[id]
	}
	return result, nil
}

// AdminListUsers — lọc/phân trang THUẦN TRONG BỘ NHỚ trên danh sách seed sẵn, chỉ để pin hành
// vi keyword/role/status/page/limit của handler+service; KHÔNG pin ILIKE/JOIN thật của Postgres
// (phần đó là internal/repository/user_repository.go, cần Postgres thật để kiểm — CI Linux).
func (f *adminLiveUserRepo) AdminListUsers(ctx context.Context, filter repository.AdminUserListFilter) ([]model.User, int64, error) {
	var matched []model.User
	for _, id := range f.order {
		u := f.byID[id]
		if filter.Keyword != "" &&
			!strings.Contains(strings.ToLower(u.Email), strings.ToLower(filter.Keyword)) &&
			!strings.Contains(strings.ToLower(u.UserName), strings.ToLower(filter.Keyword)) {
			continue
		}
		if filter.Status == "active" && !u.IsActive {
			continue
		}
		if filter.Status == "locked" && u.IsActive {
			continue
		}
		if filter.Role != "" {
			has := false
			for _, r := range f.rolesByUser[id] {
				if r == filter.Role {
					has = true
					break
				}
			}
			if !has {
				continue
			}
		}
		matched = append(matched, *u)
	}

	total := int64(len(matched))
	page := filter.Page
	if page < 1 {
		page = 1
	}
	limit := filter.Limit
	if limit < 1 {
		limit = 20
	}
	start := (page - 1) * limit
	if start > len(matched) {
		start = len(matched)
	}
	end := start + limit
	if end > len(matched) {
		end = len(matched)
	}
	return matched[start:end], total, nil
}

// LockOrUnlockUser — bản fake ĐƠN GIẢN (không tái hiện transaction/FOR UPDATE thật của
// Postgres — cái đó được pin bằng unit test thuần evaluateLastSystemAdminGuard trong
// internal/repository, và bằng go build/go vet cho phần wiring). Test này pin phần WIRING
// handler → service → repo-interface → set field, không pin cơ chế khoá dòng của Postgres.
func (f *adminLiveUserRepo) LockOrUnlockUser(
	ctx context.Context,
	targetUserID uuid.UUID,
	isActive bool,
	reason *string,
	actorID uuid.UUID,
	systemAdminRoleID uuid.UUID,
) (*model.User, error) {
	u, ok := f.byID[targetUserID]
	if !ok {
		return nil, repository.ErrAdminUserNotFound
	}
	if !isActive {
		u.IsActive = false
		u.LockedReason = reason
		now := time.Now()
		u.LockedAt = &now
		u.LockedBy = &actorID
	} else {
		u.IsActive = true
		u.LockedReason = nil
		u.LockedAt = nil
		u.LockedBy = nil
	}
	return u, nil
}

type adminLiveSystemRoleRepo struct {
	repository.SystemRoleRepositoryInterface
	byName        map[string]*model.SystemRole
	permsByRoleID map[uuid.UUID][]string
}

func (f *adminLiveSystemRoleRepo) GetSystemRoleByName(ctx context.Context, name string) (*model.SystemRole, error) {
	return f.byName[name], nil
}

func (f *adminLiveSystemRoleRepo) GetPermissionsBySystemRoleID(ctx context.Context, roleID uuid.UUID) ([]model.Permission, error) {
	var out []model.Permission
	for _, n := range f.permsByRoleID[roleID] {
		out = append(out, model.Permission{Name: n})
	}
	return out, nil
}

// adminLiveUserOrgRoleRepo — fake rỗng: Login (đường thành công, sau khi mở khoá) gọi tới
// buildUnifiedRoles -> FindByUserIDWithDetails; test này không có org role nào, chỉ cần
// không panic trên nil interface.
type adminLiveUserOrgRoleRepo struct {
	repository.UserOrganizationRoleRepositoryInterface
}

func (f *adminLiveUserOrgRoleRepo) FindByUserIDWithDetails(ctx context.Context, userID uuid.UUID, status string) ([]model.UserOrganizationRole, error) {
	return nil, nil
}

type adminLiveUserSystemRoleRepo struct {
	repository.UserSystemRoleRepositoryInterface
	byUserID map[uuid.UUID][]model.UserSystemRole
}

func (f *adminLiveUserSystemRoleRepo) FindByUserID(ctx context.Context, userID uuid.UUID, status string) ([]model.UserSystemRole, error) {
	return f.byUserID[userID], nil
}

func (f *adminLiveUserSystemRoleRepo) FindByUserIDWithDetails(ctx context.Context, userID uuid.UUID, status string) ([]model.UserSystemRole, error) {
	return f.byUserID[userID], nil
}

// ── Môi trường test ─────────────────────────────────────────────────────────

type adminLiveEnv struct {
	app         *fiber.App
	cfg         *config.Config
	rdb         *redis.Client
	mr          *miniredis.Miniredis
	userRepo    *adminLiveUserRepo
	adminID     uuid.UUID
	viewOnlyID  uuid.UUID
	targetID    uuid.UUID
	adminToken  string
	viewToken   string
	targetToken string
}

func newAdminLiveEnv(t *testing.T) *adminLiveEnv {
	t.Helper()

	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = rdb.Close() })

	cfg := &config.Config{
		JWTSecret:            adminLiveJWTSecret,
		JWTAccessExpiration:  15 * time.Minute,
		JWTRefreshExpiration: 7 * 24 * time.Hour,
	}

	adminID := uuid.New()
	viewOnlyID := uuid.New()
	targetID := uuid.New()
	deviceID := uuid.New()

	passwordHash, err := utils.HashPassword("Test@12345")
	if err != nil {
		t.Fatalf("hash password loi: %v", err)
	}

	adminUser := &model.User{Email: "admin@demo.com", UserName: "admin", IsActive: true, PasswordHash: passwordHash}
	adminUser.ID = adminID
	viewOnlyUser := &model.User{Email: "vieweronly@demo.com", UserName: "vieweronly", IsActive: true, PasswordHash: passwordHash}
	viewOnlyUser.ID = viewOnlyID
	targetUser := &model.User{Email: "student2@demo.com", UserName: "student2", IsActive: true, PasswordHash: passwordHash}
	targetUser.ID = targetID

	// Thêm 1 user khoá sẵn + vai trò khác, riêng cho test lọc keyword/role/status/phân trang
	// GET /users (không cần thao tác qua nó, chỉ cần nó tồn tại trong danh sách).
	lockedTeacherID := uuid.New()
	lockedReasonSeed := "Vi phạm từ trước"
	lockedTeacher := &model.User{Email: "teacher1@demo.com", UserName: "teacher1", IsActive: false, PasswordHash: passwordHash, LockedReason: &lockedReasonSeed}
	lockedTeacher.ID = lockedTeacherID

	userRepo := &adminLiveUserRepo{
		byID: map[uuid.UUID]*model.User{
			adminID:         adminUser,
			viewOnlyID:      viewOnlyUser,
			targetID:        targetUser,
			lockedTeacherID: lockedTeacher,
		},
		byEmail: map[string]*model.User{
			adminUser.Email:    adminUser,
			viewOnlyUser.Email: viewOnlyUser,
			targetUser.Email:   targetUser,
			lockedTeacher.Email: lockedTeacher,
		},
		order: []uuid.UUID{adminID, viewOnlyID, targetID, lockedTeacherID},
		rolesByUser: map[uuid.UUID][]string{
			adminID:         {"SYSTEM_ADMIN"},
			viewOnlyID:      {"SYSTEM_ADMIN"},
			targetID:        {"STUDENT"},
			lockedTeacherID: {"TEACHER"},
		},
	}

	adminRoleID := uuid.New()   // vai trò cấp CẢ USERS_VIEW_ALL + USERS_BAN
	viewOnlyRoleID := uuid.New() // vai trò cấp CHỈ USERS_VIEW_ALL
	systemRoleRepo := &adminLiveSystemRoleRepo{
		byName: map[string]*model.SystemRole{
			"SYSTEM_ADMIN": {Name: "SYSTEM_ADMIN"},
		},
		permsByRoleID: map[uuid.UUID][]string{
			adminRoleID:    {"USERS_VIEW_ALL", "USERS_BAN"},
			viewOnlyRoleID: {"USERS_VIEW_ALL"},
		},
	}

	userSystemRoleRepo := &adminLiveUserSystemRoleRepo{
		byUserID: map[uuid.UUID][]model.UserSystemRole{
			adminID:    {{UserID: adminID, SystemRoleID: adminRoleID, Status: model.UserSystemRoleStatusActive}},
			viewOnlyID: {{UserID: viewOnlyID, SystemRoleID: viewOnlyRoleID, Status: model.UserSystemRoleStatusActive}},
		},
	}

	authSvc := service.NewAuthService(cfg, userRepo, nil, &adminLiveUserOrgRoleRepo{}, userSystemRoleRepo, systemRoleRepo, nil, rdb)
	adminSvc := service.NewUserAdminService(userRepo, systemRoleRepo, userSystemRoleRepo, authSvc, rdb)
	adminHandler := handler.NewUserAdminHandler(adminSvc)
	pc := middleware.NewPermissionChecker(userSystemRoleRepo, systemRoleRepo, nil, nil)

	ctx := context.Background()
	const userVer = int64(1)
	for _, id := range []uuid.UUID{adminID, viewOnlyID, targetID} {
		if err := rdb.Set(ctx, constants.KeyUserVersion(id.String()), userVer, 0).Err(); err != nil {
			t.Fatalf("khong seed duoc user_version: %v", err)
		}
	}

	adminToken, _, err := utils.GenerateTokens(cfg, adminID, deviceID, "SYSTEM_ADMIN", nil, userVer)
	if err != nil {
		t.Fatalf("khong sinh duoc admin token: %v", err)
	}
	viewToken, _, err := utils.GenerateTokens(cfg, viewOnlyID, deviceID, "SYSTEM_ADMIN", nil, userVer)
	if err != nil {
		t.Fatalf("khong sinh duoc view-only token: %v", err)
	}
	targetToken, _, err := utils.GenerateTokens(cfg, targetID, deviceID, "STUDENT", nil, userVer)
	if err != nil {
		t.Fatalf("khong sinh duoc target token: %v", err)
	}

	app := fiber.New()
	api := app.Group("/api")
	SetupUserAdminRoutes(api, cfg, adminHandler, rdb, pc)
	SetupAuthRoutes(api, cfg, handler.NewAuthHandler(authSvc), nil, rdb, nil)
	// Route bảo vệ tối thiểu để quan sát trực tiếp phản ứng của AuthMiddleware (không đi qua
	// permission nào) — mô phỏng "request kế tiếp của student2" tới BẤT KỲ route nào cần đăng
	// nhập, không riêng gì route quản trị.
	api.Get("/protected-probe", middleware.AuthMiddleware(cfg, rdb), func(c *fiber.Ctx) error {
		return c.SendStatus(fiber.StatusOK)
	})

	return &adminLiveEnv{
		app:         app,
		cfg:         cfg,
		rdb:         rdb,
		mr:          mr,
		userRepo:    userRepo,
		adminID:     adminID,
		viewOnlyID:  viewOnlyID,
		targetID:    targetID,
		adminToken:  adminToken,
		viewToken:   viewToken,
		targetToken: targetToken,
	}
}

func (e *adminLiveEnv) putStatus(t *testing.T, token, targetID, body string) (int, map[string]interface{}) {
	t.Helper()
	req := httptest.NewRequest("PUT", "/api/users/"+targetID+"/status", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := e.app.Test(req)
	if err != nil {
		t.Fatalf("app.Test loi: %v", err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	var parsed map[string]interface{}
	_ = json.Unmarshal(raw, &parsed)
	return resp.StatusCode, parsed
}

func (e *adminLiveEnv) getWithToken(t *testing.T, path, token string) int {
	t.Helper()
	req := httptest.NewRequest("GET", path, nil)
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := e.app.Test(req)
	if err != nil {
		t.Fatalf("app.Test loi: %v", err)
	}
	defer resp.Body.Close()
	return resp.StatusCode
}

func (e *adminLiveEnv) getUsersList(t *testing.T, query string) (int, map[string]interface{}) {
	t.Helper()
	req := httptest.NewRequest("GET", "/api/users"+query, nil)
	req.Header.Set("Authorization", "Bearer "+e.adminToken)
	resp, err := e.app.Test(req)
	if err != nil {
		t.Fatalf("app.Test loi: %v", err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	var parsed map[string]interface{}
	_ = json.Unmarshal(raw, &parsed)
	return resp.StatusCode, parsed
}

func (e *adminLiveEnv) userVersion(t *testing.T, userID uuid.UUID) int64 {
	t.Helper()
	v, err := e.rdb.Get(context.Background(), constants.KeyUserVersion(userID.String())).Int64()
	if err != nil {
		t.Fatalf("khong doc duoc user_version: %v", err)
	}
	return v
}

// ── Tests: permission gating (2 quyền kiểm RIÊNG) ───────────────────────────

func TestUserAdminList_KhongCoUsersViewAll_Tra403(t *testing.T) {
	e := newAdminLiveEnv(t)
	// targetToken thuộc student2 — không hề có USERS_VIEW_ALL.
	status := e.getWithToken(t, "/api/users", e.targetToken)
	if status != fiber.StatusForbidden {
		t.Fatalf("muon 403 (thieu USERS_VIEW_ALL), duoc %d", status)
	}
}

func TestUserAdminUpdateStatus_CoViewAllNhungThieuUsersBan_Tra403(t *testing.T) {
	e := newAdminLiveEnv(t)
	// viewToken CÓ USERS_VIEW_ALL nhưng KHÔNG có USERS_BAN — chứng minh 2 quyền tách biệt,
	// không gộp chung 1 permission như phase-01 yêu cầu.
	status, _ := e.putStatus(t, e.viewToken, e.targetID.String(), `{"is_active":false,"reason":"vi pham"}`)
	if status != fiber.StatusForbidden {
		t.Fatalf("muon 403 (co VIEW_ALL nhung thieu BAN), duoc %d", status)
	}
}

// ── Tests: khoá tài khoản ────────────────────────────────────────────────────

func TestUserAdminUpdateStatus_ThieuReason_Tra400(t *testing.T) {
	e := newAdminLiveEnv(t)
	status, body := e.putStatus(t, e.adminToken, e.targetID.String(), `{"is_active":false}`)
	if status != fiber.StatusBadRequest {
		t.Fatalf("muon 400 (thieu reason), duoc %d: %v", status, body)
	}
}

func TestUserAdminUpdateStatus_TuKhoaChinhMinh_Tra400(t *testing.T) {
	e := newAdminLiveEnv(t)
	status, body := e.putStatus(t, e.adminToken, e.adminID.String(), `{"is_active":false,"reason":"vi pham"}`)
	if status != fiber.StatusBadRequest {
		t.Fatalf("muon 400 (tu khoa chinh minh), duoc %d: %v", status, body)
	}
}

func TestUserAdminUpdateStatus_KhoaThanhCong_SetDu3FieldVaTangUserVersion(t *testing.T) {
	e := newAdminLiveEnv(t)
	verBefore := e.userVersion(t, e.targetID)

	status, body := e.putStatus(t, e.adminToken, e.targetID.String(), `{"is_active":false,"reason":"Vi pham dieu khoan"}`)
	if status != fiber.StatusOK {
		t.Fatalf("muon 200, duoc %d: %v", status, body)
	}

	data, ok := body["data"].(map[string]interface{})
	if !ok {
		t.Fatalf("response thieu data: %v", body)
	}
	if data["is_active"] != false {
		t.Errorf("is_active = %v, muon false", data["is_active"])
	}
	if data["locked_reason"] != "Vi pham dieu khoan" {
		t.Errorf("locked_reason = %v, muon 'Vi pham dieu khoan'", data["locked_reason"])
	}
	if data["locked_at"] == nil || data["locked_at"] == "" {
		t.Errorf("locked_at rong, muon co gia tri")
	}

	verAfter := e.userVersion(t, e.targetID)
	if verAfter <= verBefore {
		t.Fatalf("user_version khong tang: truoc=%d sau=%d (RevokeAllSessions khong duoc goi?)", verBefore, verAfter)
	}
}

// ── Tests: AuthMiddleware phân biệt lý do bị đá ra ──────────────────────────

func TestProtectedRoute_SauKhiBiKhoa_Tra401VoiCodeAccountLocked(t *testing.T) {
	e := newAdminLiveEnv(t)

	// Request BÌNH THƯỜNG trước khi khoá — phải qua được.
	if status := e.getWithToken(t, "/api/protected-probe", e.targetToken); status != fiber.StatusOK {
		t.Fatalf("truoc khi khoa muon 200, duoc %d", status)
	}

	status, _ := e.putStatus(t, e.adminToken, e.targetID.String(), `{"is_active":false,"reason":"vi pham"}`)
	if status != fiber.StatusOK {
		t.Fatalf("khoa that bai, status=%d", status)
	}

	// Token CŨ của student2 (chưa hết hạn) phải bị từ chối NGAY — không đợi token tự hết hạn.
	req := httptest.NewRequest("GET", "/api/protected-probe", nil)
	req.Header.Set("Authorization", "Bearer "+e.targetToken)
	resp, err := e.app.Test(req)
	if err != nil {
		t.Fatalf("app.Test loi: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != fiber.StatusUnauthorized {
		t.Fatalf("muon 401 sau khi bi khoa, duoc %d", resp.StatusCode)
	}
	raw, _ := io.ReadAll(resp.Body)
	var parsed map[string]interface{}
	_ = json.Unmarshal(raw, &parsed)
	if parsed["code"] != "ACCOUNT_LOCKED" {
		t.Errorf(`code = %v, muon "ACCOUNT_LOCKED" (khong duoc lan voi "All sessions revoked" chung chung) - body: %s`, parsed["code"], raw)
	}
	if parsed["message"] != "Tài khoản đã bị khoá" {
		t.Errorf(`message = %v, muon "Tài khoản đã bị khoá"`, parsed["message"])
	}
}

func TestLogin_TaiKhoanBiKhoa_TraMessageRiengKhongPhaiSaiMatKhau(t *testing.T) {
	e := newAdminLiveEnv(t)

	status, _ := e.putStatus(t, e.adminToken, e.targetID.String(), `{"is_active":false,"reason":"vi pham"}`)
	if status != fiber.StatusOK {
		t.Fatalf("khoa that bai, status=%d", status)
	}

	body := `{"email":"student2@demo.com","password":"Test@12345","device_info":{"device_id":"` +
		uuid.New().String() + `","device_name":"Test Device","os":"TestOS"}}`
	req := httptest.NewRequest("POST", "/api/auth/login", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp, err := e.app.Test(req)
	if err != nil {
		t.Fatalf("app.Test loi: %v", err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	var parsed map[string]interface{}
	_ = json.Unmarshal(raw, &parsed)

	if resp.StatusCode != fiber.StatusUnauthorized {
		t.Fatalf("muon 401, duoc %d: %s", resp.StatusCode, raw)
	}
	if parsed["code"] != "ACCOUNT_LOCKED" {
		t.Errorf(`code = %v, muon "ACCOUNT_LOCKED"`, parsed["code"])
	}
	if parsed["message"] != "Tài khoản đã bị khoá" {
		t.Errorf(`message = %v, muon "Tài khoản đã bị khoá" (KHONG duoc la thong bao sai mat khau)`, parsed["message"])
	}
}

func TestUserAdminUpdateStatus_MoKhoaXongDangNhapLaiDuoc(t *testing.T) {
	e := newAdminLiveEnv(t)

	status, _ := e.putStatus(t, e.adminToken, e.targetID.String(), `{"is_active":false,"reason":"vi pham"}`)
	if status != fiber.StatusOK {
		t.Fatalf("khoa that bai, status=%d", status)
	}

	status, body := e.putStatus(t, e.adminToken, e.targetID.String(), `{"is_active":true}`)
	if status != fiber.StatusOK {
		t.Fatalf("mo khoa that bai, status=%d: %v", status, body)
	}
	data := body["data"].(map[string]interface{})
	if data["is_active"] != true {
		t.Errorf("is_active = %v, muon true sau khi mo khoa", data["is_active"])
	}
	if data["locked_reason"] != nil {
		t.Errorf("locked_reason = %v, muon nil sau khi mo khoa", data["locked_reason"])
	}

	loginBody := `{"email":"student2@demo.com","password":"Test@12345","device_info":{"device_id":"` +
		uuid.New().String() + `","device_name":"Test Device","os":"TestOS"}}`
	req := httptest.NewRequest("POST", "/api/auth/login", strings.NewReader(loginBody))
	req.Header.Set("Content-Type", "application/json")
	resp, err := e.app.Test(req)
	if err != nil {
		t.Fatalf("app.Test loi: %v", err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != fiber.StatusOK {
		t.Fatalf("muon 200 (dang nhap lai duoc sau khi mo khoa), duoc %d: %s", resp.StatusCode, raw)
	}
}

// ── Tests: GET /users lọc theo keyword/role/status + phân trang ────────────
// Seed: admin(SYSTEM_ADMIN,active) · vieweronly(SYSTEM_ADMIN,active) ·
// student2(STUDENT,active) · teacher1(TEACHER,locked).

func TestUserAdminList_LocTheoKeyword(t *testing.T) {
	e := newAdminLiveEnv(t)
	status, body := e.getUsersList(t, "?keyword=student2")
	if status != fiber.StatusOK {
		t.Fatalf("muon 200, duoc %d", status)
	}
	data := body["data"].(map[string]interface{})
	items := data["items"].([]interface{})
	if len(items) != 1 {
		t.Fatalf("muon 1 ket qua khop keyword=student2, duoc %d", len(items))
	}
	if items[0].(map[string]interface{})["email"] != "student2@demo.com" {
		t.Errorf("email = %v, muon student2@demo.com", items[0].(map[string]interface{})["email"])
	}
}

func TestUserAdminList_LocTheoRole(t *testing.T) {
	e := newAdminLiveEnv(t)
	status, body := e.getUsersList(t, "?role=TEACHER")
	if status != fiber.StatusOK {
		t.Fatalf("muon 200, duoc %d", status)
	}
	data := body["data"].(map[string]interface{})
	items := data["items"].([]interface{})
	if len(items) != 1 || items[0].(map[string]interface{})["email"] != "teacher1@demo.com" {
		t.Fatalf("loc role=TEACHER phai chi tra teacher1@demo.com, duoc: %v", items)
	}
}

func TestUserAdminList_LocTheoStatusLocked(t *testing.T) {
	e := newAdminLiveEnv(t)
	status, body := e.getUsersList(t, "?status=locked")
	if status != fiber.StatusOK {
		t.Fatalf("muon 200, duoc %d", status)
	}
	data := body["data"].(map[string]interface{})
	items := data["items"].([]interface{})
	if len(items) != 1 || items[0].(map[string]interface{})["email"] != "teacher1@demo.com" {
		t.Fatalf("loc status=locked phai chi tra teacher1@demo.com (nguoi duy nhat is_active=false), duoc: %v", items)
	}
}

func TestUserAdminList_PhanTrang(t *testing.T) {
	e := newAdminLiveEnv(t)
	status, body := e.getUsersList(t, "?page=1&limit=2")
	if status != fiber.StatusOK {
		t.Fatalf("muon 200, duoc %d", status)
	}
	data := body["data"].(map[string]interface{})
	items := data["items"].([]interface{})
	if len(items) != 2 {
		t.Fatalf("limit=2 phai tra dung 2 item trang 1, duoc %d", len(items))
	}
	if int(data["total_count"].(float64)) != 4 {
		t.Errorf("total_count = %v, muon 4 (tong so user seed)", data["total_count"])
	}
	if int(data["total_pages"].(float64)) != 2 {
		t.Errorf("total_pages = %v, muon 2 (4 user / limit 2)", data["total_pages"])
	}

	status2, body2 := e.getUsersList(t, "?page=2&limit=2")
	if status2 != fiber.StatusOK {
		t.Fatalf("muon 200 o trang 2, duoc %d", status2)
	}
	items2 := body2["data"].(map[string]interface{})["items"].([]interface{})
	if len(items2) != 2 {
		t.Fatalf("trang 2 phai con 2 item, duoc %d", len(items2))
	}
}

func TestUserAdminList_StatusKhongHopLe_Tra400(t *testing.T) {
	e := newAdminLiveEnv(t)
	status, body := e.getUsersList(t, "?status=banned")
	if status != fiber.StatusBadRequest {
		t.Fatalf("muon 400 (status khong hop le), duoc %d: %v", status, body)
	}
}
