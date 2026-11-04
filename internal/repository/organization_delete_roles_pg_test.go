package repository

// QA3-02: xoá mềm tổ chức không gỡ user_organization_roles, nên GET /me/organizations nạp vai trò với Organization = nil
// và panic (500). Chạy SQL THẬT (schema tạm): DeleteOrganization phải gỡ vai trò trong cùng transaction, và các truy vấn
// đọc phải bỏ qua dòng gán của tổ chức đã xoá (dữ liệu mồ côi có sẵn trong DB cũ).

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"study.com/v1/internal/database"
	"study.com/v1/internal/model"
	"study.com/v1/internal/testutil/pgtest"
)

func orgWithMember(t *testing.T, db *gorm.DB, user model.User, tag string) (model.Organization, model.UserOrganizationRole) {
	t.Helper()
	org := model.Organization{Name: tag + "-" + uuid.NewString()[:8]}
	if err := db.Create(&org).Error; err != nil {
		t.Fatalf("tạo tổ chức: %v", err)
	}
	role := model.Role{Name: "ORG_OWNER", OrganizationID: &org.ID}
	if err := db.Create(&role).Error; err != nil {
		t.Fatalf("tạo role: %v", err)
	}
	uor := model.UserOrganizationRole{UserID: user.ID, RoleID: role.ID, OrganizationID: org.ID, Status: model.UserOrgRoleStatusActive}
	if err := db.Create(&uor).Error; err != nil {
		t.Fatalf("gán vai trò: %v", err)
	}
	return org, uor
}

func TestDeleteOrganization_Soft_RemovesMemberRoles(t *testing.T) {
	db := pgtest.IsolatedSchema(t, database.Migrate)
	ctx := context.Background()
	user := apvUser(t, db, "qa3-02-del")
	doomed, _ := orgWithMember(t, db, user, "doomed")
	kept, _ := orgWithMember(t, db, user, "kept")

	members, err := NewOrganizationRepository(db).ActiveMemberUserIDs(ctx, doomed.ID)
	if err != nil || len(members) != 1 || members[0] != user.ID {
		t.Fatalf("ActiveMemberUserIDs = %v (err=%v), muốn đúng [%s]", members, err, user.ID)
	}

	if err := NewOrganizationRepository(db).DeleteOrganization(ctx, doomed.ID, false); err != nil {
		t.Fatalf("DeleteOrganization: %v", err)
	}

	var live int64
	db.Model(&model.UserOrganizationRole{}).Where("organization_id = ?", doomed.ID).Count(&live)
	if live != 0 {
		t.Errorf("còn %d dòng gán vai trò chưa xoá của tổ chức đã xoá, muốn 0", live)
	}
	var total int64
	db.Unscoped().Model(&model.UserOrganizationRole{}).Where("organization_id = ?", doomed.ID).Count(&total)
	if total != 1 {
		t.Errorf("dòng gán phải được xoá MỀM (giữ lịch sử), unscoped có %d dòng, muốn 1", total)
	}

	roles, err := NewUserOrganizationRoleRepository(db).FindByUserIDWithDetails(ctx, user.ID, model.UserOrgRoleStatusActive)
	if err != nil {
		t.Fatal(err)
	}
	if len(roles) != 1 || roles[0].OrganizationID != kept.ID || roles[0].Organization == nil {
		t.Errorf("chỉ tổ chức còn sống được trả về, nhận %d dòng: %+v", len(roles), roles)
	}
}

// Dữ liệu cũ: tổ chức đã bị xoá mềm trực tiếp, dòng gán vẫn active (trạng thái thật trong DB dev trước bản sửa).
func TestUserOrgRoleReads_IgnoreSoftDeletedOrganization(t *testing.T) {
	db := pgtest.IsolatedSchema(t, database.Migrate)
	ctx := context.Background()
	user := apvUser(t, db, "qa3-02-stale")
	ghost, _ := orgWithMember(t, db, user, "ghost")
	live, _ := orgWithMember(t, db, user, "live")
	if err := db.Delete(&model.Organization{}, "id = ?", ghost.ID).Error; err != nil { // KHÔNG qua repo: giữ dòng gán mồ côi
		t.Fatal(err)
	}

	repo := NewUserOrganizationRoleRepository(db)

	byUser, err := repo.FindByUserIDWithDetails(ctx, user.ID, model.UserOrgRoleStatusActive)
	if err != nil {
		t.Fatal(err)
	}
	if len(byUser) != 1 || byUser[0].OrganizationID != live.ID {
		t.Errorf("FindByUserIDWithDetails phải bỏ tổ chức đã xoá, nhận %+v", byUser)
	}

	// /me/org-roles?org_id=<tổ chức đã xoá> cũng không được trả vai trò cũ.
	byOrg, err := repo.FindByUserAndOrganization(ctx, user.ID, ghost.ID, model.UserOrgRoleStatusActive)
	if err != nil {
		t.Fatal(err)
	}
	if len(byOrg) != 0 {
		t.Errorf("FindByUserAndOrganization(tổ chức đã xoá) trả %d dòng, muốn 0", len(byOrg))
	}
	byOrg, err = repo.FindByUserAndOrganization(ctx, user.ID, live.ID, model.UserOrgRoleStatusActive)
	if err != nil || len(byOrg) != 1 {
		t.Errorf("tổ chức còn sống phải trả 1 dòng, nhận %d (%v)", len(byOrg), err)
	}
}
