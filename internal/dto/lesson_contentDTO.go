package dto

import "github.com/google/uuid"

// LessonVideo DTOs

type CreateLessonVideoDTO struct {
	VideoURL        string  `json:"video_url" validate:"required,safe_url"`
	VideoHlsURL     *string `json:"video_hls_url" validate:"omitempty,safe_url"`
	ThumbnailURL    *string `json:"thumbnail_url" validate:"omitempty,safe_url"`
	DurationSeconds int     `json:"duration_seconds" validate:"required,min=1"`
	Resolution      *string `json:"resolution"`
	FileSizeBytes   *int64  `json:"file_size_bytes"`
}

type UpdateLessonVideoDTO struct {
	VideoURL        *string `json:"video_url" validate:"omitempty,safe_url"`
	VideoHlsURL     *string `json:"video_hls_url" validate:"omitempty,safe_url"`
	ThumbnailURL    *string `json:"thumbnail_url" validate:"omitempty,safe_url"`
	DurationSeconds *int    `json:"duration_seconds" validate:"omitempty,min=1"`
	Resolution      *string `json:"resolution"`
	FileSizeBytes   *int64  `json:"file_size_bytes"`
}

type LessonVideoResponseDTO struct {
	ID                  uuid.UUID `json:"id"`
	LessonID            uuid.UUID `json:"lesson_id"`
	VideoURL            string    `json:"video_url"`
	VideoHlsURL         *string   `json:"video_hls_url,omitempty"`
	ThumbnailURL        *string   `json:"thumbnail_url,omitempty"`
	DurationSeconds     int       `json:"duration_seconds"`
	Resolution          *string   `json:"resolution,omitempty"`
	FileSizeBytes       *int64    `json:"file_size_bytes,omitempty"`
	TranscriptionStatus string    `json:"transcription_status"`
	CreatedAt           string    `json:"created_at"`
	UpdatedAt           string    `json:"updated_at"`
}

// LessonArticle DTOs

type CreateLessonArticleDTO struct {
	Content         string `json:"content" validate:"required"`
	ReadingTimeMins *int   `json:"reading_time_minutes"`
}

type UpdateLessonArticleDTO struct {
	Content         *string `json:"content"`
	ReadingTimeMins *int    `json:"reading_time_minutes"`
}

type LessonArticleResponseDTO struct {
	ID              uuid.UUID `json:"id"`
	LessonID        uuid.UUID `json:"lesson_id"`
	Content         string    `json:"content"`
	ReadingTimeMins int       `json:"reading_time_minutes"`
	CreatedAt       string    `json:"created_at"`
	UpdatedAt       string    `json:"updated_at"`
}

// LessonAttachment DTOs

type CreateLessonAttachmentDTO struct {
	FileName      string  `json:"file_name" validate:"required,min=1,max=255"`
	FileURL       string  `json:"file_url" validate:"required,safe_url"`
	FileType      *string `json:"file_type"`
	FileSizeBytes *int64  `json:"file_size_bytes"`
}

type LessonAttachmentResponseDTO struct {
	ID            uuid.UUID `json:"id"`
	LessonID      uuid.UUID `json:"lesson_id"`
	FileName      string    `json:"file_name"`
	FileURL       string    `json:"file_url"`
	FileType      *string   `json:"file_type,omitempty"`
	FileSizeBytes *int64    `json:"file_size_bytes,omitempty"`
	DownloadCount int       `json:"download_count"`
	CreatedAt     string    `json:"created_at"`
}
