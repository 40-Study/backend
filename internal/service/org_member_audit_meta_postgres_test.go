package service

// QA4-A02 (contract 6): metadata nhật ký gán / thu hồi vai trò tổ chức phải nêu tổ chức, vai trò và người được
// thao tác bằng tên. Chạy SQL THẬT (schema tạm).

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"study.com/v1/internal/dto"
	"study.com/v1/internal/model"
	"study.com/v1/internal/repository"
)

func TestOrgMemberAuditMeta_NamesOrgRoleAndUser(t *testing.T) {
	db := isolatedAPISchema(t)
	ctx := context.Background()
	org := model.Organization{Name: "Trường QA4 " + uuid.NewString()[:8]}
	if err := db.Create(&org).Error; err != nil {
		t.Fatal(err)
	}
	full := "Trần Thị B"
	user := model.User{Email: "qa4-a02-" + uuid.NewString() + "@40study.test", PasswordHash: "x", UserName: "qa4-a02-" + uuid.NewString()[:8], FullName: &full}
	if err := db.Create(&user).Error; err != nil {
		t.Fatal(err)
	}
	roleRepo := repository.NewRoleRepository(db)
	role, err := NewRoleService(roleRepo, repository.NewPermissionRepository(db)).CreateRole(ctx, nil, true, dto.CreateRoleDTO{Name: "Trợ giảng", OrganizationID: org.ID})
	if err != nil {
		t.Fatal(err)
	}
	svc := NewUserOrganizationRoleService(repository.NewUserOrganizationRoleRepository(db), repository.NewUserRepository(db),
		roleRepo, repository.NewOrganizationRepository(db), nil)
	assigned, err := svc.AssignOrgRolesToUser(ctx, user.ID, dto.AssignOrgRolesToUserDTO{OrganizationID: org.ID, RoleIDs: []uuid.UUID{role.ID}}, user.ID, nil, true)
	if err != nil || len(assigned) != 1 {
		t.Fatalf("gán: %v %+v", err, assigned)
	}
	want := map[string]any{
		"organization_id": org.ID.String(), "organization_name": org.Name,
		"role_code": role.ID.String(), "role_name": "Trợ giảng",
		"target_user_id": user.ID.String(), "target_user_name": full,
	}

	byRoles, err := svc.OrgMemberAuditMeta(ctx, user.ID, org.ID, []uuid.UUID{role.ID})
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.RevokeOrgRoleFromUser(ctx, user.ID, assigned[0].ID, user.ID, nil, true); err != nil {
		t.Fatal(err)
	}
	byAssignment, err := svc.OrgAssignmentAuditMeta(ctx, assigned[0].ID) // sau khi thu hồi, như handler gọi
	if err != nil {
		t.Fatal(err)
	}
	for name, got := range map[string]map[string]any{"gán": byRoles, "thu hồi": byAssignment} {
		if len(got) != len(want) {
			t.Errorf("%s: meta = %v, muốn %v", name, got, want)
		}
		for k, v := range want {
			if got[k] != v {
				t.Errorf("%s: meta[%s] = %v, muốn %v", name, k, got[k], v)
			}
		}
	}
}
