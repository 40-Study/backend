package model

import (
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"gorm.io/gorm"
)

type Enrollment struct {
	gorm.Model
	ID              uuid.UUID       `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	UserID          uuid.UUID       `gorm:"type:uuid;not null;index;uniqueIndex:idx_user_course" json:"user_id"`
	CourseID        uuid.UUID       `gorm:"type:uuid;not null;index;uniqueIndex:idx_user_course" json:"course_id"`
	EnrolledAt      time.Time       `gorm:"default:CURRENT_TIMESTAMP" json:"enrolled_at"`
	ExpiresAt       *time.Time      `json:"expires_at,omitempty"`
	ProgressPercent decimal.Decimal `gorm:"type:decimal(5,2);default:0;column:progress_percentage" json:"progress_percentage"`
	CompletedAt     *time.Time      `json:"completed_at,omitempty"`
	LastAccessedAt  *time.Time      `json:"last_accessed_at,omitempty"`
	CertificateID   *uuid.UUID      `gorm:"type:uuid" json:"certificate_id,omitempty"`

	// Relationships
	User           User             `gorm:"foreignKey:UserID;constraint:OnDelete:CASCADE" json:"-"`
	Course         Course           `gorm:"foreignKey:CourseID;constraint:OnDelete:CASCADE" json:"-"`
	Certificate    *Certificate     `gorm:"foreignKey:CertificateID" json:"-"`
	LessonProgress []LessonProgress `gorm:"foreignKey:EnrollmentID;constraint:OnDelete:CASCADE" json:"-"`
}

func (Enrollment) TableName() string {
	return "enrollments"
}

type LessonProgress struct {
	gorm.Model
	ID               uuid.UUID       `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	UserID           uuid.UUID       `gorm:"type:uuid;not null;index;uniqueIndex:idx_user_lesson" json:"user_id"`
	LessonID         uuid.UUID       `gorm:"type:uuid;not null;index;uniqueIndex:idx_user_lesson" json:"lesson_id"`
	EnrollmentID     uuid.UUID       `gorm:"type:uuid;not null;index" json:"enrollment_id"`
	Status           string          `gorm:"type:varchar(20);default:'not_started';check:status IN ('not_started', 'in_progress', 'completed');index" json:"status"`
	ProgressPercent  decimal.Decimal `gorm:"type:decimal(5,2);default:0;column:progress_percentage" json:"progress_percentage"`
	VideoWatchedSecs int             `gorm:"default:0;column:video_watched_seconds" json:"video_watched_seconds"`
	CompletedAt      *time.Time      `json:"completed_at,omitempty"`
	LastAccessedAt   time.Time       `gorm:"default:CURRENT_TIMESTAMP" json:"last_accessed_at"`

	// Relationships
	User       User       `gorm:"foreignKey:UserID;constraint:OnDelete:CASCADE" json:"-"`
	Lesson     Lesson     `gorm:"foreignKey:LessonID;constraint:OnDelete:CASCADE" json:"-"`
	Enrollment Enrollment `gorm:"foreignKey:EnrollmentID;constraint:OnDelete:CASCADE" json:"-"`
}

func (LessonProgress) TableName() string {
	return "lesson_progress"
}

type UserNote struct {
	gorm.Model
	ID                 uuid.UUID `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	UserID             uuid.UUID `gorm:"type:uuid;not null;index" json:"user_id"`
	LessonID           uuid.UUID `gorm:"type:uuid;not null;index" json:"lesson_id"`
	Content            string    `gorm:"type:text;not null" json:"content"`
	VideoTimestampSecs *int      `gorm:"column:video_timestamp_seconds" json:"video_timestamp_seconds,omitempty"`
	IsBookmarked       bool      `gorm:"default:false" json:"is_bookmarked"`

	// Relationships
	User   User   `gorm:"foreignKey:UserID;constraint:OnDelete:CASCADE" json:"-"`
	Lesson Lesson `gorm:"foreignKey:LessonID;constraint:OnDelete:CASCADE" json:"-"`
}

func (UserNote) TableName() string {
	return "user_notes"
}
