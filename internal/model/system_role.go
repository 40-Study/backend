package model

import (
	"database/sql"
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