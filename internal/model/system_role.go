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
