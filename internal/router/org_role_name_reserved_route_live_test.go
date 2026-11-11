package router

// Review #144, phát hiện 1 và 2 (route THẬT, Postgres schema tạm, Redis miniredis).
//
// Lỗ: org role đặt được tên trùng vai hệ thống (TEACHER, STUDENT...). SwitchRole/SelectRole với role_type=organization đưa
// thẳng tên vai vào claim active_role, nên các cổng chỉ so chuỗi claim (open-design đòi "TEACHER") bị vượt.
// Chốt: (1) API tạo/đổi tên vai trả 400 ROLE_NAME_RESERVED; (2) vai trùng tên đã có sẵn trong DB thì switch-role từ chối
// (400 ORG_ROLE_NAME_RESERVED), vai ORG_* và ORG_OWNER vẫn chọn được; (3) user-lookup giới hạn 30 lần/phút/user.
// Gỡ chặn ở role_service thì ca (1) ĐỎ; gỡ chặn ở completeLoginUnified thì ca (2) ĐỎ (token mang TEACHER mở được
// open-design); gỡ rate limiter thì ca (3) ĐỎ.

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
	"study.com/v1/internal/database"
	"study.com/v1/internal/handler"
	"study.com/v1/internal/model"
	"study.com/v1/internal/repository"
	"study.com/v1/internal/service"
	"study.com/v1/internal/testutil/pgtest"
	"study.com/v1/internal/utils"
)

func bodyCode(raw string) string {
	var b struct {
		Code string `json:"code"`
	}
	_ = json.Unmarshal([]byte(raw), &b)
	return b.Code
}

func TestReservedOrgRoleName_TaoVaDoiTenBiChan(t *testing.T) {
	e := newS6OrgRouteEnv(t)
	orgA := e.orgA.ID.String()

	for _, name := range []string{"TEACHER", " teacher ", "Student", "PARENT", "SYSTEM_ADMIN", "ORG_OWNER"} {
		status, raw := e.do(t, e.ownerTok, "POST", "/api/org-roles/", `{"name":"`+name+`","organization_id":"`+orgA+`"}`)
		if status != fiber.StatusBadRequest || bodyCode(raw) != "ROLE_NAME_RESERVED" {
			t.Errorf("tạo vai %q: got %d %s, muốn 400 ROLE_NAME_RESERVED", name, status, raw)
		}
	}
	var n int64
	e.db.Model(&model.Role{}).Where("organization_id = ? AND upper(trim(name)) IN ?", e.orgA.ID, model.BuiltInSystemRoleNames).Count(&n)
	if n != 0 {
		t.Fatalf("vẫn có %d vai trùng tên hệ thống được ghi vào DB", n)
	}

	// Tên thường vẫn tạo được; đổi tên sang TEACHER bị chặn.
	status, raw := e.do(t, e.ownerTok, "POST", "/api/org-roles/", `{"name":"ORG_TEACHER","organization_id":"`+orgA+`"}`)
	if status != fiber.StatusCreated {
		t.Fatalf("tạo ORG_TEACHER: %d %s", status, raw)
	}
	var created struct {
		Data struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	_ = json.Unmarshal([]byte(raw), &created)
	if status, raw := e.do(t, e.ownerTok, "PUT", "/api/org-roles/"+created.Data.ID, `{"name":"TEACHER"}`); status != fiber.StatusBadRequest || bodyCode(raw) != "ROLE_NAME_RESERVED" {
		t.Errorf("đổi tên sang TEACHER: got %d %s", status, raw)
	}

	// Vai TEACHER có từ trước, đã xoá mềm: không khôi phục được.
	legacy := model.Role{Name: "TEACHER", OrganizationID: &e.orgA.ID, Status: "active"}
	if err := e.db.Create(&legacy).Error; err != nil {
		t.Fatal(err)
	}
	e.db.Delete(&legacy)
	if status, raw := e.do(t, e.ownerTok, "PATCH", "/api/org-roles/"+legacy.ID.String()+"/restore", ""); status != fiber.StatusBadRequest || bodyCode(raw) != "ROLE_NAME_RESERVED" {
		t.Errorf("khôi phục vai TEACHER đã xoá: got %d %s", status, raw)
	}

	// Vai ORG_OWNER do hệ thống tạo: form sửa gửi lại đúng tên cũ thì vẫn lưu được (chỉ chặn khi ĐỔI tên).
	owner := model.Role{Name: "ORG_OWNER", OrganizationID: &e.orgA.ID, Status: "active"}
	if err := e.db.Create(&owner).Error; err != nil {
		t.Fatal(err)
	}
	if status, raw := e.do(t, e.ownerTok, "PUT", "/api/org-roles/"+owner.ID.String(), `{"name":"ORG_OWNER","description":"Chủ tổ chức"}`); status != fiber.StatusOK {
		t.Errorf("sửa mô tả vai ORG_OWNER (giữ tên): got %d %s", status, raw)
	}
}

// switchRoleEnv: AuthService THẬT trên Postgres schema tạm, AuthMiddleware + open-design THẬT.
type switchRoleEnv struct {
	app    *fiber.App
	org    model.Organization
	token  string // STUDENT hệ thống, chưa chọn tổ chức
	roleID map[string]uuid.UUID
}

func newSwitchRoleEnv(t *testing.T) *switchRoleEnv {
	t.Helper()
	db := pgtest.IsolatedSchema(t, database.Migrate)
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = rdb.Close() })
	cfg := &config.Config{JWTSecret: "reserved-org-role-secret", JWTAccessExpiration: 15 * time.Minute, JWTRefreshExpiration: 7 * 24 * time.Hour, AllowedOrigins: "http://localhost:3000"}
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}

	user := model.User{Email: "reserved-" + uuid.NewString()[:6] + "@40study.test", PasswordHash: "x", UserName: "reserved" + uuid.NewString()[:6], IsActive: true}
	must(db.Create(&user).Error)
	must(rdb.Set(context.Background(), constants.KeyUserVersion(user.ID.String()), 1, 0).Err())
	e := &switchRoleEnv{org: model.Organization{Name: "Reserved org " + uuid.NewString()[:6]}, roleID: map[string]uuid.UUID{}}
	must(db.Create(&e.org).Error)
	// "TEACHER" ghi thẳng DB: dữ liệu có từ trước khi API chặn tên (hoặc ghi tay), đúng tình huống phòng thủ lúc cấp token.
	for _, name := range []string{"TEACHER", "ORG_TEACHER", "ORG_OWNER"} {
		r := model.Role{Name: name, OrganizationID: &e.org.ID, Status: "active"}
		must(db.Create(&r).Error)
		must(db.Create(&model.UserOrganizationRole{UserID: user.ID, RoleID: r.ID, OrganizationID: e.org.ID, Status: model.UserOrgRoleStatusActive}).Error)
		e.roleID[name] = r.ID
	}

	usr := repository.NewUserSystemRoleRepository(db)
	svc := service.NewAuthService(cfg, repository.NewUserRepository(db), repository.NewRoleRepository(db),
		repository.NewUserOrganizationRoleRepository(db), usr, repository.NewSystemRoleRepository(db), usr, rdb)
	app := fiber.New()
	api := app.Group("/api")
	SetupAuthRoutes(api, cfg, handler.NewAuthHandler(svc), nil, rdb, nil)
	SetupOpenDesignRoutes(api, cfg, rdb)
	e.app = app
	tok, _, err := utils.GenerateTokens(cfg, user.ID, uuid.New(), "STUDENT", nil, 1)
	must(err)
	e.token = tok
	return e
}

func (e *switchRoleEnv) do(t *testing.T, tok, method, path, body string) (int, string) {
	t.Helper()
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+tok)
	resp, err := e.app.Test(req, -1)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, string(raw)
}

func (e *switchRoleEnv) switchTo(t *testing.T, name string) (int, string, string) {
	t.Helper()
	body := `{"role_id":"` + e.roleID[name].String() + `","role_type":"organization","organization_id":"` + e.org.ID.String() + `"}`
	status, raw := e.do(t, e.token, "POST", "/api/auth/switch-role", body)
	var out struct {
		Data struct {
			AccessToken string `json:"access_token"`
		} `json:"data"`
	}
	_ = json.Unmarshal([]byte(raw), &out)
	return status, raw, out.Data.AccessToken
}

func TestReservedOrgRoleName_SwitchRoleTuChoiVaiTrungTen(t *testing.T) {
	e := newSwitchRoleEnv(t)

	// Cổng open-design với token thường giữ như cũ: STUDENT hệ thống bị 403.
	if status, raw := e.do(t, e.token, "GET", "/api/open-design/status", ""); status != fiber.StatusForbidden {
		t.Fatalf("open-design với token STUDENT: %d %s", status, raw)
	}

	// Vai tổ chức tên TEACHER (có sẵn trong DB): từ chối, không cấp token nào.
	status, raw, tok := e.switchTo(t, "TEACHER")
	if status != fiber.StatusBadRequest || bodyCode(raw) != "ORG_ROLE_NAME_RESERVED" || tok != "" {
		t.Errorf("switch sang org role TEACHER: got %d %s (token=%t), muốn 400 ORG_ROLE_NAME_RESERVED", status, raw, tok != "")
	}
	if tok != "" {
		if status, _ := e.do(t, tok, "GET", "/api/open-design/status", ""); status == fiber.StatusOK {
			t.Errorf("token từ org role TEACHER mở được open-design (cổng chỉ dành cho giảng viên đã duyệt)")
		}
	}

	// Vai ORG_TEACHER và ORG_OWNER vẫn chọn được; ORG_TEACHER không qua cổng open-design.
	for _, name := range []string{"ORG_TEACHER", "ORG_OWNER"} {
		status, raw, tok := e.switchTo(t, name)
		if status != fiber.StatusOK || tok == "" {
			t.Fatalf("switch sang %s: %d %s", name, status, raw)
		}
		if name == "ORG_TEACHER" {
			if status, raw := e.do(t, tok, "GET", "/api/open-design/status", ""); status != fiber.StatusForbidden {
				t.Errorf("open-design với active_role ORG_TEACHER: %d %s, muốn 403", status, raw)
			}
		}
	}
}

func TestUserLookup_RateLimit30MoiPhutTheoUser(t *testing.T) {
	e := newS6OrgRouteEnv(t)
	path := "/api/organizations/" + e.orgA.ID.String() + "/user-lookup?email=nobody-rl@40study.test"
	for i := 1; i <= 30; i++ {
		if status, raw := e.do(t, e.ownerTok, "GET", path, ""); status != fiber.StatusNotFound {
			t.Fatalf("lượt %d: %d %s, muốn 404 (chưa chạm giới hạn)", i, status, raw)
		}
	}
	status, raw := e.do(t, e.ownerTok, "GET", path, "")
	if status != fiber.StatusTooManyRequests || !strings.Contains(raw, "retry_after") {
		t.Fatalf("lượt 31: %d %s, muốn 429 kèm retry_after", status, raw)
	}
	// Giới hạn theo user: người khác vẫn tra được.
	if status, raw := e.do(t, e.adminTok, "GET", path, ""); status != fiber.StatusNotFound {
		t.Errorf("admin sau khi chủ tổ chức bị chặn: %d %s, muốn 404", status, raw)
	}
}
