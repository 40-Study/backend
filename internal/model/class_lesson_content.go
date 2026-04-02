package model

import (
	"time"

	"github.com/google/uuid"
)

type ClassLessonContent struct {
	ID              uuid.UUID  `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	ClassID         uuid.UUID  `gorm:"type:uuid;not null;uniqueIndex:idx_class_lesson_content,priority:1;column:class_id"`
	LessonContentID uuid.UUID  `gorm:"type:uuid;not null;uniqueIndex:idx_class_lesson_content,priority:2;column:lesson_content_id"`
	OpenDate        *time.Time `gorm:"type:timestamp;column:open_date" json:"open_date,omitempty"`
	DueDate         *time.Time `gorm:"type:timestamp;column:due_date" json:"due_date,omitempty"`
	ScheduledAt     *time.Time `gorm:"type:timestamp;column:scheduled_at" json:"scheduled_at,omitempty"`
	EndAt           *time.Time `gorm:"type:timestamp;column:end_at" json:"end_at,omitempty"`
	Status          string     `gorm:"type:varchar(20);default:'scheduled'" json:"status"` // scheduled, live, ended, cancelled
	CreatedAt       time.Time  `gorm:"autoCreateTime" json:"created_at"`
	UpdatedAt       time.Time  `gorm:"autoUpdateTime" json:"updated_at"`

	// Relationships
	Class         Class         `gorm:"foreignKey:ClassID;constraint:OnDelete:CASCADE" json:"-"`
	LessonContent LessonContent `gorm:"foreignKey:LessonContentID;constraint:OnDelete:CASCADE" json:"-"`
}

func (ClassLessonContent) TableName() string {
	return "class_lesson_contents"
}
