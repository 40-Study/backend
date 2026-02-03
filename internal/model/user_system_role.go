package model

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// UserSystemRole represents the assignment of system roles to users
type UserSystemRole struct {
	ID           uuid.UUID      `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	UserID       uuid.UUID      `gorm:"type:uuid;not null;uniqueIndex:idx_usr_user_role,priority:1;index:idx_usr_user_id" json:"user_id"`
	SystemRoleID uuid.UUID      `gorm:"type:uuid;not null;uniqueIndex:idx_usr_user_role,priority:2;index:idx_usr_system_role_id" json:"system_role_id"`
	GrantedAt    time.Time      `gorm:"default:CURRENT_TIMESTAMP" json:"granted_at"`
	GrantedBy    *uuid.UUID     `gorm:"type:uuid" json:"granted_by,omitempty"`
	Notes        *string        `gorm:"type:text" json:"notes,omitempty"`
	Status       string         `gorm:"type:varchar(20);default:'active';not null;index:idx_usr_status" json:"status"`
	RevokedBy    *uuid.UUID     `gorm:"type:uuid" json:"revoked_by,omitempty"`
	RevokedAt    *time.Time     `json:"revoked_at,omitempty"`
	CreatedAt    time.Time      `json:"created_at"`
	UpdatedAt    time.Time      `json:"updated_at"`
	DeletedAt    gorm.DeletedAt `gorm:"index" json:"deleted_at,omitempty"`

	// Relationships
	User       *User       `gorm:"foreignKey:UserID;constraint:OnDelete:CASCADE" json:"user,omitempty"`
	SystemRole *SystemRole `gorm:"foreignKey:SystemRoleID;constraint:OnDelete:CASCADE" json:"system_role,omitempty"`
	Granter    *User       `gorm:"foreignKey:GrantedBy" json:"granter,omitempty"`
	Revoker    *User       `gorm:"foreignKey:RevokedBy" json:"revoker,omitempty"`
}

func (UserSystemRole) TableName() string {
	return "user_system_roles"
}

// UserSystemRole status constants
const (
	UserSystemRoleStatusActive    = "active"
	UserSystemRoleStatusSuspended = "suspended"
	UserSystemRoleStatusRevoked   = "revoked"
)
