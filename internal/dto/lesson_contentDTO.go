package dto

import (
	"time"

	"github.com/google/uuid"
)

// LessonContent DTOs
// Type: video, livestream, exercise

type CreateLessonContentDTO struct {
	Type         string     `json:"type" validate:"required,oneof=video livestream exercise"`
	Title        *string    `json:"title"`
	VideoURL     *string    `json:"video_url"`
	Duration     *int       `json:"duration"`
	ExerciseID   *uuid.UUID `json:"exercise_id"`
	IsMandatory  *bool      `json:"is_mandatory"`
	DisplayOrder *int       `json:"display_order"`
}

type UpdateLessonContentDTO struct {
	Type         *string    `json:"type" validate:"omitempty,oneof=video livestream exercise"`
	Title        *string    `json:"title"`
	VideoURL     *string    `json:"video_url"`
	Duration     *int       `json:"duration"`
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
	ExerciseID   *uuid.UUID `json:"exercise_id,omitempty"`
	IsMandatory  bool       `json:"is_mandatory"`
	DisplayOrder int        `json:"display_order"`
	CreatedAt    time.Time  `json:"created_at"`
	UpdatedAt    time.Time  `json:"updated_at"`
}
