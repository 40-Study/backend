package model

import (
	"time"

	"github.com/google/uuid"
)

type Report struct {
	ID           uuid.UUID  `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	CreatedAt    time.Time  `json:"created_at"`
	ReporterID   uuid.UUID  `gorm:"type:uuid;not null;index" json:"reporter_id"`
	ReportedType string     `gorm:"type:varchar(30);not null;check:reported_type IN ('course', 'review', 'discussion', 'user')" json:"reported_type"`
	ReportedID   uuid.UUID  `gorm:"type:uuid;not null;index" json:"reported_id"`
	Reason       string     `gorm:"type:varchar(50);not null;check:reason IN ('spam', 'inappropriate', 'copyright', 'harassment', 'other')" json:"reason"`
	Description  *string    `gorm:"type:text" json:"description,omitempty"`
	Status       string     `gorm:"type:varchar(20);default:'pending';check:status IN ('pending', 'reviewing', 'resolved', 'dismissed')" json:"status"`
	AdminNotes   *string    `gorm:"type:text" json:"admin_notes,omitempty"`
	ResolvedBy   *uuid.UUID `gorm:"type:uuid" json:"resolved_by,omitempty"`
	ResolvedAt   *time.Time `json:"resolved_at,omitempty"`

	// Relationships
	Reporter User  `gorm:"foreignKey:ReporterID;constraint:OnDelete:CASCADE" json:"-"`
	Resolver *User `gorm:"foreignKey:ResolvedBy" json:"-"`
}

func (Report) TableName() string {
	return "reports"
}
