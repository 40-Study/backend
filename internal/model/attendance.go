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

	// Enhanced tracking fields
	CheckInTime       *time.Time `json:"check_in_time,omitempty"`
	CheckOutTime      *time.Time `json:"check_out_time,omitempty"`
	ExpectedTime      *string    `gorm:"type:time" json:"expected_time,omitempty"`
	LateMinutes       int        `gorm:"default:0" json:"late_minutes"`
	EarlyLeaveMinutes int        `gorm:"default:0" json:"early_leave_minutes"`
	Location          *string    `gorm:"type:varchar(50)" json:"location,omitempty"` // online, offline, room_name
	DeviceInfo        *string    `gorm:"type:jsonb" json:"device_info,omitempty"`
	VerifiedBy        *uuid.UUID `gorm:"type:uuid" json:"verified_by,omitempty"`

	// Relationships
	Class    Class `gorm:"foreignKey:ClassID;constraint:OnDelete:CASCADE" json:"-"`
	Student  User  `gorm:"foreignKey:StudentID;constraint:OnDelete:CASCADE" json:"-"`
	Verifier *User `gorm:"foreignKey:VerifiedBy" json:"-"`
}

func (Attendance) TableName() string {
	return "attendances"
}
