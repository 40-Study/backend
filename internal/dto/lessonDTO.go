package dto

import "github.com/google/uuid"

// Lesson DTOs

type CreateLessonDTO struct {
	Title       string  `json:"title" validate:"required,min=2,max=255"`
	Description *string `json:"description"`
	ContentType string  `json:"content_type" validate:"required,oneof=video article quiz assignment"`
	DurationMins *int   `json:"duration_minutes"`
	IsPreview   *bool   `json:"is_preview"`
	IsMandatory *bool   `json:"is_mandatory"`
}

type UpdateLessonDTO struct {
	Title        *string `json:"title" validate:"omitempty,min=2,max=255"`
	Description  *string `json:"description"`
	ContentType  *string `json:"content_type" validate:"omitempty,oneof=video article quiz assignment"`
	DurationMins *int    `json:"duration_minutes"`
	IsPreview    *bool   `json:"is_preview"`
	IsMandatory  *bool   `json:"is_mandatory"`
}

type LessonResponseDTO struct {
	ID           uuid.UUID `json:"id"`
	SectionID    uuid.UUID `json:"section_id"`
	Title        string    `json:"title"`
	Description  *string   `json:"description,omitempty"`
	ContentType  string    `json:"content_type"`
	DisplayOrder int       `json:"display_order"`
	DurationMins int       `json:"duration_minutes"`
	IsPreview    bool      `json:"is_preview"`
	IsMandatory  bool      `json:"is_mandatory"`
}
