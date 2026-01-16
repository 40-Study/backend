package model

import (
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"gorm.io/gorm"
)

// 34. Role (Bảng vai trò)
type Role struct {
	ID          uuid.UUID      `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	Name        string         `gorm:"unique;not null" json:"name"`
	Description pgtype.Text    `json:"description"`
	CreatedAt   time.Time      `gorm:"autoCreateTime" json:"created_at"`
	UpdatedAt   time.Time      `gorm:"autoUpdateTime" json:"updated_at"`
	DeletedAt   gorm.DeletedAt `gorm:"index" json:"deleted_at"`

	Permissions []Permission `gorm:"many2many:role_permissions;" json:"permissions"`
}

func (Role) TableName() string {
	return "roles"
}

// 30. Permission (Bảng quyền hạn)
type Permission struct {
	ID          uuid.UUID      `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	Name        string         `gorm:"unique;not null" json:"name"`
	Description pgtype.Text    `json:"description"`
	CreatedAt   time.Time      `gorm:"autoCreateTime" json:"created_at"`
	UpdatedAt   time.Time      `gorm:"autoUpdateTime" json:"updated_at"`
	DeletedAt   gorm.DeletedAt `gorm:"index" json:"deleted_at"`
}

func (Permission) TableName() string {
	return "permissions"
}

// 43. UserRole (Bảng vai trò người dùng)
type UserRole struct {
	ID         uuid.UUID      `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	UserID     uuid.UUID      `gorm:"type:uuid;not null;index" json:"user_id"`
	RoleID     uuid.UUID      `gorm:"type:uuid;not null;index" json:"role_id"`
	AssignedAt time.Time      `gorm:"autoCreateTime" json:"assigned_at"`
	DeletedAt  gorm.DeletedAt `gorm:"index" json:"deleted_at"`
}

func (UserRole) TableName() string {
	return "user_roles"
}

// 35. RolePermission (Bảng quyền vai trò)
type RolePermission struct {
	ID           uuid.UUID      `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	RoleID       uuid.UUID      `gorm:"type:uuid;not null;index:role_perm_idx,unique" json:"role_id"`
	PermissionID uuid.UUID      `gorm:"type:uuid;not null;index:role_perm_idx,unique" json:"permission_id"`
	CreatedAt    time.Time      `gorm:"autoCreateTime" json:"created_at"`
	DeletedAt    gorm.DeletedAt `gorm:"index" json:"deleted_at"`
}

func (RolePermission) TableName() string {
	return "role_permissions"
}

// 3. ActivityLog (Bảng nhật ký hoạt động người dùng)
type ActivityLog struct {
	ID           uuid.UUID              `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	UserIDPtr    uuid.UUID              `gorm:"type:uuid;column:user_id" json:"user_id"`
	ActivityType string                 `gorm:"not null" json:"activity_type"`
	ActivityName string                 `gorm:"not null" json:"activity_name"`
	Description  pgtype.Text            `json:"description"`
	IpAddress    pgtype.Text            `json:"ip_address"`
	UserAgent    pgtype.Text            `json:"user_agent"`
	DeviceType   pgtype.Text            `json:"device_type"`
	Location     pgtype.Text            `json:"location"`
	Success      pgtype.Bool            `gorm:"default:true" json:"success"`
	Metadata     map[string]interface{} `gorm:"type:jsonb;serializer:json" json:"metadata"`
	CreatedAt    time.Time              `gorm:"autoCreateTime" json:"created_at"`
}

func (ActivityLog) TableName() string {
	return "activity_logs"
}
