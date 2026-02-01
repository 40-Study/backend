package model

import (
	"database/sql"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type Role struct {
	ID             uuid.UUID      `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	Name           string         `gorm:"type:varchar(100);not null;uniqueIndex:idx_role_name_org" json:"name"`
	OrganizationID uuid.UUID      `gorm:"type:uuid;not null;uniqueIndex:idx_role_name_org;index:idx_role_org_id" json:"organization_id"`
	Description    sql.NullString `gorm:"type:varchar(500)" json:"description,omitempty"`
	Status         string         `gorm:"type:varchar(20);default:'active';not null;index" json:"status"`
	CreatedAt      time.Time      `gorm:"autoCreateTime" json:"created_at"`
	UpdatedAt      time.Time      `gorm:"autoUpdateTime" json:"updated_at"`
	DeletedAt      gorm.DeletedAt `gorm:"index" json:"-"`

	// Relationships
	Organization *Organization `gorm:"foreignKey:OrganizationID;constraint:OnDelete:CASCADE" json:"organization,omitempty"`
	Permissions  []Permission  `gorm:"-" json:"permissions,omitempty"`
}

func (Role) TableName() string {
	return "roles"
}

func (r *Role) BeforeCreate(tx *gorm.DB) error {
	if r.ID == uuid.Nil {
		r.ID = uuid.New()
	}
	return nil
}
