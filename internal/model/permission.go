package model

import "database/sql"

type Permission struct {
	BaseModel
	Name        string         `gorm:"type:varchar(100);uniqueIndex;not null" json:"name"`
	Description sql.NullString `gorm:"type:varchar(500)" json:"description,omitempty"`
	Status      string         `gorm:"type:varchar(20);default:'active';not null;index" json:"status"`

	// Relationships
	Roles []Role `gorm:"-" json:"-"`
}

func (Permission) TableName() string {
	return "permissions"
}
