package router

// Lane S6: kịch bản leo quyền org role, đi bằng HTTP qua route THẬT — AuthMiddleware -> PermissionChecker
// THẬT -> RoleHandler/UserOrganizationRoleHandler THẬT -> service THẬT -> Postgres THẬT (schema tạm).
//
// Chủ tổ chức A (ORG_ROLES_MANAGE + ORG_MEMBERS_MANAGE trong tổ chức A) trước đây: tạo role, gán
// SYSTEM_SETTINGS_MANAGE cho role đó, tự nhận role, rồi gọi được POST /notifications/send (chỉ admin) và
// mọi route hệ thống. Nay: bước gán quyền hệ thống 403, role của tổ chức khác 403, và dù dữ liệu cũ đã chứa
// quyền hệ thống trong org role thì route hệ thống vẫn 403. Từng vai: chủ tổ chức A, người không có quyền
// quản lý role (học viên), admin hệ thống.

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
	"study.com/v1/internal/middleware"
	"study.com/v1/internal/model"
	"study.com/v1/internal/repository"
	"study.com/v1/internal/service"
	"study.com/v1/internal/testutil/pgtest"
	"study.com/v1/internal/utils"
)

type s6OrgRouteEnv struct {
	app        *fiber.App
	perm       map[string]uuid.UUID
	orgA, orgB model.Organization
	roleB      model.Role
	ownerTok   string // chủ tổ chức A, active_org = A
	studentTok string // không có quyền quản lý role
	adminTok   string // SYSTEM_ADMIN
	ownerID    uuid.UUID
	memberID   uuid.UUID // thuộc CẢ tổ chức A và B
	poisoned   model.Role
}

func newS6OrgRouteEnv(t *testing.T) *s6OrgRouteEnv {
	t.Helper()
	db := pgtest.IsolatedSchema(t, database.Migrate)

	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = rdb.Close() })
	cfg := &config.Config{JWTSecret: "s6-org-role-route-secret", JWTAccessExpiration: 15 * time.Minute, JWTRefreshExpiration: 7 * 24 * time.Hour}

	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	e := &s6OrgRouteEnv{perm: map[string]uuid.UUID{}}
	for _, n := range []string{"ORG_ROLES_MANAGE", "ORG_MEMBERS_MANAGE", "REPORTS_VIEW_ORG", "SYSTEM_SETTINGS_MANAGE", "PAYMENTS_MANAGE", "USERS_BAN"} {
		p := model.Permission{Name: n}
		must(db.Create(&p).Error)
		e.perm[n] = p.ID
	}
	e.orgA = model.Organization{Name: "S6 route org A " + uuid.NewString()[:6]}
	e.orgB = model.Organization{Name: "S6 route org B " + uuid.NewString()[:6]}
	must(db.Create(&e.orgA).Error)
	must(db.Create(&e.orgB).Error)

	mkRole := func(org model.Organization, name string, perms ...string) model.Role {
		r := model.Role{Name: name + "-" + uuid.NewString()[:6], OrganizationID: &org.ID, Status: "active"}
		must(db.Create(&r).Error)
		for _, p := range perms {
			must(db.Create(&model.RolePermission{RoleID: r.ID, PermissionID: e.perm[p]}).Error)
		}
		return r
	}
	mkUser := func(name string) model.User {
		u := model.User{Email: name + "-" + uuid.NewString()[:6] + "@40study.test", PasswordHash: "x", UserName: name + uuid.NewString()[:6], IsActive: true}
		must(db.Create(&u).Error)
		must(rdb.Set(context.Background(), constants.KeyUserVersion(u.ID.String()), 1, 0).Err())
		return u
	}

	owner, student, admin := mkUser("owner"), mkUser("student"), mkUser("admin")
	e.ownerID = owner.ID
	ownerRole := mkRole(e.orgA, "ORG_OWNER", "ORG_ROLES_MANAGE", "ORG_MEMBERS_MANAGE")
	must(db.Create(&model.UserOrganizationRole{UserID: owner.ID, RoleID: ownerRole.ID, OrganizationID: e.orgA.ID, Status: model.UserOrgRoleStatusActive}).Error)
	e.roleB = mkRole(e.orgB, "ROLE_B", "REPORTS_VIEW_ORG")
	// Dữ liệu cũ: org role của tổ chức A mang quyền hệ thống (gán trước khi vá), chủ tổ chức đã nhận role đó.
	e.poisoned = mkRole(e.orgA, "POISONED", "SYSTEM_SETTINGS_MANAGE", "PAYMENTS_MANAGE", "USERS_BAN")
	must(db.Create(&model.UserOrganizationRole{UserID: owner.ID, RoleID: e.poisoned.ID, OrganizationID: e.orgA.ID, Status: model.UserOrgRoleStatusActive}).Error)

	// Người dùng thuộc cả hai tổ chức: dùng để kiểm chủ tổ chức A không đọc được vai trò ở tổ chức B.
	member := mkUser("member")
	e.memberID = member.ID
	memberRoleA := mkRole(e.orgA, "MEMBER_A", "REPORTS_VIEW_ORG")
	must(db.Create(&model.UserOrganizationRole{UserID: member.ID, RoleID: memberRoleA.ID, OrganizationID: e.orgA.ID, Status: model.UserOrgRoleStatusActive}).Error)
	must(db.Create(&model.UserOrganizationRole{UserID: member.ID, RoleID: e.roleB.ID, OrganizationID: e.orgB.ID, Status: model.UserOrgRoleStatusActive}).Error)

	adminSys := model.SystemRole{Name: "SYSTEM_ADMIN", Status: "active"}
	must(db.Create(&adminSys).Error)
	for _, p := range []string{"ORG_ROLES_MANAGE", "ORG_MEMBERS_MANAGE", "SYSTEM_SETTINGS_MANAGE"} {
		must(db.Create(&model.SystemRolePermission{SystemRoleID: adminSys.ID, PermissionID: e.perm[p]}).Error)
	}
	must(db.Create(&model.UserSystemRole{UserID: admin.ID, SystemRoleID: adminSys.ID, Status: model.UserSystemRoleStatusActive}).Error)

	roleRepo := repository.NewRoleRepository(db)
	uorRepo := repository.NewUserOrganizationRoleRepository(db)
	pc := middleware.NewPermissionChecker(repository.NewUserSystemRoleRepository(db), repository.NewSystemRoleRepository(db), uorRepo, roleRepo)
	roleSvc := service.NewRoleService(roleRepo, repository.NewPermissionRepository(db))
	uorSvc := service.NewUserOrganizationRoleService(uorRepo, repository.NewUserRepository(db), roleRepo, repository.NewOrganizationRepository(db), nil)

	app := fiber.New()
	api := app.Group("/api")
	SetupOrgRoleRoutes(api, cfg, handler.NewRoleHandler(roleSvc, pc), rdb, pc)
	SetupUserOrganizationRoleRoutes(api, cfg, handler.NewUserOrganizationRoleHandler(uorSvc, pc), rdb, pc)
	SetupNotificationRoutes(api, cfg, handler.NewNotificationHandler(&s5NotificationSvc{}), rdb, pc)
	e.app = app

	token := func(u model.User, role string, org *uuid.UUID) string {
		tok, _, err := utils.GenerateTokens(cfg, u.ID, uuid.New(), role, org, 1)
		must(err)
		return tok
	}
	e.ownerTok = token(owner, "ORG_OWNER", &e.orgA.ID)
	e.studentTok = token(student, "STUDENT", nil)
	e.adminTok = token(admin, "SYSTEM_ADMIN", nil)
	return e
}

func (e *s6OrgRouteEnv) do(t *testing.T, tok, method, path, body string) (int, string) {
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

func (e *s6OrgRouteEnv) permsBody(names ...string) string {
	ids := make([]string, len(names))
	for i, n := range names {
		ids[i] = e.perm[n].String()
	}
	b, _ := json.Marshal(map[string]interface{}{"permission_ids": ids})
	return string(b)
}

func TestS6_OrgRoleRoutes_ChuToChucKhongLeoQuyen(t *testing.T) {
	e := newS6OrgRouteEnv(t)

	// 1. Đúng kịch bản review: tạo role trong tổ chức mình (được), rồi gán quyền hệ thống (bị chặn).
	body := `{"name":"Tro giang","organization_id":"` + e.orgA.ID.String() + `"}`
	status, raw := e.do(t, e.ownerTok, "POST", "/api/org-roles/", body)
	if status != fiber.StatusCreated {
		t.Fatalf("chủ tổ chức tạo role trong tổ chức mình: %d %s", status, raw)
	}
	var created struct {
		Data struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.Unmarshal([]byte(raw), &created); err != nil || created.Data.ID == "" {
		t.Fatalf("không đọc được id role: %v %s", err, raw)
	}
	mine := "/api/org-roles/" + created.Data.ID + "/permissions"
	for _, method := range []string{"PUT", "POST"} {
		for _, sys := range []string{"SYSTEM_SETTINGS_MANAGE", "PAYMENTS_MANAGE", "USERS_BAN"} {
			if status, raw := e.do(t, e.ownerTok, method, mine, e.permsBody("REPORTS_VIEW_ORG", sys)); status != fiber.StatusForbidden {
				t.Errorf("%s gán %s cho role của mình: %d %s, muốn 403", method, sys, status, raw)
			}
		}
	}
	if status, raw := e.do(t, e.ownerTok, "PUT", mine, e.permsBody("REPORTS_VIEW_ORG")); status != fiber.StatusOK {
		t.Errorf("gán quyền thuộc phạm vi tổ chức bị chặn nhầm: %d %s", status, raw)
	}

	// 2. Tổ chức khác: tạo role, đọc/sửa/gỡ quyền của role tổ chức B đều 403.
	if status, raw := e.do(t, e.ownerTok, "POST", "/api/org-roles/", `{"name":"Xam nhap","organization_id":"`+e.orgB.ID.String()+`"}`); status != fiber.StatusForbidden {
		t.Errorf("tạo role trong tổ chức B: %d %s, muốn 403", status, raw)
	}
	other := "/api/org-roles/" + e.roleB.ID.String()
	for _, c := range []struct{ method, path, body string }{
		{"GET", other, ""},
		{"GET", other + "/permissions", ""},
		{"PUT", other + "/permissions", e.permsBody("ORG_ROLES_MANAGE")},
		{"POST", other + "/permissions", e.permsBody("ORG_ROLES_MANAGE")},
		{"DELETE", other + "/permissions", e.permsBody("REPORTS_VIEW_ORG")},
		{"PATCH", other + "/restore", ""},
		{"PUT", other, `{"name":"doi ten"}`},
		{"GET", "/api/org-roles/" + e.roleB.ID.String() + "/users", ""},
	} {
		if status, raw := e.do(t, e.ownerTok, c.method, c.path, c.body); status != fiber.StatusForbidden {
			t.Errorf("%s %s (chủ tổ chức A): %d %s, muốn 403", c.method, c.path, status, raw)
		}
	}
	// Danh sách chỉ có role của tổ chức A, dù xin tổ chức B thì 403.
	if status, raw := e.do(t, e.ownerTok, "GET", "/api/org-roles/?organization_id="+e.orgB.ID.String(), ""); status != fiber.StatusForbidden {
		t.Errorf("liệt kê role tổ chức B: %d %s, muốn 403", status, raw)
	}
	status, raw = e.do(t, e.ownerTok, "GET", "/api/org-roles/", "")
	if status != fiber.StatusOK || strings.Contains(raw, e.roleB.ID.String()) {
		t.Errorf("danh sách role của chủ tổ chức A: %d, lộ role tổ chức B: %s", status, raw)
	}

	// 3. Dữ liệu cũ đã chứa quyền hệ thống trong org role: route hệ thống vẫn 403 cho chủ tổ chức.
	notif := `{"title":"t","content":"c","notification_type":"system","user_ids":["00000000-0000-4000-8000-000000000001"]}`
	if status, raw := e.do(t, e.ownerTok, "POST", "/api/notifications/send", notif); status != fiber.StatusForbidden {
		t.Errorf("POST /notifications/send (chủ tổ chức mang quyền hệ thống trong org role cũ): %d %s, muốn 403", status, raw)
	}
	// Quyền thuộc phạm vi tổ chức vẫn dùng được (đọc danh sách role ở trên đã 200).

	// 4. Vai không có quyền quản lý role: 403 ở router.
	if status, raw := e.do(t, e.studentTok, "GET", "/api/org-roles/", ""); status != fiber.StatusForbidden {
		t.Errorf("học viên xem /org-roles: %d %s, muốn 403", status, raw)
	}
	if status, raw := e.do(t, e.studentTok, "PUT", mine, e.permsBody("REPORTS_VIEW_ORG")); status != fiber.StatusForbidden {
		t.Errorf("học viên sửa quyền role: %d %s, muốn 403", status, raw)
	}
}

func TestS6_OrgRoleRoutes_AdminDiDuocMoiToChucNhungKhongGanQuyenHeThong(t *testing.T) {
	e := newS6OrgRouteEnv(t)
	other := "/api/org-roles/" + e.roleB.ID.String()

	if status, raw := e.do(t, e.adminTok, "GET", other+"/permissions", ""); status != fiber.StatusOK {
		t.Errorf("admin đọc quyền role tổ chức B: %d %s, muốn 200", status, raw)
	}
	if status, raw := e.do(t, e.adminTok, "PUT", other+"/permissions", e.permsBody("REPORTS_VIEW_ORG", "ORG_MEMBERS_MANAGE")); status != fiber.StatusOK {
		t.Errorf("admin gán quyền tổ chức: %d %s, muốn 200", status, raw)
	}
	// Ngay cả admin cũng không gán được quyền hệ thống cho org role.
	if status, raw := e.do(t, e.adminTok, "PUT", other+"/permissions", e.permsBody("SYSTEM_SETTINGS_MANAGE")); status != fiber.StatusForbidden {
		t.Errorf("admin gán quyền hệ thống cho org role: %d %s, muốn 403", status, raw)
	}
	// Route hệ thống vẫn chạy cho admin (qua system role).
	notif := `{"title":"t","content":"c","notification_type":"system","user_ids":["00000000-0000-4000-8000-000000000001"]}`
	if status, raw := e.do(t, e.adminTok, "POST", "/api/notifications/send", notif); status != fiber.StatusCreated {
		t.Errorf("admin gửi thông báo: %d %s, muốn 201", status, raw)
	}
}

func TestS6_OrgMemberRoutes_ChuToChucChiThayToChucCuaMinh(t *testing.T) {
	e := newS6OrgRouteEnv(t)

	// GET /users/:id/org-roles: người dùng thuộc cả A và B; chủ tổ chức A chỉ thấy vai trò ở tổ chức A.
	status, raw := e.do(t, e.ownerTok, "GET", "/api/users/"+e.memberID.String()+"/org-roles", "")
	if status != fiber.StatusOK {
		t.Fatalf("GET org-roles của user: %d %s", status, raw)
	}
	if strings.Contains(raw, e.orgB.ID.String()) || strings.Contains(raw, e.roleB.ID.String()) {
		t.Errorf("chủ tổ chức A thấy vai trò ở tổ chức B: %s", raw)
	}
	if !strings.Contains(raw, e.orgA.ID.String()) {
		t.Errorf("không thấy vai trò tổ chức A: %s", raw)
	}
	// Admin thấy cả hai.
	status, raw = e.do(t, e.adminTok, "GET", "/api/users/"+e.memberID.String()+"/org-roles", "")
	if status != fiber.StatusOK || !strings.Contains(raw, e.orgA.ID.String()) || !strings.Contains(raw, e.orgB.ID.String()) {
		t.Errorf("admin phải thấy cả hai tổ chức: %d %s", status, raw)
	}
}