package model

import (
	"time"

	"github.com/google/uuid"
)

type RolePermission struct {
	RoleID       uuid.UUID `gorm:"type:uuid;primaryKey" json:"role_id"`
	PermissionID uuid.UUID `gorm:"type:uuid;primaryKey;index:idx_permission_id" json:"permission_id"`
	CreatedAt    time.Time `gorm:"autoCreateTime" json:"created_at"`

	// Relationships
	Role       Role       `gorm:"foreignKey:RoleID;constraint:OnDelete:CASCADE" json:"role,omitempty"`
	Permission Permission `gorm:"foreignKey:PermissionID;constraint:OnDelete:CASCADE" json:"permission,omitempty"`
}

func (RolePermission) TableName() string {
	return "role_permissions"
}
