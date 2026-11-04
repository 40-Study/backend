package service

// QA3-22: tên vai trò tổ chức trùng trả 400 "duplicated key not allowed" (lỗi unique của GORM lọt ra), gán lại cùng vai
// trò trả 409 nhưng tiếng Anh thô. Cả hai phải là 409 với sentinel riêng và thông báo tiếng Việt. Chạy SQL THẬT.

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/google/uuid"
	"study.com/v1/internal/dto"
	"study.com/v1/internal/model"
	"study.com/v1/internal/repository"
)

func TestOrgRole_DuplicateNameAndReassign_AreVietnameseConflicts(t *testing.T) {
	db := isolatedAPISchema(t)
	ctx := context.Background()

	org := model.Organization{Name: "QA3-22 org " + uuid.NewString()[:8]}
	if err := db.Create(&org).Error; err != nil {
		t.Fatal(err)
	}
	user := model.User{Email: "qa3-22-" + uuid.NewString() + "@40study.test", PasswordHash: "x", UserName: "qa3-22-" + uuid.NewString()[:8]}
	if err := db.Create(&user).Error; err != nil {
		t.Fatal(err)
	}

	roleRepo := repository.NewRoleRepository(db)
	roles := NewRoleService(roleRepo, repository.NewPermissionRepository(db))
	uorSvc := NewUserOrganizationRoleService(repository.NewUserOrganizationRoleRepository(db), repository.NewUserRepository(db),
		roleRepo, repository.NewOrganizationRepository(db), nil)

	first, err := roles.CreateRole(ctx, nil, true, dto.CreateRoleDTO{Name: "Trợ giảng", OrganizationID: org.ID})
	if err != nil {
		t.Fatalf("tạo vai trò đầu: %v", err)
	}

	// Tạo trùng tên trong cùng tổ chức.
	_, err = roles.CreateRole(ctx, nil, true, dto.CreateRoleDTO{Name: "Trợ giảng", OrganizationID: org.ID})
	if !errors.Is(err, ErrRoleNameTaken) {
		t.Fatalf("tạo trùng tên: err = %v, muốn ErrRoleNameTaken", err)
	}
	if strings.Contains(err.Error(), "duplicated key") || !strings.Contains(err.Error(), "Tên vai trò") {
		t.Errorf("thông báo phải là tiếng Việt, nhận %q", err.Error())
	}

	// Đổi tên sang tên đã có.
	second, err := roles.CreateRole(ctx, nil, true, dto.CreateRoleDTO{Name: "Giám sát", OrganizationID: org.ID})
	if err != nil {
		t.Fatal(err)
	}
	taken := "Trợ giảng"
	if _, err := roles.UpdateRole(ctx, second.ID, nil, true, dto.UpdateRoleDTO{Name: &taken}); !errors.Is(err, ErrRoleNameTaken) {
		t.Errorf("đổi sang tên đã có: err = %v, muốn ErrRoleNameTaken", err)
	}

	// Tên cũ trong tổ chức KHÁC vẫn dùng được (unique theo (tên, tổ chức)).
	other := model.Organization{Name: "QA3-22 other " + uuid.NewString()[:8]}
	if err := db.Create(&other).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := roles.CreateRole(ctx, nil, true, dto.CreateRoleDTO{Name: "Trợ giảng", OrganizationID: other.ID}); err != nil {
		t.Errorf("cùng tên ở tổ chức khác phải được: %v", err)
	}

	// Gán lần 1 ok, lần 2 là xung đột tiếng Việt kèm tên vai trò.
	assign := dto.AssignOrgRolesToUserDTO{OrganizationID: org.ID, RoleIDs: []uuid.UUID{first.ID}}
	if _, err := uorSvc.AssignOrgRolesToUser(ctx, user.ID, assign, user.ID, nil, true); err != nil {
		t.Fatalf("gán lần 1: %v", err)
	}
	_, err = uorSvc.AssignOrgRolesToUser(ctx, user.ID, assign, user.ID, nil, true)
	if !errors.Is(err, ErrOrgRoleAlreadyAssigned) {
		t.Fatalf("gán lần 2: err = %v, muốn ErrOrgRoleAlreadyAssigned", err)
	}
	if strings.Contains(err.Error(), "already assigned") || !strings.Contains(err.Error(), "Trợ giảng") {
		t.Errorf("thông báo phải tiếng Việt và nêu tên vai trò, nhận %q", err.Error())
	}
}
