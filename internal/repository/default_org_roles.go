package repository

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"study.com/v1/data"
	"study.com/v1/internal/model"
)

// QA-C-01 (261110): tổ chức mới chỉ có vai ORG_OWNER nên chủ tổ chức (và cả admin) không có vai nào để thêm giảng viên
// hay học viên vào tổ chức, kéo theo không gán được giảng viên vào lớp của tổ chức. Mỗi tổ chức có sẵn bộ vai mặc định.
//
// Quyền: chỉ quyền phạm vi tổ chức (data/permissions/org_owner_permissions.json) mới có nghĩa với org role
// (PermissionChecker lọc bỏ quyền khác), và đó đều là quyền QUẢN TRỊ tổ chức. Giảng viên và học viên không quản trị
// tổ chức nên vai của họ không mang quyền nào: là thành viên active là đủ để được gán dạy lớp của tổ chức
// (requireOrgMemberTeacher) hay học lớp; quyền dạy (COURSES_CREATE...) vẫn đến từ system role TEACHER. Tên vai trùng
// mã system role để web hiện đúng nhãn "Giảng viên"/"Học viên" (getOrgRoleLabel) mà không cần trường tên hiển thị mới.
type defaultOrgRole struct {
	name        string
	description string
	permissions []string
}

// OrgOwnerRoleName: vai chủ tổ chức, được cấp cho người tạo tổ chức.
const OrgOwnerRoleName = "ORG_OWNER"

func defaultOrgRoles() []defaultOrgRole {
	return []defaultOrgRole{
		{OrgOwnerRoleName, "Chủ tổ chức: quản lý thành viên, vai trò, danh mục và báo cáo của tổ chức", data.OrgPermissionNames()},
		{"TEACHER", "Giảng viên của tổ chức: được gán dạy các lớp thuộc tổ chức", nil},
		{"STUDENT", "Học viên của tổ chức", nil},
	}
}

// EnsureDefaultOrgRoles tạo các vai mặc định còn THIẾU của một tổ chức (idempotent), trả số vai vừa tạo. Vai đã có cùng
// tên (kể cả đã xoá mềm, do unique idx_role_name_org) được giữ nguyên, không đụng quyền hay trạng thái: đó là quyết
// định của người quản trị. Chỉ vai vừa tạo mới được gán quyền mặc định. Không cấp vai cho ai.
func EnsureDefaultOrgRoles(tx *gorm.DB, orgID uuid.UUID) (int, error) {
	created := 0
	for _, spec := range defaultOrgRoles() {
		role := model.Role{
			Name:           spec.name,
			OrganizationID: &orgID,
			Description:    sql.NullString{String: spec.description, Valid: true},
			Status:         model.RoleStatusActive,
		}
		// ON CONFLICT DO NOTHING thay cho kiểm-rồi-tạo: hai tiến trình chạy cùng lúc không làm hỏng transaction.
		res := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&role)
		if res.Error != nil {
			return created, fmt.Errorf("create org role %s: %w", spec.name, res.Error)
		}
		if res.RowsAffected == 0 {
			continue
		}
		created++
		if len(spec.permissions) == 0 {
			continue
		}
		var perms []model.Permission
		if err := tx.Where("name IN ?", spec.permissions).Find(&perms).Error; err != nil {
			return created, err
		}
		for _, perm := range perms {
			rp := model.RolePermission{RoleID: role.ID, PermissionID: perm.ID}
			if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&rp).Error; err != nil {
				return created, err
			}
		}
	}
	return created, nil
}

// BackfillDefaultOrgRoles chạy EnsureDefaultOrgRoles cho mọi tổ chức chưa xoá (lúc khởi động). Chỉ THÊM vai còn thiếu,
// không gán cho ai và không sửa vai có sẵn, nên chạy lại bao nhiêu lần cũng như nhau. Mỗi tổ chức một transaction để
// một tổ chức lỗi không chặn các tổ chức khác.
func BackfillDefaultOrgRoles(ctx context.Context, db *gorm.DB) (int, error) {
	var orgIDs []uuid.UUID
	if err := db.WithContext(ctx).Model(&model.Organization{}).Pluck("id", &orgIDs).Error; err != nil {
		return 0, err
	}
	total := 0
	for _, id := range orgIDs {
		err := db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
			n, err := EnsureDefaultOrgRoles(tx, id)
			if err == nil {
				total += n
			}
			return err
		})
		if err != nil {
			return total, fmt.Errorf("organization %s: %w", id, err)
		}
	}
	return total, nil
}
