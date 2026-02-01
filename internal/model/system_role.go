package model

import (
	"database/sql"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type SystemRole struct {
	ID          uuid.UUID      `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	Name        string         `gorm:"type:varchar(100);uniqueIndex;not null" json:"name"`
	Description sql.NullString `gorm:"type:varchar(500)" json:"description,omitempty"`
	Status      string         `gorm:"type:varchar(20);default:'active';not null;index" json:"status"`
	CreatedAt   time.Time      `gorm:"autoCreateTime" json:"created_at"`
	UpdatedAt   time.Time      `gorm:"autoUpdateTime" json:"updated_at"`
	DeletedAt   gorm.DeletedAt `gorm:"index" json:"-"`

	// Relationships
	Permissions []Permission `gorm:"-" json:"permissions,omitempty"`
}

func (SystemRole) TableName() string {
	return "system_roles"
}

func (r *SystemRole) BeforeCreate(tx *gorm.DB) error {
	if r.ID == uuid.Nil {
		r.ID = uuid.New()
	}
	return nil
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
