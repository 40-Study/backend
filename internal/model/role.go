package model

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// Role represents a role in the system (student, parent, teacher, admin, etc.)
// Based on 'roles' table in schema
type Role struct {
	ID             uuid.UUID      `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	Code           string         `gorm:"type:varchar(50);uniqueIndex;not null" json:"code"`
	Name           string         `gorm:"type:varchar(100);not null" json:"name"`
	Description    *string        `gorm:"type:text" json:"description,omitempty"`
	IsSystemRole   bool           `gorm:"default:false;column:is_system_role" json:"is_system_role"`
	OrganizationID *uuid.UUID     `gorm:"type:uuid;column:organization_id" json:"organization_id,omitempty"`
	PriorityLevel  int            `gorm:"default:0;column:priority_level" json:"priority_level"`
	IsActive       bool           `gorm:"default:true;column:is_active" json:"is_active"`
	CreatedAt      time.Time      `json:"created_at"`
	UpdatedAt      time.Time      `json:"updated_at"`
	DeletedAt      gorm.DeletedAt `gorm:"index" json:"deleted_at,omitempty"`

	// Relationships
	UserRoles []UserRole `gorm:"foreignKey:RoleID" json:"-"`
}

func (Role) TableName() string {
	return "roles"
}
