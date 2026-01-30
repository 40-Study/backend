package model

import (
	"time"

	"github.com/google/uuid"
)

// UserRole represents the many-to-many relationship between users and roles
// Based on 'user_roles' junction table in schema
type UserRole struct {
	ID             uuid.UUID  `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	UserID         uuid.UUID  `gorm:"type:uuid;not null;index" json:"user_id"`
	RoleID         uuid.UUID  `gorm:"type:uuid;not null;index" json:"role_id"`
	OrganizationID *uuid.UUID `gorm:"type:uuid;index;column:organization_id" json:"organization_id,omitempty"`
	GrantedAt      time.Time  `gorm:"default:CURRENT_TIMESTAMP;column:granted_at" json:"granted_at"`
	ExpiresAt      *time.Time `gorm:"column:expires_at" json:"expires_at,omitempty"`
	GrantedBy      *uuid.UUID `gorm:"type:uuid;column:granted_by" json:"granted_by,omitempty"`
	Notes          *string    `gorm:"type:text" json:"notes,omitempty"`
	CreatedAt      time.Time  `json:"created_at"`

	// Relationships (for eager loading)
	User *User `gorm:"foreignKey:UserID" json:"user,omitempty"`
	Role *Role `gorm:"foreignKey:RoleID" json:"role,omitempty"`
}

func (UserRole) TableName() string {
	return "user_roles"
}

// UserRoleUniqueIndex represents the unique constraint: UNIQUE(user_id, role_id, organization_id)
// This is handled by GORM migration or manually via SQL
