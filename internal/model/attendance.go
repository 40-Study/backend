package model

import (
	"time"

	"github.com/google/uuid"
)

type Attendance struct {
	ID        uuid.UUID `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	ClassID   uuid.UUID `gorm:"type:uuid;not null;uniqueIndex:idx_class_student_date,priority:1;column:class_id"`
	StudentID uuid.UUID `gorm:"type:uuid;not null;uniqueIndex:idx_class_student_date,priority:2;column:student_id"`
	Date      time.Time `gorm:"type:date;not null;uniqueIndex:idx_class_student_date,priority:3" json:"date"`
	Status    string    `gorm:"type:varchar(20);not null" json:"status"` // present, absent, late, excused
	Note      *string   `gorm:"type:text" json:"note,omitempty"`
	CreatedAt time.Time `gorm:"autoCreateTime" json:"created_at"`

	// Relationships
	Class   Class `gorm:"foreignKey:ClassID;constraint:OnDelete:CASCADE" json:"-"`
	Student User  `gorm:"foreignKey:StudentID;constraint:OnDelete:CASCADE" json:"-"`
}

func (Attendance) TableName() string {
	return "attendances"
}
