package dto

import (
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

// Enrollment DTOs

type EnrollmentResponseDTO struct {
	ID              uuid.UUID       `json:"id"`
	UserID          uuid.UUID       `json:"user_id"`
	CourseID        uuid.UUID       `json:"course_id"`
	EnrolledAt      string          `json:"enrolled_at"`
	ProgressPercent decimal.Decimal `json:"progress_percentage"`
	CompletedAt     *string         `json:"completed_at,omitempty"`
	LastAccessedAt  *string         `json:"last_accessed_at,omitempty"`
	CourseTitle     string          `json:"course_title,omitempty"`
	CourseSlug      string          `json:"course_slug,omitempty"`
	CourseThumbnail *string         `json:"course_thumbnail,omitempty"`
	CourseCategory  string          `json:"course_category,omitempty"`
	CourseStatus    string          `json:"course_status,omitempty"`
	CourseDeleted   bool            `json:"course_deleted"`
}

type EnrollmentDetailDTO struct {
	EnrollmentResponseDTO
	LessonProgress []LessonProgressResponseDTO `json:"lesson_progress,omitempty"`
}

type EnrollmentListResponseDTO struct {
	Enrollments []EnrollmentResponseDTO `json:"enrollments"`
	Total       int64                   `json:"total"`
	Page        int                     `json:"page"`
	PageSize    int                     `json:"page_size"`
}

// LessonProgress DTOs

type UpdateLessonProgressDTO struct {
	Status           *string          `json:"status" validate:"omitempty,oneof=not_started in_progress completed"`
	ProgressPercent  *decimal.Decimal `json:"progress_percentage"`
	VideoWatchedSecs *int             `json:"video_watched_seconds"`
}

type LessonProgressResponseDTO struct {
	ID               uuid.UUID       `json:"id"`
	UserID           uuid.UUID       `json:"user_id"`
	LessonID         uuid.UUID       `json:"lesson_id"`
	EnrollmentID     uuid.UUID       `json:"enrollment_id"`
	Status           string          `json:"status"`
	ProgressPercent  decimal.Decimal `json:"progress_percentage"`
	VideoWatchedSecs int             `json:"video_watched_seconds"`
	CompletedAt      *string         `json:"completed_at,omitempty"`
	LastAccessedAt   string          `json:"last_accessed_at"`
}

// Course Enrollments (Instructor view)

type CourseEnrollmentItemDTO struct {
	ID              uuid.UUID       `json:"id"`
	UserID          uuid.UUID       `json:"user_id"`
	UserEmail       string          `json:"user_email"`
	UserName        string          `json:"user_name"`
	EnrolledAt      string          `json:"enrolled_at"`
	ProgressPercent decimal.Decimal `json:"progress_percentage"`
	CompletedAt     *string         `json:"completed_at,omitempty"`
}

type CourseEnrollmentListDTO struct {
	Enrollments []CourseEnrollmentItemDTO `json:"enrollments"`
	Total       int64                     `json:"total"`
	Page        int                       `json:"page"`
	PageSize    int                       `json:"page_size"`
}

// Debug DTO (includes soft-deleted)

type DebugEnrollmentDTO struct {
	ID         uuid.UUID `json:"id"`
	UserID     uuid.UUID `json:"user_id"`
	UserEmail  string    `json:"user_email"`
	CourseID   uuid.UUID `json:"course_id"`
	EnrolledAt string    `json:"enrolled_at"`
	IsDeleted  bool      `json:"is_deleted"`
	DeletedAt  *string   `json:"deleted_at,omitempty"`
}
