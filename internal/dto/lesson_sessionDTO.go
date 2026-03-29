package dto

import (
	"time"

	"github.com/google/uuid"
)

// LessonSession DTOs
// Status: scheduled, live, ended, cancelled

type CreateLessonSessionDTO struct {
	LessonID    uuid.UUID `json:"lesson_id" validate:"required"`
	Title       string    `json:"title" validate:"required,min=2,max=255"`
	Description *string   `json:"description"`
	StartTime   time.Time `json:"start_time" validate:"required"`
	EndTime     *time.Time `json:"end_time"`
	MeetingURL  *string   `json:"meeting_url"`
	MeetingID   *string   `json:"meeting_id"`
	HostID      *string   `json:"host_id"`
}

type UpdateLessonSessionDTO struct {
	Title       *string    `json:"title" validate:"omitempty,min=2,max=255"`
	Description *string    `json:"description"`
	StartTime   *time.Time `json:"start_time"`
	EndTime     *time.Time `json:"end_time"`
	MeetingURL  *string    `json:"meeting_url"`
	MeetingID   *string    `json:"meeting_id"`
	HostID      *string    `json:"host_id"`
	Status      *string    `json:"status" validate:"omitempty,oneof=scheduled live ended cancelled"`
}

type LessonSessionResponseDTO struct {
	ID          uuid.UUID  `json:"id"`
	LessonID    uuid.UUID  `json:"lesson_id"`
	Title       string     `json:"title"`
	Description *string    `json:"description,omitempty"`
	StartTime   time.Time  `json:"start_time"`
	EndTime     *time.Time `json:"end_time,omitempty"`
	MeetingURL  *string    `json:"meeting_url,omitempty"`
	MeetingID   *string    `json:"meeting_id,omitempty"`
	HostID      *string    `json:"host_id,omitempty"`
	Status      string     `json:"status"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
}
