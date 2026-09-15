package router

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
	"study.com/v1/internal/dto"
	"study.com/v1/internal/handler"
	"study.com/v1/internal/model"
	"study.com/v1/internal/repository"
	"study.com/v1/internal/service"
	"study.com/v1/internal/utils"
)

// ─────────────────────────────────────────────────────────────────────────────
// BLOCKER-1 (review 260915) — test "route SỐNG" cho POST /api/auth/select-org.
//
// Vì sao phải có file này: bộ test cũ (auth_select_org_test.go) FAKE cả
// AuthServiceInterface, nên nó chỉ chứng minh được "handler gọi service" chứ không chứng minh
// được service trả lời được. Đúng cái lỗi BLOCKER-1 nằm SAU lớp fake đó: SelectOrg đọc pending
// key của luồng đăng nhập và đòi pending.SelectedRole != nil, mà SelectRole luôn hoàn tất login
// rồi XOÁ pending key ⇒ mọi lời gọi thật đều 400, còn test fake thì vẫn xanh.
//
// File này đi hết đường thật: Fiber route THẬT (SetupAuthRoutes) → AuthMiddleware THẬT →
// AuthHandler THẬT → AuthService THẬT → Redis THẬT (miniredis). Chỉ repository được fake, vì
// chúng là biên DB — và chúng cũng chính là thứ quyết định "user có thuộc org này không".
// ─────────────────────────────────────────────────────────────────────────────

const (
	selectOrgTestJWTSecret = "select-org-live-test-secret"
	selectOrgTestUserVer   = int64(1)
)

// ownedOrg — một tổ chức mà user test THẬT SỰ thuộc (fake repo sẽ trả về).
type ownedOrg struct {
	ID   uuid.UUID
	Name string
}

// ── Repository fakes ────────────────────────────────────────────────────────
// Nhúng interface thật để method không override panic nếu bị gọi — nghĩa là test cũng pin luôn
// "đường đi này KHÔNG chạm tới chúng".

type liveUserRepo struct {
	repository.UserRepositoryInterface
	user *model.User
}

func (f *liveUserRepo) FindUserByID(ctx context.Context, id uuid.UUID) (*model.User, error) {
	return f.user, nil
}

type liveUserOrgRoleRepo struct {
	repository.UserOrganizationRoleRepositoryInterface
	rows []model.UserOrganizationRole
}

func (f *liveUserOrgRoleRepo) FindByUserIDWithDetails(ctx context.Context, userID uuid.UUID, status string) ([]model.UserOrganizationRole, error) {
	return f.rows, nil
}

type liveUserSystemRoleRepo struct {
	repository.UserSystemRoleRepositoryInterface
	rows []model.UserSystemRole
}

func (f *liveUserSystemRoleRepo) FindByUserIDWithDetails(ctx context.Context, userID uuid.UUID, status string) ([]model.UserSystemRole, error) {
	return f.rows, nil
}

// ── Môi trường test ─────────────────────────────────────────────────────────

type selectOrgLiveEnv struct {
	app      *fiber.App
	cfg      *config.Config
	rdb      *redis.Client
	mr       *miniredis.Miniredis
	userID   uuid.UUID
	deviceID uuid.UUID
	token    string
}

// newSelectOrgLiveEnv dựng app thật với miniredis. orgs = các tổ chức user thuộc (rỗng = không
// thuộc tổ chức nào).
//
// CHÚ Ý: KHÔNG hề seed pending login key (`pending_login:*`) nào. Đó là chủ ý — đúng cái lỗi
// BLOCKER-1 là "route chỉ chạy khi có pending key, mà luồng thật không bao giờ còn pending key".
// Test nào ở đây cũng phải xanh trong điều kiện KHÔNG có pending key.
func newSelectOrgLiveEnv(t *testing.T, orgs ...ownedOrg) *selectOrgLiveEnv {
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

	user := &model.User{UserName: "hocsinh", Email: "hocsinh@example.com", IsActive: true}
	user.ID = userID

	// Một org role để SwitchOrg tìm được currentRole theo activeRole trong token.
	sysRoleID := uuid.New()
	sysRole := &model.SystemRole{Name: "STUDENT"}
	sysRole.ID = sysRoleID

	userRepo := &liveUserRepo{user: user}

	orgRows := make([]model.UserOrganizationRole, 0, len(orgs))
	for _, o := range orgs {
		org := &model.Organization{Name: o.Name}
		org.ID = o.ID
		role := &model.Role{Name: "HOC_SINH"}
		role.ID = uuid.New()
		row := model.UserOrganizationRole{
			UserID:         userID,
			OrganizationID: o.ID,
			RoleID:         role.ID,
			Role:           role,
			Organization:   org,
		}
		orgRows = append(orgRows, row)
	}

	svc := service.NewAuthService(
		cfg,
		userRepo,
		nil, // roleRepo — không dùng trên đường này
		&liveUserOrgRoleRepo{rows: orgRows},
		&liveUserSystemRoleRepo{rows: []model.UserSystemRole{{
			UserID:       userID,
			SystemRoleID: sysRoleID,
			SystemRole:   sysRole,
		}}},
		nil, // systemRoleRepo
		nil, // userRoleRepo
		rdb,
	)

	// Seed đúng những gì AuthMiddleware cần: user_version trong Redis.
	ctx := context.Background()
	if err := rdb.Set(ctx, constants.KeyUserVersion(userID.String()), selectOrgTestUserVer, 0).Err(); err != nil {
		t.Fatalf("không seed được user_version: %v", err)
	}

	accessToken, _, err := utils.GenerateTokens(cfg, userID, deviceID, "STUDENT", nil, selectOrgTestUserVer)
	if err != nil {
		t.Fatalf("không sinh được access token: %v", err)
	}

	// oauthHandler = nil: chỉ tạo method value lúc đăng ký route, không gọi tới — an toàn.
	app := fiber.New()
	api := app.Group("/api")
	SetupAuthRoutes(api, cfg, handler.NewAuthHandler(svc), nil, rdb)

	return &selectOrgLiveEnv{
		app:      app,
		cfg:      cfg,
		rdb:      rdb,
		mr:       mr,
		userID:   userID,
		deviceID: deviceID,
		token:    accessToken,
	}
}

// assertNoPendingLoginKey — chốt chặn BLOCKER-1: đường select-org phải sống mà KHÔNG cần pending
// key. Nếu ai đó "sửa" bằng cách cho route đọc lại pending_key, test này sẽ đỏ vì app không hề
// ghi key đó.
func (e *selectOrgLiveEnv) assertNoPendingLoginKey(t *testing.T) {
	t.Helper()
	for _, k := range e.mr.Keys() {
		if strings.HasPrefix(k, "pending_login:") || strings.Contains(k, "pending") {
			t.Fatalf("Redis có pending key %q — test này chỉ có nghĩa khi KHÔNG có pending key", k)
		}
	}
}

func (e *selectOrgLiveEnv) postSelectOrg(t *testing.T, body string, sendToken bool) (int, map[string]interface{}, []byte) {
	t.Helper()
	req := httptest.NewRequest("POST", "/api/auth/select-org", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	if sendToken {
		req.Header.Set("Authorization", "Bearer "+e.token)
	}
	resp, err := e.app.Test(req)
	if err != nil {
		t.Fatalf("app.Test lỗi: %v", err)
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("đọc body lỗi: %v", err)
	}

	var parsed map[string]interface{}
	_ = json.Unmarshal(raw, &parsed)
	return resp.StatusCode, parsed, raw
}

// dataOf lấy body["data"] và fail gọn gàng nếu response không có data.
func dataOf(t *testing.T, body map[string]interface{}) map[string]interface{} {
	t.Helper()
	data, ok := body["data"].(map[string]interface{})
	if !ok {
		t.Fatalf("response không có object `data`, body = %#v", body)
	}
	return data
}

// ── Test 1: happy path ──────────────────────────────────────────────────────

// TestSelectOrg_Live_HappyPath_ReissuesTokenWithNewActiveOrg — user thuộc orgA, gọi select-org
// với orgA ⇒ 200, completed=true, active_org=orgA, VÀ access token mới mang active_org_id=orgA.
//
// Vế cuối là vế quan trọng nhất: đổi org mà token không đổi thì permission vẫn tính theo org cũ,
// tức là tính năng không có tác dụng gì dù HTTP 200.
func TestSelectOrg_Live_HappyPath_ReissuesTokenWithNewActiveOrg(t *testing.T) {
	orgA := ownedOrg{ID: uuid.New(), Name: "Trường THPT A"}
	env := newSelectOrgLiveEnv(t, orgA)
	env.assertNoPendingLoginKey(t)

	status, body, raw := env.postSelectOrg(t, `{"organization_id":"`+orgA.ID.String()+`"}`, true)
	if status != fiber.StatusOK {
		t.Fatalf("mong đợi 200, nhận %d (body=%#v) — route chết hoặc middleware chặn", status, body)
	}

	// Giải mã THÊM một lần vào DTO thật: khẳng định json tag của SelectRoleResponseDto khớp với
	// thứ handler trả ra (đổi tag sẽ làm vế này đỏ; map[string]interface{} thì không bắt được).
	var typed struct {
		Message string                    `json:"message"`
		Data    dto.SelectRoleResponseDto `json:"data"`
	}
	if err := json.Unmarshal(raw, &typed); err != nil {
		t.Fatalf("response không khớp SelectRoleResponseDto: %v", err)
	}
	if !typed.Data.Completed || typed.Data.AccessToken == "" || typed.Data.ActiveOrg == nil {
		t.Errorf("DTO giải mã thiếu trường: completed=%v token=%q active_org=%v",
			typed.Data.Completed, typed.Data.AccessToken, typed.Data.ActiveOrg)
	}
	if typed.Data.ActiveOrg.ID != orgA.ID.String() {
		t.Errorf("DTO active_org.id = %q, mong đợi %q", typed.Data.ActiveOrg.ID, orgA.ID.String())
	}

	data := dataOf(t, body)
	if data["completed"] != true {
		t.Errorf("completed = %v, mong đợi true", data["completed"])
	}

	activeOrg, ok := data["active_org"].(map[string]interface{})
	if !ok {
		t.Fatalf("data.active_org không phải object: %#v", data["active_org"])
	}
	if activeOrg["id"] != orgA.ID.String() {
		t.Errorf("active_org.id = %v, mong đợi %v", activeOrg["id"], orgA.ID.String())
	}

	newToken, _ := data["access_token"].(string)
	if newToken == "" {
		t.Fatal("không có access_token mới trong response — user sẽ tiếp tục dùng token mang org cũ")
	}
	if newToken == env.token {
		t.Error("access_token KHÔNG đổi sau khi đổi org — token cũ vẫn mang active_org cũ")
	}

	claims, err := utils.ParseToken(env.cfg, newToken)
	if err != nil {
		t.Fatalf("access token mới không parse được: %v", err)
	}
	if claims.ActiveOrgID == nil || *claims.ActiveOrgID != orgA.ID {
		t.Errorf("active_org_id trong token mới = %v, mong đợi %v", claims.ActiveOrgID, orgA.ID)
	}
	if claims.UserID != env.userID {
		t.Errorf("user_id trong token mới = %v, mong đợi %v", claims.UserID, env.userID)
	}
	if claims.ActiveRole != "STUDENT" {
		t.Errorf("active_role = %q, mong đợi giữ nguyên %q — select-org KHÔNG được đổi role", claims.ActiveRole, "STUDENT")
	}
}

// ── Test 2: org không thuộc user ────────────────────────────────────────────

// TestSelectOrg_Live_ForeignOrg_Rejected — user KHÔNG thuộc orgB ⇒ phải bị từ chối và KHÔNG được
// cấp token. Đây là chốt chặn leo quyền: nhận org lạ nghĩa là tự đặt mình vào tổ chức người khác.
//
// Về mã trạng thái: brief BLOCKER-1 ghi "org không thuộc user → 400", còn LOW-7 của chính review
// 260915 đề xuất phân loại lại thành 403 (lỗi quyền, không phải lỗi nhập liệu) — và LOW-7 được
// giao kèm trong cùng yêu cầu ("sửa cái nào < 10 dòng"). Bản này áp dụng 403 theo LOW-7.
// Muốn quay lại đúng chữ 400: đổi nhánh `service.ErrOrgNotBelongToUser` trong
// handler.selectOrgErrorStatus thành fiber.StatusBadRequest — một dòng.
func TestSelectOrg_Live_ForeignOrg_Rejected(t *testing.T) {
	orgA := ownedOrg{ID: uuid.New(), Name: "Trường THPT A"}
	orgB := ownedOrg{ID: uuid.New(), Name: "Trường THPT B"}
	env := newSelectOrgLiveEnv(t, orgA)
	env.assertNoPendingLoginKey(t)

	status, body, _ := env.postSelectOrg(t, `{"organization_id":"`+orgB.ID.String()+`"}`, true)
	if status != fiber.StatusForbidden {
		t.Fatalf("org không thuộc user: mong đợi 403, nhận %d (body=%#v)", status, body)
	}
	if _, hasData := body["data"]; hasData {
		t.Errorf("response lỗi vẫn kèm `data` (có thể đã cấp token cho org lạ): %#v", body)
	}

	// Không được ghi đè refresh token đang có: nếu đường lỗi vẫn chạy tới completeLogin thì
	// refresh token của phiên hiện tại đã bị thay.
	refreshKey := constants.KeyRefresh(env.userID.String())
	if env.rdb.Exists(context.Background(), refreshKey).Val() != 0 {
		t.Error("refresh token bị ghi dù request bị từ chối — đường lỗi chạm tới completeLogin")
	}
}

// ── Test 3: rỗng = Độc lập ──────────────────────────────────────────────────

// TestSelectOrg_Live_EmptyOrg_IndependentMode — organization_id rỗng ⇒ chế độ "Độc lập":
// active_org = null và token mới KHÔNG mang active_org_id (permission chỉ còn từ system role).
func TestSelectOrg_Live_EmptyOrg_IndependentMode(t *testing.T) {
	orgA := ownedOrg{ID: uuid.New(), Name: "Trường THPT A"}
	env := newSelectOrgLiveEnv(t, orgA)
	env.assertNoPendingLoginKey(t)

	status, body, _ := env.postSelectOrg(t, `{}`, true)
	if status != fiber.StatusOK {
		t.Fatalf("chế độ Độc lập: mong đợi 200, nhận %d (body=%#v)", status, body)
	}

	data := dataOf(t, body)
	if data["active_org"] != nil {
		t.Errorf("active_org = %#v, mong đợi null ở chế độ Độc lập", data["active_org"])
	}

	newToken, _ := data["access_token"].(string)
	claims, err := utils.ParseToken(env.cfg, newToken)
	if err != nil {
		t.Fatalf("access token mới không parse được: %v", err)
	}
	if claims.ActiveOrgID != nil {
		t.Errorf("active_org_id = %v, mong đợi nil ở chế độ Độc lập", *claims.ActiveOrgID)
	}
}

// ── Test 4: route phải nằm SAU auth ─────────────────────────────────────────

// TestSelectOrg_Live_RequiresAccessToken — không token ⇒ 401. Vừa chốt route nằm sau
// AuthMiddleware, vừa chốt danh tính KHÔNG thể lấy từ body: request dưới đây mang đủ
// organization_id hợp lệ, nếu handler còn tin body thì nó đã đi tiếp.
func TestSelectOrg_Live_RequiresAccessToken(t *testing.T) {
	orgA := ownedOrg{ID: uuid.New(), Name: "Trường THPT A"}
	env := newSelectOrgLiveEnv(t, orgA)

	status, body, _ := env.postSelectOrg(t, `{"organization_id":"`+orgA.ID.String()+`"}`, false)
	if status != fiber.StatusUnauthorized {
		t.Fatalf("thiếu access token: mong đợi 401, nhận %d (body=%#v)", status, body)
	}
}

// TestSelectOrg_Live_MalformedOrgID_Rejected — chuỗi không phải UUID bị chặn ở tầng validate,
// service không được chạm tới. Chốt luôn rằng route còn đi qua đúng chuỗi middleware + validate.
func TestSelectOrg_Live_MalformedOrgID_Rejected(t *testing.T) {
	env := newSelectOrgLiveEnv(t)

	status, _, _ := env.postSelectOrg(t, `{"organization_id":"khong-phai-uuid"}`, true)
	if status != fiber.StatusBadRequest {
		t.Fatalf("organization_id sai định dạng: mong đợi 400, nhận %d", status)
	}
}

// TestSelectOrg_Live_RouteRegisteredInRealApp — chot chan: route phai nam trong bang route THAT
// (khong phai app toi gian do test tu dung). Go dong dang ky ⇒ 404 ⇒ bai test do.
func TestSelectOrg_Live_RouteRegisteredInRealApp(t *testing.T) {
	env := newSelectOrgLiveEnv(t)

	var found bool
	for _, r := range env.app.GetRoutes() {
		if r.Method == "POST" && r.Path == "/api/auth/select-org" {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("POST /api/auth/select-org không có trong bảng route của app thật")
	}
}
