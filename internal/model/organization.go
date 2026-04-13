package model

import "database/sql"

type Organization struct {
	BaseModel
	Name        string         `gorm:"type:varchar(100);uniqueIndex;not null" json:"name"`
	Description sql.NullString `gorm:"type:varchar(500)" json:"description,omitempty"`
	Status      string         `gorm:"type:varchar(20);default:'active';not null;index" json:"status"`
	Timezone    *string        `gorm:"type:varchar(50);default:'Asia/Ho_Chi_Minh'" json:"timezone,omitempty"` // IANA timezone

	// Relationships
	Roles []Role `gorm:"foreignKey:OrganizationID" json:"roles,omitempty"`
}

func (Organization) TableName() string {
	return "organizations"
}
