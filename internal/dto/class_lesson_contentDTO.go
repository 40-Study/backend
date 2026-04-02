package dto

import (
	"time"

	"github.com/google/uuid"
)

type AssignClassToContentDTO struct {
	ClassID     uuid.UUID `json:"class_id" validate:"required"`
	OpenDate    *string   `json:"open_date"`    // RFC3339
	DueDate     *string   `json:"due_date"`     // RFC3339
	ScheduledAt *string   `json:"scheduled_at"` // RFC3339 (livestream)
	EndAt       *string   `json:"end_at"`       // RFC3339 (livestream)
}

type BulkAssignClassesToContentDTO struct {
	ClassIDs    []uuid.UUID `json:"class_ids"`    // empty = all classes in course
	OpenDate    *string     `json:"open_date"`    // RFC3339
	DueDate     *string     `json:"due_date"`     // RFC3339
	ScheduledAt *string     `json:"scheduled_at"` // RFC3339 (livestream)
	EndAt       *string     `json:"end_at"`       // RFC3339 (livestream)
}

type UpdateClassContentScheduleDTO struct {
	OpenDate    *string `json:"open_date"`
	DueDate     *string `json:"due_date"`
	ScheduledAt *string `json:"scheduled_at"`
	EndAt       *string `json:"end_at"`
	Status      *string `json:"status" validate:"omitempty,oneof=scheduled live ended cancelled"`
}

type ClassLessonContentResponseDTO struct {
	ID              uuid.UUID  `json:"id"`
	ClassID         uuid.UUID  `json:"class_id"`
	LessonContentID uuid.UUID  `json:"lesson_content_id"`
	ContentType     string     `json:"content_type"`
	ContentTitle    *string    `json:"content_title,omitempty"`
	ClassName       string     `json:"class_name"`
	OpenDate        *time.Time `json:"open_date,omitempty"`
	DueDate         *time.Time `json:"due_date,omitempty"`
	ScheduledAt     *time.Time `json:"scheduled_at,omitempty"`
	EndAt           *time.Time `json:"end_at,omitempty"`
	Status          string     `json:"status"`
	CreatedAt       string     `json:"created_at"`
	UpdatedAt       string     `json:"updated_at"`
}

type ClassLessonContentListResponseDTO struct {
	Items    []ClassLessonContentResponseDTO `json:"items"`
	Total    int64                           `json:"total"`
	Page     int                             `json:"page"`
	PageSize int                             `json:"page_size"`
}
