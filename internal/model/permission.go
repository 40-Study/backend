package model

import (
	"database/sql"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type Permission struct {
	ID          uuid.UUID      `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	Name        string         `gorm:"type:varchar(100);uniqueIndex;not null" json:"name"`
	Description sql.NullString `gorm:"type:varchar(500)" json:"description,omitempty"`
	Status      string         `gorm:"type:varchar(20);default:'active';not null;index" json:"status"`
	CreatedAt   time.Time      `gorm:"autoCreateTime" json:"created_at"`
	UpdatedAt   time.Time      `gorm:"autoUpdateTime" json:"updated_at"`
	DeletedAt   gorm.DeletedAt `gorm:"index" json:"-"`

	// Relationships
	Roles []Role `gorm:"-" json:"-"`
}

func (Permission) TableName() string {
	return "permissions"
}

func (p *Permission) BeforeCreate(tx *gorm.DB) error {
	if p.ID == uuid.Nil {
		p.ID = uuid.New()
	}
	return nil
}
