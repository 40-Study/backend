package seeds

import (
	"errors"
	"fmt"
	"log"
	"time"

	"gorm.io/gorm"
	"study.com/v1/internal/model"
	"study.com/v1/internal/repository"
)

// demoOrgMembership: chủ + thành viên của một tổ chức demo. QA4 A17: 5 tổ chức đã seed không có chủ hay thành viên nào,
// nên trang thành viên tổ chức của admin/chủ tổ chức trống. Required=true: tổ chức do SeedDemoOrganizations tạo, thiếu là
// lỗi. Required=false: tổ chức của bộ dữ liệu fortex.local (không seed ở repo này) — chưa có thì bỏ qua và ghi log.
type demoOrgMembership struct {
	Org      string
	Owner    string
	Teachers []string
	Students []string
	Required bool
}

var demoOrgMemberships = []demoOrgMembership{
	{Org: "Trung tâm Lập trình ABC", Owner: "teacher1@demo.com", Teachers: []string{"teacher2@demo.com"},
		Students: []string{"student1@demo.com", "student2@demo.com"}, Required: true},
	{Org: "Học viện CNTT XYZ", Owner: "teacher2@demo.com", Teachers: []string{"teacher1@demo.com"},
		Students: []string{"student2@demo.com", "student3@demo.com"}, Required: true},
	{Org: "Trung tâm ForteX Academy", Owner: "teacher1@fortex.local", Teachers: []string{"teacher2@fortex.local"},
		Students: []string{"student1@fortex.local", "student2@fortex.local"}},
	{Org: "Học viện Công nghệ Sao Việt", Owner: "teacher3@fortex.local", Teachers: []string{"teacher4@fortex.local"},
		Students: []string{"student3@fortex.local", "student4@fortex.local"}},
	{Org: "Trung tâm Ngoại ngữ Global Edu", Owner: "teacher5@fortex.local", Teachers: []string{"teacher6@fortex.local"},
		Students: []string{"student5@fortex.local", "student6@fortex.local"}},
}

// SeedDemoOrgMembers cấp vai tổ chức (ORG_OWNER / ORG_TEACHER / ORG_STUDENT, tạo bằng EnsureDefaultOrgRoles như lúc tạo tổ
// chức qua API) cho tài khoản demo. Idempotent theo (user, role, org); dòng đã có, kể cả đã thu hồi hay xoá mềm, giữ nguyên:
// đó là quyết định của người quản trị.
func (s *Seeder) SeedDemoOrgMembers() error {
	granted := 0
	for _, m := range demoOrgMemberships {
		var org model.Organization
		err := s.db.Where("name = ?", m.Org).First(&org).Error
		if errors.Is(err, gorm.ErrRecordNotFound) && !m.Required {
			log.Printf("Demo org members: bỏ qua %q (tổ chức chưa có trong DB)\n", m.Org)
			continue
		}
		if err != nil {
			return fmt.Errorf("demo org %q: %w", m.Org, err)
		}
		if _, err := repository.EnsureDefaultOrgRoles(s.db, org.ID); err != nil {
			return fmt.Errorf("demo org %q roles: %w", m.Org, err)
		}
		groups := []struct {
			role   string
			emails []string
		}{
			{repository.OrgOwnerRoleName, []string{m.Owner}},
			{repository.OrgTeacherRoleName, m.Teachers},
			{repository.OrgStudentRoleName, m.Students},
		}
		for _, g := range groups {
			for _, email := range g.emails {
				n, err := s.grantDemoOrgRole(org, g.role, email, m.Required)
				if err != nil {
					return fmt.Errorf("demo org %q: %w", m.Org, err)
				}
				granted += n
			}
		}
	}
	log.Printf("Seeded demo org members (%d new grants)\n", granted)
	return nil
}

// grantDemoOrgRole tạo một UserOrganizationRole active nếu chưa có. Trả 1 khi vừa tạo. Tài khoản thiếu: lỗi với tổ chức
// bắt buộc, bỏ qua (log) với tổ chức fortex.local.
func (s *Seeder) grantDemoOrgRole(org model.Organization, roleName, email string, required bool) (int, error) {
	var user model.User
	err := s.db.Where("email = ?", email).First(&user).Error
	if errors.Is(err, gorm.ErrRecordNotFound) && !required {
		log.Printf("Demo org members: bỏ qua %s ở %q (tài khoản chưa có)\n", email, org.Name)
		return 0, nil
	}
	if err != nil {
		return 0, fmt.Errorf("user %s: %w", email, err)
	}
	var role model.Role
	if err := s.db.Where("name = ? AND organization_id = ?", roleName, org.ID).First(&role).Error; err != nil {
		return 0, fmt.Errorf("role %s: %w", roleName, err)
	}
	var existing int64
	if err := s.db.Unscoped().Model(&model.UserOrganizationRole{}).
		Where("user_id = ? AND role_id = ? AND organization_id = ?", user.ID, role.ID, org.ID).Count(&existing).Error; err != nil {
		return 0, fmt.Errorf("check %s of %s: %w", roleName, email, err)
	}
	if existing > 0 {
		return 0, nil
	}
	row := model.UserOrganizationRole{UserID: user.ID, RoleID: role.ID, OrganizationID: org.ID,
		GrantedAt: time.Now(), Status: model.UserOrgRoleStatusActive}
	if err := s.db.Create(&row).Error; err != nil {
		return 0, fmt.Errorf("grant %s to %s: %w", roleName, email, err)
	}
	return 1, nil
}

// repairDemoTimestamps (QA4 A17): tài khoản/đơn hàng của bộ dữ liệu demo (demo.com, fortex.local) có last_login_at trước
// created_at và paid_at trước created_at. Suy lại từ created_at: lần đăng nhập cuối = created_at + khoảng lệch cũ (không quá
// hiện tại), thanh toán = 5 phút sau khi tạo đơn. Chỉ sửa dòng sai, nên chạy lại là no-op. Trả số dòng đã sửa.
func (s *Seeder) repairDemoTimestamps() (users, orders int64, err error) {
	const demoAccounts = "(email LIKE '%@demo.com' OR email LIKE '%@fortex.local')"
	res := s.db.Exec(`UPDATE users SET last_login_at = LEAST(now(), created_at + (created_at - last_login_at))
		WHERE last_login_at < created_at AND ` + demoAccounts)
	if res.Error != nil {
		return 0, 0, fmt.Errorf("repair demo last_login_at: %w", res.Error)
	}
	users = res.RowsAffected
	res = s.db.Exec(`UPDATE orders SET paid_at = created_at + interval '5 minutes'
		WHERE paid_at < created_at AND user_id IN (SELECT id FROM users WHERE ` + demoAccounts + `)`)
	if res.Error != nil {
		return users, 0, fmt.Errorf("repair demo order paid_at: %w", res.Error)
	}
	return users, res.RowsAffected, nil
}
