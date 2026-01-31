package model

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type LessonVideo struct {
	gorm.Model
	ID                  uuid.UUID `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	LessonID            uuid.UUID `gorm:"type:uuid;uniqueIndex;not null" json:"lesson_id"`
	VideoURL            string    `gorm:"type:varchar(500);not null;column:video_url" json:"video_url"`
	VideoHlsURL         *string   `gorm:"type:varchar(500);column:video_hls_url" json:"video_hls_url,omitempty"`
	ThumbnailURL        *string   `gorm:"type:varchar(500);column:thumbnail_url" json:"thumbnail_url,omitempty"`
	DurationSeconds     int       `gorm:"not null" json:"duration_seconds"`
	Resolution          *string   `gorm:"type:varchar(20)" json:"resolution,omitempty"`
	FileSizeBytes       *int64    `gorm:"type:bigint" json:"file_size_bytes,omitempty"`
	Transcription       *string   `gorm:"type:text" json:"transcription,omitempty"`
	TranscriptionStatus string    `gorm:"type:varchar(20);default:'pending';check:transcription_status IN ('pending', 'processing', 'completed', 'failed')" json:"transcription_status"`

	// Relationships
	Lesson Lesson `gorm:"foreignKey:LessonID;constraint:OnDelete:CASCADE" json:"-"`
}

func (LessonVideo) TableName() string {
	return "lesson_videos"
}

type LessonArticle struct {
	gorm.Model
	ID              uuid.UUID `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	LessonID        uuid.UUID `gorm:"type:uuid;uniqueIndex;not null" json:"lesson_id"`
	Content         string    `gorm:"type:text;not null" json:"content"`
	ReadingTimeMins int       `gorm:"default:5;column:reading_time_minutes" json:"reading_time_minutes"`

	// Relationships
	Lesson Lesson `gorm:"foreignKey:LessonID;constraint:OnDelete:CASCADE" json:"-"`
}

func (LessonArticle) TableName() string {
	return "lesson_articles"
}

type LessonAttachment struct {
	ID            uuid.UUID `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	CreatedAt     time.Time `json:"created_at"`
	LessonID      uuid.UUID `gorm:"type:uuid;not null;index" json:"lesson_id"`
	FileName      string    `gorm:"type:varchar(255);not null" json:"file_name"`
	FileURL       string    `gorm:"type:varchar(500);not null;column:file_url" json:"file_url"`
	FileType      *string   `gorm:"type:varchar(50)" json:"file_type,omitempty"`
	FileSizeBytes *int64    `gorm:"type:bigint" json:"file_size_bytes,omitempty"`
	DownloadCount int       `gorm:"default:0" json:"download_count"`

	// Relationships
	Lesson Lesson `gorm:"foreignKey:LessonID;constraint:OnDelete:CASCADE" json:"-"`
}

func (LessonAttachment) TableName() string {
	return "lesson_attachments"
}
