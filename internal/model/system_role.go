package model

import (
	"database/sql"
	"strings"
	"time"

	"github.com/google/uuid"
)

type SystemRole struct {
	BaseModel
	Name        string         `gorm:"type:varchar(100);uniqueIndex;not null" json:"name"`
	Description sql.NullString `gorm:"type:varchar(500)" json:"description,omitempty"`
	Status      string         `gorm:"type:varchar(20);default:'active';not null;index" json:"status"`

	// Relationships
	Permissions []Permission `gorm:"-" json:"permissions,omitempty"`
}

func (SystemRole) TableName() string {
	return "system_roles"
}

type SystemRolePermission struct {
	SystemRoleID uuid.UUID `gorm:"type:uuid;primaryKey" json:"system_role_id"`
	PermissionID uuid.UUID `gorm:"type:uuid;primaryKey;index:idx_sys_role_perm_id" json:"permission_id"`
	CreatedAt    time.Time `gorm:"autoCreateTime" json:"created_at"`

	// Relationships
	SystemRole SystemRole `gorm:"foreignKey:SystemRoleID;constraint:OnDelete:CASCADE" json:"system_role,omitempty"`
	Permission Permission `gorm:"foreignKey:PermissionID;constraint:OnDelete:CASCADE" json:"permission,omitempty"`
}

func (SystemRolePermission) TableName() string {
	return "system_role_permissions"
}

// SystemRolePermissionSeed ghi nhớ cặp (role, quyền) mà seeder ĐÃ cấp ít nhất một lần (S2).
// Không có bảng này, seeder không phân biệt được "chưa từng cấp" với "admin đã chủ động gỡ": cả hai
// đều là thiếu dòng trong system_role_permissions, nên mỗi lần khởi động seeder cấp lại quyền admin
// vừa gỡ. Seeder chỉ cấp cặp CHƯA có dấu ở đây, rồi đánh dấu — nhờ vậy quyền mới trong roles.json
// vẫn tới role (cặp chưa được đánh dấu), còn quyền admin gỡ tay thì được giữ (cặp đã đánh dấu).
type SystemRolePermissionSeed struct {
	SystemRoleID uuid.UUID `gorm:"type:uuid;primaryKey" json:"system_role_id"`
	PermissionID uuid.UUID `gorm:"type:uuid;primaryKey" json:"permission_id"`
	CreatedAt    time.Time `gorm:"autoCreateTime" json:"created_at"`
}

func (SystemRolePermissionSeed) TableName() string {
	return "system_role_permission_seeds"
}

// BuiltInSystemRoleNames: vai trò hệ thống do seed (data/roles.json) tạo và code đăng nhập / phân
// quyền / đăng ký phụ thuộc theo TÊN. Xoá một vai trò trong số này làm hỏng nền tảng (QA A1) nên
// API quản trị từ chối. Phải khớp roles.json — test trong package service pin sự khớp đó.
var BuiltInSystemRoleNames = []string{
	"SYSTEM_ADMIN", "ORG_OWNER", "TEACHER", "TEACHER_APPLICANT", "STUDENT", "PARENT",
}

// OrgRoleNameReserved (review #144, phát hiện 1): tên org role không được trùng tên vai hệ thống dựng sẵn. Chọn org role lúc
// đăng nhập thì TÊN vai thành claim active_role, mà một số cổng chỉ so chuỗi claim (open-design đòi "TEACHER", lesson
// code và contest đòi "STUDENT", DenyActiveRole chặn "PARENT"). Vai tên TEACHER trong tổ chức sẽ biến người được gán
// thành "giảng viên" ở các cổng đó dù chưa được admin duyệt. So sau khi trim + viết hoa để "teacher " cũng bị chặn.
//
// ORG_OWNER là ngoại lệ ở bước cấp token (OrgRoleNameBlockedForToken): đó chính là org role chủ tổ chức do hệ thống tạo
// (CreateOrganization, backfill) và web đọc đúng claim này để vào khu /org; không cổng backend nào cấp quyền theo chuỗi
// "ORG_OWNER" (quyền chủ tổ chức luôn tính từ permission trong DB). API tạo/đổi tên vai vẫn chặn cả ORG_OWNER.
func OrgRoleNameReserved(name string) bool {
	return IsBuiltInSystemRole(strings.ToUpper(strings.TrimSpace(name)))
}

// OrgRoleNameBlockedForToken: org role có tên này không được cấp thành active_role (xem OrgRoleNameReserved).
func OrgRoleNameBlockedForToken(name string) bool {
	return OrgRoleNameReserved(name) && strings.ToUpper(strings.TrimSpace(name)) != "ORG_OWNER"
}

// IsBuiltInSystemRole báo name có phải vai trò hệ thống dựng sẵn không (so khớp chính xác, vì
// code phụ thuộc đúng chuỗi này).
func IsBuiltInSystemRole(name string) bool {
	for _, n := range BuiltInSystemRoleNames {
		if n == name {
			return true
		}
	}
	return false
}
