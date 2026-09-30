package service

// Lane S6 (Postgres thật), theo từng vai — org role leo quyền thành admin nền tảng.
//
// Kịch bản gốc (review S5, MAJOR-3): ORG_OWNER của tổ chức A (có ORG_ROLES_MANAGE + ORG_MEMBERS_MANAGE)
//  1. tạo role trong tổ chức mình,
//  2. PUT /org-roles/:id/permissions gán SYSTEM_SETTINGS_MANAGE/PAYMENTS_MANAGE/USERS_BAN,
//  3. tự gán role đó cho chính mình,
//  4. PermissionChecker gộp quyền org role vào tập quyền nên RequirePermissions("SYSTEM_SETTINGS_MANAGE")
//     và isAdminActor đều đúng: hoàn tiền, cộng xu, sửa/xoá khoá của người khác.
// Cũng sửa được quyền của role thuộc tổ chức B, và Restore/Get/List không kiểm tổ chức.
//
// Bỏ kiểm ở loadRoleForActor / requireOrgScopePermissions / bộ lọc data.IsOrgPermission trong
// PermissionChecker thì test tương ứng ĐỎ.

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"study.com/v1/internal/dto"
	"study.com/v1/internal/middleware"
	"study.com/v1/internal/model"
	"study.com/v1/internal/repository"
)

type s6OrgEnv struct {
	*s2Fixture
	roles        *RoleService
	userOrgRoles *UserOrganizationRoleService
	checker      *middleware.PermissionChecker
	orgA, orgB   model.Organization
	permIDs      map[string]uuid.UUID
}

var s6AllPerms = []string{
	"ORG_ROLES_MANAGE", "ORG_MEMBERS_MANAGE", "REPORTS_VIEW_ORG", // phạm vi tổ chức
	"SYSTEM_SETTINGS_MANAGE", "PAYMENTS_MANAGE", "USERS_BAN", "ROLES_MANAGE_SYSTEM", // hệ thống
	"COURSES_CREATE", // quyền giảng viên: cũng KHÔNG thuộc phạm vi tổ chức
}

func newS6OrgEnv(t *testing.T) *s6OrgEnv {
	t.Helper()
	f := newS2Fixture(t)
	e := &s6OrgEnv{s2Fixture: f, permIDs: map[string]uuid.UUID{}}
	for _, name := range s6AllPerms {
		p := model.Permission{Name: name}
		if err := f.db.Create(&p).Error; err != nil {
			t.Fatalf("tạo permission %s: %v", name, err)
		}
		e.permIDs[name] = p.ID
	}
	e.orgA = model.Organization{Name: "S6 org A " + uuid.NewString()[:8]}
	e.orgB = model.Organization{Name: "S6 org B " + uuid.NewString()[:8]}
	for _, o := range []*model.Organization{&e.orgA, &e.orgB} {
		if err := f.db.Create(o).Error; err != nil {
			t.Fatalf("tạo organization: %v", err)
		}
	}
	roleRepo := repository.NewRoleRepository(f.db)
	e.roles = NewRoleService(roleRepo, repository.NewPermissionRepository(f.db))
	uorRepo := repository.NewUserOrganizationRoleRepository(f.db)
	e.userOrgRoles = NewUserOrganizationRoleService(uorRepo, repository.NewUserRepository(f.db), roleRepo, repository.NewOrganizationRepository(f.db), nil)
	e.checker = middleware.NewPermissionChecker(repository.NewUserSystemRoleRepository(f.db), repository.NewSystemRoleRepository(f.db), uorRepo, roleRepo)
	return e
}

func (e *s6OrgEnv) ids(names ...string) []uuid.UUID {
	out := make([]uuid.UUID, len(names))
	for i, n := range names {
		out[i] = e.permIDs[n]
	}
	return out
}

// orgRole tạo role của tổ chức org (ghi thẳng DB, không qua service) rồi gán các quyền.
func (e *s6OrgEnv) orgRole(org model.Organization, name string, perms ...string) model.Role {
	e.t.Helper()
	r := model.Role{Name: name + "-" + uuid.NewString()[:6], OrganizationID: &org.ID, Status: "active"}
	if err := e.db.Create(&r).Error; err != nil {
		e.t.Fatalf("tạo role: %v", err)
	}
	for _, p := range perms {
		if err := e.db.Create(&model.RolePermission{RoleID: r.ID, PermissionID: e.permIDs[p]}).Error; err != nil {
			e.t.Fatalf("gán quyền %s: %v", p, err)
		}
	}
	return r
}

func (e *s6OrgEnv) grant(user model.User, org model.Organization, role model.Role) {
	e.t.Helper()
	uor := model.UserOrganizationRole{UserID: user.ID, RoleID: role.ID, OrganizationID: org.ID, Status: model.UserOrgRoleStatusActive}
	if err := e.db.Create(&uor).Error; err != nil {
		e.t.Fatalf("gán org role: %v", err)
	}
}

func (e *s6OrgEnv) has(user model.User, org *model.Organization, perm string) bool {
	e.t.Helper()
	var orgID *uuid.UUID
	if org != nil {
		orgID = &org.ID
	}
	ok, err := e.checker.HasPermission(context.Background(), user.ID, orgID, perm)
	if err != nil {
		e.t.Fatalf("HasPermission(%s): %v", perm, err)
	}
	return ok
}

func (e *s6OrgEnv) rolePerms(role model.Role) map[string]bool {
	e.t.Helper()
	got, err := repository.NewRoleRepository(e.db).GetPermissionsByRoleID(context.Background(), role.ID)
	if err != nil {
		e.t.Fatal(err)
	}
	out := map[string]bool{}
	for _, p := range got {
		out[p.Name] = true
	}
	return out
}

// Chủ tổ chức A không gán được quyền hệ thống/quyền ngoài phạm vi tổ chức cho role của mình, bằng cả
// ba đường (Add, Set) và cả khi admin gọi; gán quyền thuộc phạm vi tổ chức vẫn được.
func TestS6_OrgRole_KhongGanDuocQuyenHeThong(t *testing.T) {
	e := newS6OrgEnv(t)
	ctx := context.Background()
	owner := e.user("owner")
	ownerRole := e.orgRole(e.orgA, "ORG_OWNER", "ORG_ROLES_MANAGE", "ORG_MEMBERS_MANAGE")
	e.grant(owner, e.orgA, ownerRole)
	mine := e.orgRole(e.orgA, "TRO_GIANG")
	orgA := &e.orgA.ID

	for _, sys := range []string{"SYSTEM_SETTINGS_MANAGE", "PAYMENTS_MANAGE", "USERS_BAN", "ROLES_MANAGE_SYSTEM", "COURSES_CREATE"} {
		req := dto.AddPermissionsToRoleDTO{PermissionIDs: e.ids("REPORTS_VIEW_ORG", sys)}
		if err := e.roles.AddPermissionsToRole(ctx, mine.ID, orgA, false, req); !errors.Is(err, ErrPermissionNotOrgScope) {
			t.Errorf("Add %s (chủ tổ chức): err=%v, muốn ErrPermissionNotOrgScope", sys, err)
		}
		if err := e.roles.SetRolePermissions(ctx, mine.ID, orgA, false, req); !errors.Is(err, ErrPermissionNotOrgScope) {
			t.Errorf("Set %s (chủ tổ chức): err=%v, muốn ErrPermissionNotOrgScope", sys, err)
		}
		// admin cũng không được: org role không bao giờ mang quyền hệ thống.
		if err := e.roles.SetRolePermissions(ctx, mine.ID, nil, true, req); !errors.Is(err, ErrPermissionNotOrgScope) {
			t.Errorf("Set %s (admin): err=%v, muốn ErrPermissionNotOrgScope", sys, err)
		}
	}
	if got := e.rolePerms(mine); len(got) != 0 {
		t.Fatalf("role vẫn bị gán quyền sau khi bị từ chối: %v", got)
	}

	if err := e.roles.SetRolePermissions(ctx, mine.ID, orgA, false, dto.AddPermissionsToRoleDTO{PermissionIDs: e.ids("REPORTS_VIEW_ORG", "ORG_MEMBERS_MANAGE")}); err != nil {
		t.Fatalf("gán quyền thuộc phạm vi tổ chức bị chặn nhầm: %v", err)
	}
	if got := e.rolePerms(mine); !got["REPORTS_VIEW_ORG"] || !got["ORG_MEMBERS_MANAGE"] || len(got) != 2 {
		t.Errorf("quyền role sau Set = %v, muốn REPORTS_VIEW_ORG + ORG_MEMBERS_MANAGE", got)
	}
}

// Đúng kịch bản của review: tạo role, gán quyền hệ thống, tự nhận role. Nay bước 2 bị chặn; và nếu dữ
// liệu cũ đã chứa quyền hệ thống trong org role (gán trước khi vá) thì PermissionChecker cũng không cộng
// chúng vào tập quyền của người dùng.
func TestS6_OrgRole_ChuToChucKhongTuThanhAdminNenTang(t *testing.T) {
	e := newS6OrgEnv(t)
	owner := e.user("owner")
	ownerRole := e.orgRole(e.orgA, "ORG_OWNER", "ORG_ROLES_MANAGE", "ORG_MEMBERS_MANAGE")
	e.grant(owner, e.orgA, ownerRole)

	// Dữ liệu cũ: một org role đã mang quyền hệ thống (ghi thẳng DB như trước khi vá).
	poisoned := e.orgRole(e.orgA, "POISONED", "SYSTEM_SETTINGS_MANAGE", "PAYMENTS_MANAGE", "USERS_BAN", "REPORTS_VIEW_ORG")
	e.grant(owner, e.orgA, poisoned)

	for _, sys := range []string{"SYSTEM_SETTINGS_MANAGE", "PAYMENTS_MANAGE", "USERS_BAN"} {
		if e.has(owner, &e.orgA, sys) {
			t.Errorf("chủ tổ chức nhận quyền hệ thống %s qua org role", sys)
		}
	}
	for _, orgPerm := range []string{"ORG_ROLES_MANAGE", "ORG_MEMBERS_MANAGE", "REPORTS_VIEW_ORG"} {
		if !e.has(owner, &e.orgA, orgPerm) {
			t.Errorf("chủ tổ chức mất quyền thuộc phạm vi tổ chức %s", orgPerm)
		}
	}
	// Ngoài tổ chức đang active thì quyền org không có hiệu lực (giữ hành vi cũ).
	if e.has(owner, nil, "ORG_ROLES_MANAGE") {
		t.Error("quyền org role có hiệu lực dù không có active_org_id")
	}
}

// Mọi route /org-roles/:id/* phải kiểm role thuộc tổ chức của người gọi: chủ tổ chức A không đọc,
// không đổi quyền, không khôi phục role của tổ chức B, và không tạo role trong tổ chức B.
func TestS6_OrgRole_KhongDungDuocRoleCuaToChucKhac(t *testing.T) {
	e := newS6OrgEnv(t)
	ctx := context.Background()
	roleB := e.orgRole(e.orgB, "ROLE_B", "REPORTS_VIEW_ORG")
	orgA := &e.orgA.ID
	forbidden := func(name string, err error) {
		t.Helper()
		if !errors.Is(err, ErrNotRoleOrgMember) {
			t.Errorf("%s: err=%v, muốn ErrNotRoleOrgMember", name, err)
		}
	}

	_, err := e.roles.GetRoleByID(ctx, roleB.ID, orgA, false)
	forbidden("GetRoleByID", err)
	_, err = e.roles.GetRolePermissions(ctx, roleB.ID, orgA, false)
	forbidden("GetRolePermissions", err)
	forbidden("AddPermissions", e.roles.AddPermissionsToRole(ctx, roleB.ID, orgA, false, dto.AddPermissionsToRoleDTO{PermissionIDs: e.ids("ORG_ROLES_MANAGE")}))
	forbidden("SetPermissions", e.roles.SetRolePermissions(ctx, roleB.ID, orgA, false, dto.AddPermissionsToRoleDTO{PermissionIDs: e.ids("ORG_ROLES_MANAGE")}))
	forbidden("RemovePermissions", e.roles.RemovePermissionsFromRole(ctx, roleB.ID, orgA, false, dto.RemovePermissionsFromRoleDTO{PermissionIDs: e.ids("REPORTS_VIEW_ORG")}))
	if got := e.rolePerms(roleB); len(got) != 1 || !got["REPORTS_VIEW_ORG"] {
		t.Errorf("quyền role tổ chức B bị đổi: %v", got)
	}

	// Không có active_org_id (chưa chọn tổ chức) thì cũng không đụng được role nào.
	_, err = e.roles.GetRoleByID(ctx, roleB.ID, nil, false)
	forbidden("GetRoleByID khi không có active org", err)

	// Tạo role trong tổ chức khác: organization_id từ body phải khớp tổ chức đang active.
	_, err = e.roles.CreateRole(ctx, orgA, false, dto.CreateRoleDTO{Name: "Tro giang", OrganizationID: e.orgB.ID})
	forbidden("CreateRole org khác", err)
	if _, err := e.roles.CreateRole(ctx, orgA, false, dto.CreateRoleDTO{Name: "Tro giang", OrganizationID: e.orgA.ID}); err != nil {
		t.Errorf("CreateRole trong tổ chức của mình bị chặn nhầm: %v", err)
	}

	// Khôi phục: role đã xoá mềm của tổ chức B.
	if err := e.db.Delete(&model.Role{}, "id = ?", roleB.ID).Error; err != nil {
		t.Fatal(err)
	}
	forbidden("RestoreRole", e.roles.RestoreRole(ctx, roleB.ID, orgA, false))
	if err := e.roles.RestoreRole(ctx, roleB.ID, &e.orgB.ID, false); err != nil {
		t.Errorf("chủ tổ chức B khôi phục role của mình bị chặn nhầm: %v", err)
	}
	if err := e.roles.RestoreRole(ctx, uuid.New(), orgA, false); err == nil || errors.Is(err, ErrNotRoleOrgMember) {
		t.Errorf("khôi phục role không tồn tại: err=%v, muốn lỗi not found", err)
	}

	// Admin đi được mọi tổ chức.
	if _, err := e.roles.GetRoleByID(ctx, roleB.ID, nil, true); err != nil {
		t.Errorf("admin đọc role tổ chức B: %v", err)
	}
}

// Danh sách role: không phải admin thì luôn bị ép về tổ chức đang active; xin tổ chức khác là 403.
func TestS6_OrgRole_DanhSachChiThayToChucCuaMinh(t *testing.T) {
	e := newS6OrgEnv(t)
	ctx := context.Background()
	e.orgRole(e.orgA, "A1")
	e.orgRole(e.orgA, "A2")
	e.orgRole(e.orgB, "B1")
	orgA := &e.orgA.ID

	got, err := e.roles.GetAllRoles(ctx, 1, 50, "", "", nil, orgA, false)
	if err != nil || got.Total != 2 {
		t.Fatalf("chủ tổ chức A không lọc org: err=%v total=%v, muốn 2 role của tổ chức A", err, got)
	}
	for _, r := range got.Roles {
		if r.OrganizationID == nil || *r.OrganizationID != e.orgA.ID {
			t.Errorf("thấy role ngoài tổ chức A: %+v", r)
		}
	}
	if _, err := e.roles.GetAllRoles(ctx, 1, 50, "", "", &e.orgB.ID, orgA, false); !errors.Is(err, ErrNotRoleOrgMember) {
		t.Errorf("xin role tổ chức B: err=%v, muốn ErrNotRoleOrgMember", err)
	}
	if _, err := e.roles.GetAllRoles(ctx, 1, 50, "", "", nil, nil, false); !errors.Is(err, ErrNotRoleOrgMember) {
		t.Errorf("không có active org: err=%v, muốn ErrNotRoleOrgMember", err)
	}
	all, err := e.roles.GetAllRoles(ctx, 1, 50, "", "", nil, nil, true)
	if err != nil || all.Total != 3 {
		t.Errorf("admin: err=%v total=%v, muốn 3 role", err, all)
	}
}

// Route thành viên/vai trò của user: chủ tổ chức A không xem được vai trò của user ở tổ chức B, không
// liệt kê user của role tổ chức B, và không gán role không thuộc tổ chức nào vào tổ chức của mình.
func TestS6_OrgMember_ChiThayToChucCuaMinh(t *testing.T) {
	e := newS6OrgEnv(t)
	ctx := context.Background()
	member := e.user("member")
	roleA := e.orgRole(e.orgA, "A_ROLE", "REPORTS_VIEW_ORG")
	roleB := e.orgRole(e.orgB, "B_ROLE", "REPORTS_VIEW_ORG")
	e.grant(member, e.orgA, roleA)
	e.grant(member, e.orgB, roleB)
	orgA := &e.orgA.ID

	got, err := e.userOrgRoles.GetUserOrgRoles(ctx, member.ID, "", orgA, false)
	if err != nil || len(got) != 1 || got[0].OrganizationID != e.orgA.ID {
		t.Fatalf("vai trò của user thấy từ tổ chức A: err=%v got=%+v, muốn đúng 1 vai trò của tổ chức A", err, got)
	}
	if _, err := e.userOrgRoles.GetUserOrgRoles(ctx, member.ID, "", nil, false); !errors.Is(err, ErrOrgRoleForbidden) {
		t.Errorf("không có active org: err=%v, muốn ErrOrgRoleForbidden", err)
	}
	if all, err := e.userOrgRoles.GetUserOrgRoles(ctx, member.ID, "", nil, true); err != nil || len(all) != 2 {
		t.Errorf("admin: err=%v n=%d, muốn 2 vai trò", err, len(all))
	}

	if _, err := e.userOrgRoles.GetUsersWithOrgRoleByRoleID(ctx, roleB.ID, 1, 20, "", orgA, false); !errors.Is(err, ErrNotRoleOrgMember) {
		t.Errorf("liệt kê user của role tổ chức B từ tổ chức A: err=%v, muốn ErrNotRoleOrgMember", err)
	}
	if res, err := e.userOrgRoles.GetUsersWithOrgRoleByRoleID(ctx, roleA.ID, 1, 20, "", orgA, false); err != nil || res.Total != 1 {
		t.Errorf("liệt kê user của role tổ chức mình: err=%v res=%+v", err, res)
	}

	// Role không thuộc tổ chức nào (organization_id NULL) không được gán vào tổ chức của mình.
	global := model.Role{Name: "GLOBAL-" + uuid.NewString()[:6], Status: "active"}
	if err := e.db.Create(&global).Error; err != nil {
		t.Fatal(err)
	}
	other := e.user("other")
	_, err = e.userOrgRoles.AssignOrgRolesToUser(ctx, other.ID, dto.AssignOrgRolesToUserDTO{OrganizationID: e.orgA.ID, RoleIDs: []uuid.UUID{global.ID}}, member.ID, orgA, false)
	if err == nil {
		t.Error("gán role không thuộc tổ chức nào vào tổ chức A được chấp nhận")
	}
	if _, err := e.userOrgRoles.AssignOrgRolesToUser(ctx, other.ID, dto.AssignOrgRolesToUserDTO{OrganizationID: e.orgA.ID, RoleIDs: []uuid.UUID{roleA.ID}}, member.ID, orgA, false); err != nil {
		t.Errorf("gán role của tổ chức mình bị chặn nhầm: %v", err)
	}
}
