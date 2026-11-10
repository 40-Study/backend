package service

// QA-C-01 (261110, Postgres thật, schema tạm): tổ chức mới chỉ có vai "Chủ tổ chức" nên không thêm được giảng viên hay
// học viên. Bộ vai mặc định (ORG_OWNER kèm quyền chủ tổ chức, ORG_TEACHER, ORG_STUDENT không mang quyền quản trị) phải có ở tổ
// chức mới, chủ tổ chức thêm được thành viên bằng API gán org role đang có, và tổ chức cũ được bổ sung idempotent.
// Bỏ EnsureDefaultOrgRoles khỏi CreateOrganization thì ca "tổ chức mới" ĐỎ; bỏ ON CONFLICT hoặc gán quyền cho vai đã có
// thì ca backfill ĐỎ.

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"study.com/v1/internal/dto"
	"study.com/v1/internal/model"
	"study.com/v1/internal/repository"
)

func orgRolePerms(t *testing.T, e *s6OrgEnv, orgID uuid.UUID) map[string][]string {
	t.Helper()
	var roles []model.Role
	if err := e.db.Where("organization_id = ?", orgID).Find(&roles).Error; err != nil {
		t.Fatal(err)
	}
	out := map[string][]string{}
	for _, r := range roles {
		var names []string
		e.db.Model(&model.Permission{}).Joins("JOIN role_permissions rp ON rp.permission_id = permissions.id").
			Where("rp.role_id = ?", r.ID).Order("permissions.name").Pluck("permissions.name", &names)
		out[r.Name] = names
	}
	return out
}

func TestQAC01_ToChucMoiCoVaiMacDinh_ChuToChucThemDuocThanhVien_Postgres(t *testing.T) {
	e := newS6OrgEnv(t)
	ctx := context.Background()
	creator, teacher, student := e.user("org-creator"), e.user("org-teacher"), e.user("org-student")

	orgSvc := NewOrganizationService(repository.NewOrganizationRepository(e.db))
	created, err := orgSvc.CreateOrganization(ctx, creator.ID, dto.CreateOrganizationDTO{Name: "QA-261110-Org " + uuid.NewString()[:6]})
	if err != nil {
		t.Fatalf("tạo tổ chức: %v", err)
	}
	perms := orgRolePerms(t, e, created.ID)
	// s6AllPerms chỉ seed 3 quyền phạm vi tổ chức: ORG_MEMBERS_MANAGE, ORG_ROLES_MANAGE, REPORTS_VIEW_ORG.
	if got := perms["ORG_OWNER"]; len(got) != 3 || got[0] != "ORG_MEMBERS_MANAGE" {
		t.Errorf("ORG_OWNER có quyền %v, muốn đủ 3 quyền phạm vi tổ chức đã seed", got)
	}
	for _, name := range []string{repository.OrgTeacherRoleName, repository.OrgStudentRoleName} {
		got, ok := perms[name]
		if !ok {
			t.Fatalf("tổ chức mới thiếu vai %s (có %v)", name, perms)
		}
		if len(got) != 0 {
			t.Errorf("vai %s không được mang quyền quản trị tổ chức, có %v", name, got)
		}
	}

	// Chủ tổ chức (active_org_id = tổ chức mới) thêm giảng viên và học viên bằng API gán org role đang có.
	if ok, err := e.checker.HasPermission(ctx, creator.ID, &created.ID, "ORG_MEMBERS_MANAGE"); err != nil || !ok {
		t.Fatalf("chủ tổ chức không có ORG_MEMBERS_MANAGE: ok=%v err=%v", ok, err)
	}
	roleID := func(name string) uuid.UUID {
		var r model.Role
		if err := e.db.Where("organization_id = ? AND name = ?", created.ID, name).First(&r).Error; err != nil {
			t.Fatal(err)
		}
		return r.ID
	}
	for u, role := range map[*model.User]string{&teacher: repository.OrgTeacherRoleName, &student: repository.OrgStudentRoleName} {
		req := dto.AssignOrgRolesToUserDTO{OrganizationID: created.ID, RoleIDs: []uuid.UUID{roleID(role)}}
		if _, err := e.userOrgRoles.AssignOrgRolesToUser(ctx, u.ID, req, creator.ID, &created.ID, false); err != nil {
			t.Fatalf("chủ tổ chức thêm %s: %v", role, err)
		}
		// Thành viên mới không được quyền quản trị tổ chức.
		if ok, _ := e.checker.HasPermission(ctx, u.ID, &created.ID, "ORG_MEMBERS_MANAGE"); ok {
			t.Errorf("thành viên vai %s lại có ORG_MEMBERS_MANAGE", role)
		}
	}
	var members int64
	e.db.Model(&model.UserOrganizationRole{}).Where("organization_id = ? AND status = ?", created.ID, model.UserOrgRoleStatusActive).Count(&members)
	if members != 3 {
		t.Errorf("tổ chức có %d thành viên active, muốn 3 (chủ + giảng viên + học viên)", members)
	}
}

func TestQAC01_BackfillVaiMacDinh_Idempotent_Postgres(t *testing.T) {
	e := newS6OrgEnv(t) // orgA, orgB tạo thẳng DB, không có vai nào (như tổ chức seed "Học viện CNTT XYZ")
	ctx := context.Background()

	// orgB đã có ORG_TEACHER do admin tự tạo, mang quyền riêng, rồi xoá mềm: backfill phải tôn trọng, không tạo lại,
	// không khôi phục, không đổi quyền.
	custom := model.Role{Name: repository.OrgTeacherRoleName, OrganizationID: &e.orgB.ID, Status: "active"}
	mustCreate(t, e.db, &custom)
	mustCreate(t, e.db, &model.RolePermission{RoleID: custom.ID, PermissionID: e.permIDs["REPORTS_VIEW_ORG"]})
	e.db.Delete(&custom)

	n, err := repository.BackfillDefaultOrgRoles(ctx, e.db)
	if err != nil {
		t.Fatalf("backfill: %v", err)
	}
	if n != 5 { // orgA: 3 vai, orgB: ORG_OWNER + ORG_STUDENT
		t.Errorf("backfill tạo %d vai, muốn 5", n)
	}
	if a := orgRolePerms(t, e, e.orgA.ID); len(a) != 3 || len(a["ORG_OWNER"]) != 3 {
		t.Errorf("orgA sau backfill: %v", a)
	}
	b := orgRolePerms(t, e, e.orgB.ID)
	if _, ok := b[repository.OrgTeacherRoleName]; ok {
		t.Errorf("vai ORG_TEACHER đã xoá mềm của orgB bị tạo lại/khôi phục: %v", b)
	}
	var kept []string
	e.db.Model(&model.Permission{}).Joins("JOIN role_permissions rp ON rp.permission_id = permissions.id").
		Where("rp.role_id = ?", custom.ID).Pluck("permissions.name", &kept)
	if len(kept) != 1 || kept[0] != "REPORTS_VIEW_ORG" {
		t.Errorf("quyền của vai có sẵn bị đổi: %v", kept)
	}

	if again, err := repository.BackfillDefaultOrgRoles(ctx, e.db); err != nil || again != 0 {
		t.Errorf("chạy lại backfill: tạo %d vai, err=%v, muốn 0", again, err)
	}
	var unassigned int64
	e.db.Model(&model.UserOrganizationRole{}).Where("organization_id IN ?", []uuid.UUID{e.orgA.ID, e.orgB.ID}).Count(&unassigned)
	if unassigned != 0 {
		t.Errorf("backfill không được gán vai cho ai, có %d dòng", unassigned)
	}
}

// QA-C-20: xoá tổ chức không còn thu hồi phiên (user_version), nên phải chứng minh token cũ mang active_org_id của tổ
// chức đã xoá KHÔNG còn quyền tổ chức nào: quyền tính lại từ DB mỗi request. Cả xoá mềm lẫn xoá cứng.
func TestQAC20_XoaToChucThiTokenCuMatQuyenToChuc_Postgres(t *testing.T) {
	e := newS6OrgEnv(t)
	ctx := context.Background()
	orgSvc := NewOrganizationService(repository.NewOrganizationRepository(e.db))
	for _, hard := range []bool{false, true} {
		owner := e.user("owner-del")
		org, err := orgSvc.CreateOrganization(ctx, owner.ID, dto.CreateOrganizationDTO{Name: "QA-261110-Del " + uuid.NewString()[:6]})
		if err != nil {
			t.Fatal(err)
		}
		if ok, _ := e.checker.HasPermission(ctx, owner.ID, &org.ID, "ORG_MEMBERS_MANAGE"); !ok {
			t.Fatalf("hard=%v: chủ tổ chức chưa có quyền trước khi xoá", hard)
		}
		if err := orgSvc.DeleteOrganization(ctx, org.ID, hard); err != nil {
			t.Fatalf("hard=%v: xoá: %v", hard, err)
		}
		for _, perm := range []string{"ORG_MEMBERS_MANAGE", "ORG_ROLES_MANAGE", "REPORTS_VIEW_ORG"} {
			if ok, err := e.checker.HasPermission(ctx, owner.ID, &org.ID, perm); err != nil || ok {
				t.Errorf("hard=%v: token cũ (active_org_id = tổ chức đã xoá) vẫn có %s: ok=%v err=%v", hard, perm, ok, err)
			}
		}
	}
}
