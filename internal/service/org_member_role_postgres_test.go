package service

// Lane R5, B-16 (Postgres thật, schema tạm):
//   - GET /organizations/:id/members trước đây chỉ có user_id, UI không hiện được người. Nay mỗi dòng mang user
//     (tên, email), bỏ Preload("User") hoặc bỏ nhánh uor.User trong toUserOrgRoleResponseDTO thì test ĐỎ.
//   - xoá org role còn người đang giữ trước đây trả 200 và để lại bản ghi gán "active" trỏ vào role đã xoá.
//     Nay ErrRoleInUse (handler 409); gỡ hết người rồi mới xoá được. Bỏ kiểm CountActiveAssignments thì ĐỎ.

import (
	"context"
	"errors"
	"testing"

	"study.com/v1/internal/model"
)

func TestR5_OrgMembers_CoTenVaEmail_Postgres(t *testing.T) {
	e := newS6OrgEnv(t)
	ctx := context.Background()
	member := e.user("member-with-name")
	e.grant(member, e.orgA, e.orgRole(e.orgA, "THANH_VIEN"))

	out, err := e.userOrgRoles.GetOrganizationMembers(ctx, e.orgA.ID, 1, 20, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(out.UserOrgRoles) != 1 {
		t.Fatalf("có %d thành viên, muốn 1", len(out.UserOrgRoles))
	}
	row := out.UserOrgRoles[0]
	if row.User == nil {
		t.Fatalf("thành viên thiếu thông tin người dùng (chỉ có user_id=%v)", row.UserID)
	}
	if row.User.ID != member.ID || row.User.UserName != member.UserName || row.User.Email != member.Email {
		t.Errorf("user=%+v, muốn id=%v user_name=%q email=%q", row.User, member.ID, member.UserName, member.Email)
	}
	if row.Role == nil {
		t.Error("thành viên thiếu role")
	}
}

func TestR5_XoaOrgRoleDangDuocGan_409_Postgres(t *testing.T) {
	e := newS6OrgEnv(t)
	ctx := context.Background()
	holder := e.user("holder")
	role := e.orgRole(e.orgA, "TRO_GIANG")
	e.grant(holder, e.orgA, role)
	orgA := &e.orgA.ID

	if err := e.roles.DeleteRole(ctx, role.ID, orgA, false, false); !errors.Is(err, ErrRoleInUse) {
		t.Fatalf("xoá role còn người giữ: err=%v, muốn ErrRoleInUse", err)
	}
	if err := e.roles.DeleteRole(ctx, role.ID, orgA, false, true); !errors.Is(err, ErrRoleInUse) {
		t.Fatalf("xoá cứng role còn người giữ: err=%v, muốn ErrRoleInUse", err)
	}
	var stillThere model.Role
	if err := e.db.First(&stillThere, "id = ?", role.ID).Error; err != nil {
		t.Fatalf("role đã bị xoá dù còn người giữ: %v", err)
	}

	// Gỡ vai trò khỏi người giữ (status inactive) thì xoá được; bản ghi gán cũ không còn active nên không treo.
	if err := e.db.Model(&model.UserOrganizationRole{}).Where("role_id = ?", role.ID).
		Update("status", model.UserOrgRoleStatusInactive).Error; err != nil {
		t.Fatal(err)
	}
	if err := e.roles.DeleteRole(ctx, role.ID, orgA, false, false); err != nil {
		t.Fatalf("xoá role không còn ai giữ: %v", err)
	}

	// Role không có ai giữ từ đầu cũng xoá được.
	free := e.orgRole(e.orgA, "RONG")
	if err := e.roles.DeleteRole(ctx, free.ID, orgA, false, false); err != nil {
		t.Errorf("xoá role trống: %v", err)
	}
}
