package model

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// UserOrganizationRole represents the assignment of organization roles to users
type UserOrganizationRole struct {
	ID             uuid.UUID      `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	UserID         uuid.UUID      `gorm:"type:uuid;not null;uniqueIndex:idx_uor_user_role_org,priority:1;index:idx_uor_user_id" json:"user_id"`
	RoleID         uuid.UUID      `gorm:"type:uuid;not null;uniqueIndex:idx_uor_user_role_org,priority:2;index:idx_uor_role_id" json:"role_id"`
	OrganizationID uuid.UUID      `gorm:"type:uuid;not null;uniqueIndex:idx_uor_user_role_org,priority:3;index:idx_uor_org_id" json:"organization_id"`
	GrantedAt      time.Time      `gorm:"default:CURRENT_TIMESTAMP" json:"granted_at"`
	GrantedBy      *uuid.UUID     `gorm:"type:uuid" json:"granted_by,omitempty"`
	Notes          *string        `gorm:"type:text" json:"notes,omitempty"`
	Status         string         `gorm:"type:varchar(20);default:'active';not null;index:idx_uor_status" json:"status"`
	RevokedBy      *uuid.UUID     `gorm:"type:uuid" json:"revoked_by,omitempty"`
	RevokedAt      *time.Time     `json:"revoked_at,omitempty"`
	CreatedAt      time.Time      `json:"created_at"`
	UpdatedAt      time.Time      `json:"updated_at"`
	DeletedAt      gorm.DeletedAt `gorm:"index" json:"deleted_at,omitempty"`

	// Relationships
	User         *User         `gorm:"foreignKey:UserID;constraint:OnDelete:CASCADE" json:"user,omitempty"`
	Role         *Role         `gorm:"foreignKey:RoleID;constraint:OnDelete:CASCADE" json:"role,omitempty"`
	Organization *Organization `gorm:"foreignKey:OrganizationID;constraint:OnDelete:CASCADE" json:"organization,omitempty"`
	Granter      *User         `gorm:"foreignKey:GrantedBy" json:"granter,omitempty"`
	Revoker      *User         `gorm:"foreignKey:RevokedBy" json:"revoker,omitempty"`
}

func (UserOrganizationRole) TableName() string {
	return "user_organization_roles"
}

// UserOrganizationRole status constants
const (
	UserOrgRoleStatusActive    = "active"
	UserOrgRoleStatusSuspended = "suspended"
	UserOrgRoleStatusRevoked   = "revoked"
)
