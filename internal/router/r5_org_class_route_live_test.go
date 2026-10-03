package router

// Lane R5: chủ/quản trị tổ chức quản lý lớp của tổ chức, đi bằng HTTP qua route THẬT (AuthMiddleware ->
// PermissionChecker THẬT -> handler THẬT -> service THẬT -> Postgres THẬT, schema tạm).
//
// B-02/B-05/B-12: GET /organizations/:id/classes (danh sách lớp của tổ chức), GET /classes/:id, /students,
// /attendances (buổi học: xem class_org_manage_postgres_test.go ở service) và PUT /classes/:id (kích hoạt) của chủ tổ chức A đi được với lớp thuộc A, còn lớp của
// tổ chức khác hoặc lớp cá nhân vẫn 404 (đọc) / 403 (ghi). B-16: members trả tên + email; xoá org role còn người giữ 409.
// Bỏ classAccessAsAdmin hoặc nhánh WithAuthorizer ở services.go thì các ca "chủ tổ chức A" ĐỎ.

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

type r5RouteEnv struct {
	app                          *fiber.App
	orgA, orgB                   model.Organization
	classA, classB, personal     model.Class
	ownerATok, ownerBTok, stuTok string
	ownerAID, studentID          uuid.UUID
	holderRole                   model.Role
}

func newR5RouteEnv(t *testing.T) *r5RouteEnv {
	t.Helper()
	db := pgtest.IsolatedSchema(t, database.Migrate)
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = rdb.Close() })
	cfg := &config.Config{JWTSecret: "r5-org-class-route-secret", JWTAccessExpiration: 15 * time.Minute, JWTRefreshExpiration: 7 * 24 * time.Hour}
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	e := &r5RouteEnv{}
	perm := model.Permission{Name: "ORG_MEMBERS_MANAGE"}
	must(db.Create(&perm).Error)
	roleP := model.Permission{Name: "ORG_ROLES_MANAGE"}
	must(db.Create(&roleP).Error)

	e.orgA = model.Organization{Name: "R5 org A " + uuid.NewString()[:6]}
	e.orgB = model.Organization{Name: "R5 org B " + uuid.NewString()[:6]}
	must(db.Create(&e.orgA).Error)
	must(db.Create(&e.orgB).Error)
	mkRole := func(org model.Organization, name string, perms ...model.Permission) model.Role {
		r := model.Role{Name: name + "-" + uuid.NewString()[:6], OrganizationID: &org.ID, Status: "active"}
		must(db.Create(&r).Error)
		for _, p := range perms {
			must(db.Create(&model.RolePermission{RoleID: r.ID, PermissionID: p.ID}).Error)
		}
		return r
	}
	mkUser := func(name string) model.User {
		u := model.User{Email: name + "-" + uuid.NewString()[:6] + "@40study.test", PasswordHash: "x", UserName: name + uuid.NewString()[:6], IsActive: true}
		must(db.Create(&u).Error)
		must(rdb.Set(context.Background(), constants.KeyUserVersion(u.ID.String()), 1, 0).Err())
		return u
	}
	ownerA, ownerB, student, teacher := mkUser("ownera"), mkUser("ownerb"), mkUser("student"), mkUser("teacher")
	e.ownerAID, e.studentID = ownerA.ID, student.ID
	must(db.Create(&model.UserOrganizationRole{UserID: ownerA.ID, RoleID: mkRole(e.orgA, "ORG_OWNER", perm, roleP).ID, OrganizationID: e.orgA.ID, Status: model.UserOrgRoleStatusActive}).Error)
	must(db.Create(&model.UserOrganizationRole{UserID: ownerB.ID, RoleID: mkRole(e.orgB, "ORG_OWNER", perm, roleP).ID, OrganizationID: e.orgB.ID, Status: model.UserOrgRoleStatusActive}).Error)
	e.holderRole = mkRole(e.orgA, "TRO_GIANG")
	must(db.Create(&model.UserOrganizationRole{UserID: teacher.ID, RoleID: e.holderRole.ID, OrganizationID: e.orgA.ID, Status: model.UserOrgRoleStatusActive}).Error)

	e.classA = model.Class{Name: "R5 lớp A", Status: "draft", OrganizationID: &e.orgA.ID}
	e.classB = model.Class{Name: "R5 lớp B", Status: "active", OrganizationID: &e.orgB.ID}
	e.personal = model.Class{Name: "R5 lớp cá nhân", Status: "active"}
	for _, c := range []*model.Class{&e.classA, &e.classB, &e.personal} {
		must(db.Create(c).Error)
	}
	must(db.Create(&model.StudentClass{StudentID: student.ID, ClassID: e.classA.ID, Status: "active"}).Error)

	roleRepo := repository.NewRoleRepository(db)
	uorRepo := repository.NewUserOrganizationRoleRepository(db)
	pc := middleware.NewPermissionChecker(repository.NewUserSystemRoleRepository(db), repository.NewSystemRoleRepository(db), uorRepo, roleRepo)
	classRepo, courseRepo := repository.NewClassRepository(db), repository.NewCourseRepository(db)
	classSvc := service.NewClassService(classRepo, courseRepo, repository.NewTeacherRepository(db), repository.NewStudentRepository(db), nil).WithAuthorizer(pc)
	attSvc := service.NewAttendanceService(repository.NewAttendanceRepository(db), classRepo, courseRepo).WithAuthorizer(pc)
	roleSvc := service.NewRoleService(roleRepo, repository.NewPermissionRepository(db))
	uorSvc := service.NewUserOrganizationRoleService(uorRepo, repository.NewUserRepository(db), roleRepo, repository.NewOrganizationRepository(db), nil)

	app := fiber.New()
	api := app.Group("/api")
	SetupClassRoutes(api, cfg, handler.NewClassHandler(classSvc, pc), handler.NewAttendanceHandler(attSvc, pc), rdb, pc)
	SetupOrgRoleRoutes(api, cfg, handler.NewRoleHandler(roleSvc, pc), rdb, pc)
	SetupUserOrganizationRoleRoutes(api, cfg, handler.NewUserOrganizationRoleHandler(uorSvc, pc), rdb, pc)
	e.app = app

	token := func(u model.User, role string, org *uuid.UUID) string {
		tok, _, err := utils.GenerateTokens(cfg, u.ID, uuid.New(), role, org, 1)
		must(err)
		return tok
	}
	e.ownerATok = token(ownerA, "ORG_OWNER", &e.orgA.ID)
	e.ownerBTok = token(ownerB, "ORG_OWNER", &e.orgB.ID)
	e.stuTok = token(student, "STUDENT", nil)
	return e
}

func (e *r5RouteEnv) do(t *testing.T, tok, method, path, body string) (int, string) {
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

func TestR5_OrgClassRoutes_ChuToChucQuanLyLopCuaToChuc(t *testing.T) {
	e := newR5RouteEnv(t)

	t.Run("danh sách lớp của tổ chức", func(t *testing.T) {
		status, raw := e.do(t, e.ownerATok, "GET", "/api/organizations/"+e.orgA.ID.String()+"/classes", "")
		if status != fiber.StatusOK || !strings.Contains(raw, e.classA.ID.String()) {
			t.Fatalf("chủ A xem lớp tổ chức A: %d %s", status, raw)
		}
		if strings.Contains(raw, e.classB.ID.String()) || strings.Contains(raw, e.personal.ID.String()) {
			t.Errorf("danh sách tổ chức A lộ lớp ngoài tổ chức: %s", raw)
		}
		if status, raw := e.do(t, e.ownerATok, "GET", "/api/organizations/"+e.orgB.ID.String()+"/classes", ""); status != fiber.StatusForbidden {
			t.Errorf("chủ A xem lớp tổ chức B: %d %s, muốn 403", status, raw)
		}
		if status, raw := e.do(t, e.stuTok, "GET", "/api/organizations/"+e.orgA.ID.String()+"/classes", ""); status != fiber.StatusForbidden {
			t.Errorf("học viên xem lớp tổ chức: %d %s, muốn 403", status, raw)
		}
	})

	t.Run("chi tiết lớp, học viên, buổi học, điểm danh", func(t *testing.T) {
		base := "/api/classes/" + e.classA.ID.String()
		for _, p := range []string{"", "/students", "/teachers", "/attendances"} {
			if status, raw := e.do(t, e.ownerATok, "GET", base+p, ""); status != fiber.StatusOK {
				t.Errorf("chủ A GET %s: %d %s, muốn 200", base+p, status, raw)
			}
		}
		// Lớp của tổ chức khác và lớp cá nhân: 404 (không dò được).
		for _, id := range []uuid.UUID{e.classB.ID, e.personal.ID} {
			for _, p := range []string{"", "/students", "/attendances"} {
				if status, raw := e.do(t, e.ownerATok, "GET", "/api/classes/"+id.String()+p, ""); status != fiber.StatusNotFound {
					t.Errorf("chủ A GET /classes/%s%s: %d %s, muốn 404", id, p, status, raw)
				}
			}
		}
	})

	t.Run("cờ can_manage và ô chọn học viên", func(t *testing.T) {
		base := "/api/classes/" + e.classA.ID.String()
		if _, raw := e.do(t, e.ownerATok, "GET", base, ""); !strings.Contains(raw, `"can_manage":true`) || !strings.Contains(raw, `"can_assign_teachers":true`) {
			t.Errorf("chủ A phải có can_manage và can_assign_teachers: %s", raw)
		}
		if _, raw := e.do(t, e.stuTok, "GET", base, ""); strings.Contains(raw, "can_manage") {
			t.Errorf("học viên không được có can_manage: %s", raw)
		}
		if status, raw := e.do(t, e.ownerATok, "GET", base+"/enrollable-students?keyword=a", ""); status != fiber.StatusOK || strings.Contains(raw, "@") {
			t.Errorf("chủ A tìm học viên: %d %s, muốn 200 và không có email", status, raw)
		}
		if status, raw := e.do(t, e.ownerBTok, "GET", base+"/enrollable-students", ""); status != fiber.StatusNotFound {
			t.Errorf("chủ B tìm học viên lớp của A: %d %s, muốn 404", status, raw)
		}
		if status, raw := e.do(t, e.stuTok, "GET", base+"/enrollable-students", ""); status != fiber.StatusForbidden {
			t.Errorf("học viên tìm học viên: %d %s, muốn 403", status, raw)
		}
	})

	t.Run("kích hoạt lớp và ghi danh", func(t *testing.T) {
		base := "/api/classes/" + e.classA.ID.String()
		if status, raw := e.do(t, e.ownerATok, "PUT", base, `{"status":"active"}`); status != fiber.StatusOK || !strings.Contains(raw, `"active"`) {
			t.Fatalf("chủ A kích hoạt lớp: %d %s", status, raw)
		}
		if status, raw := e.do(t, e.ownerBTok, "PUT", base, `{"status":"archived"}`); status != fiber.StatusForbidden {
			t.Errorf("chủ B sửa lớp của A: %d %s, muốn 403", status, raw)
		}
		if status, raw := e.do(t, e.stuTok, "PUT", base, `{"status":"archived"}`); status != fiber.StatusForbidden {
			t.Errorf("học viên sửa lớp: %d %s, muốn 403", status, raw)
		}
	})
}

func TestR5_OrgMembersVaXoaRole_Routes(t *testing.T) {
	e := newR5RouteEnv(t)

	status, raw := e.do(t, e.ownerATok, "GET", "/api/organizations/"+e.orgA.ID.String()+"/members", "")
	if status != fiber.StatusOK {
		t.Fatalf("members: %d %s", status, raw)
	}
	var out struct {
		Data struct {
			Rows []struct {
				UserID string `json:"user_id"`
				User   *struct {
					UserName string `json:"user_name"`
					Email    string `json:"email"`
				} `json:"user"`
			} `json:"user_organization_roles"`
		} `json:"data"`
	}
	if err := json.Unmarshal([]byte(raw), &out); err != nil || len(out.Data.Rows) == 0 {
		t.Fatalf("không đọc được members: %v %s", err, raw)
	}
	for _, r := range out.Data.Rows {
		if r.User == nil || r.User.UserName == "" || r.User.Email == "" {
			t.Errorf("thành viên %s thiếu tên/email: %s", r.UserID, raw)
		}
	}

	// Xoá role còn người giữ: 409 và role còn nguyên.
	del := "/api/org-roles/" + e.holderRole.ID.String()
	if status, raw := e.do(t, e.ownerATok, "DELETE", del, ""); status != fiber.StatusConflict || !strings.Contains(raw, `"code":"ROLE_IN_USE"`) {
		t.Fatalf("xoá role còn người giữ: %d %s, muốn 409 kèm code ROLE_IN_USE", status, raw)
	}
	if status, raw := e.do(t, e.ownerATok, "GET", del, ""); status != fiber.StatusOK {
		t.Errorf("role đã mất dù xoá bị từ chối: %d %s", status, raw)
	}
}
