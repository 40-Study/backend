package dto

import (
	"time"

	"github.com/google/uuid"
)

// ============================================================================
// CREATE
// ============================================================================

type CreatePersonalEventDTO struct {
	Title       string  `json:"title" validate:"required,min=1,max=255"`
	Description *string `json:"description,omitempty"`
	StartTime   string  `json:"start_time" validate:"required"`
	EndTime     string  `json:"end_time" validate:"required"`
	Color       *string `json:"color,omitempty" validate:"omitempty,max=50"`
	Location    *string `json:"location,omitempty" validate:"omitempty,max=255"`
	IsAllDay    bool    `json:"is_all_day"`
	ReminderMin *int    `json:"reminder_minutes,omitempty" validate:"omitempty,min=0,max=10080"`
}

// ============================================================================
// UPDATE
// ============================================================================

type UpdatePersonalEventDTO struct {
	Title       *string `json:"title,omitempty" validate:"omitempty,min=1,max=255"`
	Description *string `json:"description,omitempty"`
	StartTime   *string `json:"start_time,omitempty"`
	EndTime     *string `json:"end_time,omitempty"`
	Color       *string `json:"color,omitempty" validate:"omitempty,max=50"`
	Location    *string `json:"location,omitempty" validate:"omitempty,max=255"`
	IsAllDay    *bool   `json:"is_all_day,omitempty"`
	ReminderMin *int    `json:"reminder_minutes,omitempty" validate:"omitempty,min=0,max=10080"`
}

// ============================================================================
// RESPONSE
// ============================================================================

type PersonalEventResponseDTO struct {
	ID          uuid.UUID `json:"id"`
	UserID      uuid.UUID `json:"user_id"`
	Title       string    `json:"title"`
	Description *string   `json:"description,omitempty"`
	StartTime   time.Time `json:"start_time"`
	EndTime     time.Time `json:"end_time"`
	Color       *string   `json:"color,omitempty"`
	Location    *string   `json:"location,omitempty"`
	IsAllDay    bool      `json:"is_all_day"`
	ReminderMin *int      `json:"reminder_minutes,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}
