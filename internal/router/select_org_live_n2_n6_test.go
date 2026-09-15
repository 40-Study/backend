package router

// Test cho N2 va N6 (review vong 2, 260915) — hai lo hong ma select_org_live_test.go KHONG phu
// duoc vi luon seed mot system role KHOP DUNG voi active_role trong token (xem nhan xet cuoi
// review vong 2: "khong test nao phu N2 ... nhanh khong-khop chua bao gio chay"). Hai file nay
// deu dung chung cac fake repo (liveUserRepo, liveUserOrgRoleRepo, liveUserSystemRoleRepo) da
// khai bao trong select_org_live_test.go, cung package "router".

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
	"study.com/v1/internal/handler"
	"study.com/v1/internal/model"
	"study.com/v1/internal/service"
	"study.com/v1/internal/utils"
)

// n2n6TestEnv: ban rut gon cua selectOrgLiveEnv, cho phep TUY CHINH activeRole trong token, danh
// sach system role duoc seed, va user.IsActive — ba truc ma newSelectOrgLiveEnv co dinh san.
type n2n6TestEnv struct {
	app      *fiber.App
	rdb      *redis.Client
	mr       *miniredis.Miniredis
	userID   uuid.UUID
	deviceID uuid.UUID
	token    string
}

func newN2N6TestEnv(t *testing.T, tokenActiveRole string, seededSystemRoles []model.UserSystemRole, userIsActive bool) *n2n6TestEnv {
	t.Helper()

	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = rdb.Close() })

	cfg := &config.Config{
		JWTSecret:            selectOrgTestJWTSecret,
		JWTAccessExpiration:  15 * time.Minute,
		JWTRefreshExpiration: 7 * 24 * time.Hour,
	}

	userID := uuid.New()
	deviceID := uuid.New()

	user := &model.User{UserName: "giaovien", Email: "gv@example.com", IsActive: userIsActive}
	user.ID = userID

	svc := service.NewAuthService(
		cfg,
		&liveUserRepo{user: user},
		nil,
		&liveUserOrgRoleRepo{rows: nil},
		&liveUserSystemRoleRepo{rows: seededSystemRoles},
		nil,
		nil,
		rdb,
	)

	ctx := context.Background()
	if err := rdb.Set(ctx, constants.KeyUserVersion(userID.String()), selectOrgTestUserVer, 0).Err(); err != nil {
		t.Fatalf("khong seed duoc user_version: %v", err)
	}

	accessToken, _, err := utils.GenerateTokens(cfg, userID, deviceID, tokenActiveRole, nil, selectOrgTestUserVer)
	if err != nil {
		t.Fatalf("khong sinh duoc access token: %v", err)
	}

	app := fiber.New()
	api := app.Group("/api")
	SetupAuthRoutes(api, cfg, handler.NewAuthHandler(svc), nil, rdb)

	return &n2n6TestEnv{app: app, rdb: rdb, mr: mr, userID: userID, deviceID: deviceID, token: accessToken}
}

func (e *n2n6TestEnv) postSelectOrgEmpty(t *testing.T) (int, []byte) {
	t.Helper()
	req := httptest.NewRequest("POST", "/api/auth/select-org", strings.NewReader("{}"))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+e.token)
	resp, err := e.app.Test(req)
	if err != nil {
		t.Fatalf("app.Test loi: %v", err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, raw
}

// TestSelectOrg_Live_ActiveRoleLaOrgRole_TraLoiRoRangKhongTraTokenRong (N2, review vong 2 260915):
// token mang active_role="KE_TOAN" (mot ten role KHONG khop bat ky system role nao — mo phong
// dung kich ban "dang o org role" hoac "system role vua bi thu hoi" ma finding N2 mo ta). Truoc
// fix, SwitchOrg se sinh token moi voi active_role="" mot cach im lang. Sau fix phai tra loi ro
// (409) va KHONG duoc ghi refresh token moi xuong Redis (tuc khong "thanh cong mot nua").
func TestSelectOrg_Live_ActiveRoleLaOrgRole_TraLoiRoRangKhongTraTokenRong(t *testing.T) {
	// KHONG seed system role nao khop "KE_TOAN" — danh sach system role rong hoan toan de
	// mo phong dung "active_role hien tai khong phai system role nao ca".
	env := newN2N6TestEnv(t, "KE_TOAN", nil, true)

	status, raw := env.postSelectOrgEmpty(t)
	if status != fiber.StatusConflict {
		t.Fatalf("status = %d, muon 409 (active role khong resolve duoc), body: %s", status, raw)
	}
	if !strings.Contains(string(raw), "active role could not be resolved") {
		t.Fatalf("body khong chua thong bao ErrActiveRoleNotResolvable: %s", raw)
	}

	// Khang dinh KHONG co refresh token nao duoc ghi cho device nay — proof truc tiep rang
	// completeLogin KHONG he chay qua, khong chi la response tra loi.
	refreshKey := constants.KeyRefresh(env.userID.String())
	exists, err := env.rdb.HExists(context.Background(), refreshKey, env.deviceID.String()).Result()
	if err != nil {
		t.Fatalf("loi kiem tra refresh key: %v", err)
	}
	if exists {
		t.Error("refresh token DA duoc ghi xuong Redis du bi tu choi — nghia la completeLogin van chay mot phan truoc khi loi")
	}
}

// TestSelectOrg_Live_UserInactive_BiTuChoi (N6, review vong 2 260915): user.IsActive=false —
// SelectOrg khong duoc cap lai token cho tai khoan da bi vo hieu hoa, giong Login da lam.
func TestSelectOrg_Live_UserInactive_BiTuChoi(t *testing.T) {
	// He STUDENT khop dung active_role trong token de co lap rieng dieu kien IsActive — khong
	// de finding N2 lam nhieu ket qua cua test nay.
	sysRoleID := uuid.New()
	sysRole := &model.SystemRole{Name: "STUDENT"}
	sysRole.ID = sysRoleID

	env := newN2N6TestEnv(t, "STUDENT", []model.UserSystemRole{{
		SystemRoleID: sysRoleID,
		SystemRole:   sysRole,
	}}, false /* IsActive */)

	status, raw := env.postSelectOrgEmpty(t)
	if status != fiber.StatusUnauthorized {
		t.Fatalf("status = %d, muon 401 (tai khoan da bi vo hieu hoa), body: %s", status, raw)
	}
	if !strings.Contains(string(raw), "inactive") {
		t.Fatalf("body khong chua thong bao ErrUserInactive: %s", raw)
	}

	refreshKey := constants.KeyRefresh(env.userID.String())
	exists, err := env.rdb.HExists(context.Background(), refreshKey, env.deviceID.String()).Result()
	if err != nil {
		t.Fatalf("loi kiem tra refresh key: %v", err)
	}
	if exists {
		t.Error("refresh token DA duoc ghi xuong Redis cho tai khoan da vo hieu hoa")
	}
}
