package model

import (
	"database/sql"

	"github.com/google/uuid"
)

type Role struct {
	BaseModel
	Name           string         `gorm:"type:varchar(100);not null;uniqueIndex:idx_role_name_org" json:"name"`
	OrganizationID uuid.UUID      `gorm:"type:uuid;not null;uniqueIndex:idx_role_name_org;index:idx_role_org_id" json:"organization_id"`
	Description    sql.NullString `gorm:"type:varchar(500)" json:"description,omitempty"`
	Status         string         `gorm:"type:varchar(20);default:'active';not null;index" json:"status"`

	// Relationships
	Organization *Organization `gorm:"foreignKey:OrganizationID;constraint:OnDelete:CASCADE" json:"organization,omitempty"`
	Permissions  []Permission  `gorm:"-" json:"permissions,omitempty"`
}

func (Role) TableName() string {
	return "roles"
}
