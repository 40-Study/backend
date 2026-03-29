package dto

import (
	"time"

	"github.com/google/uuid"
)

// LessonContent DTOs
// Type: video, livestream, exercise

type CreateLessonContentDTO struct {
	LessonID     uuid.UUID  `json:"lesson_id" validate:"required"`
	Type         string     `json:"type" validate:"required,oneof=video livestream exercise"`
	Title        *string    `json:"title"`
	VideoURL     *string    `json:"video_url"`
	Duration     *int       `json:"duration"`
	StreamID     *string    `json:"stream_id"`
	ExerciseID   *uuid.UUID `json:"exercise_id"`
	IsMandatory  *bool      `json:"is_mandatory"`
	DisplayOrder *int       `json:"display_order"`
}

type UpdateLessonContentDTO struct {
	Type         *string    `json:"type" validate:"omitempty,oneof=video livestream exercise"`
	Title        *string    `json:"title"`
	VideoURL     *string    `json:"video_url"`
	Duration     *int       `json:"duration"`
	StreamID     *string    `json:"stream_id"`
	ExerciseID   *uuid.UUID `json:"exercise_id"`
	IsMandatory  *bool      `json:"is_mandatory"`
	DisplayOrder *int       `json:"display_order"`
}

type LessonContentResponseDTO struct {
	ID           uuid.UUID  `json:"id"`
	LessonID     uuid.UUID  `json:"lesson_id"`
	Type         string     `json:"type"`
	Title        *string    `json:"title,omitempty"`
	VideoURL     *string    `json:"video_url,omitempty"`
	Duration     int        `json:"duration"`
	StreamID     *string    `json:"stream_id,omitempty"`
	ExerciseID   *uuid.UUID `json:"exercise_id,omitempty"`
	IsMandatory  bool       `json:"is_mandatory"`
	DisplayOrder int        `json:"display_order"`
	CreatedAt    time.Time  `json:"created_at"`
	UpdatedAt    time.Time  `json:"updated_at"`
}
