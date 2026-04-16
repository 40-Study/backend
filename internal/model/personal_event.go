package model

import (
	"time"

	"github.com/google/uuid"
)

type PersonalEvent struct {
	ID          uuid.UUID `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
	UserID      uuid.UUID `gorm:"type:uuid;not null;index" json:"user_id"`
	Title       string    `gorm:"type:varchar(255);not null" json:"title"`
	Description *string   `gorm:"type:text" json:"description,omitempty"`
	StartTime   time.Time `gorm:"not null" json:"start_time"`
	EndTime     time.Time `gorm:"not null" json:"end_time"`
	Color       *string   `gorm:"type:varchar(50)" json:"color,omitempty"`
	Location    *string   `gorm:"type:varchar(255)" json:"location,omitempty"`
	IsAllDay    bool      `gorm:"default:false" json:"is_all_day"`
	ReminderMin *int      `json:"reminder_minutes,omitempty"`

	User User `gorm:"foreignKey:UserID;constraint:OnDelete:CASCADE" json:"-"`
}

func (PersonalEvent) TableName() string { return "personal_events" }
