package dto

import (
	"time"

	"github.com/google/uuid"
)

// Lesson DTOs

type CreateLessonDTO struct {
	Title        string    `json:"title" validate:"required,min=2,max=255"`
	Description  *string   `json:"description"`
	DisplayOrder int       `json:"display_order" validate:"min=0"`
	DurationMins *int      `json:"duration_minutes"`
	IsPreview    *bool     `json:"is_preview"`
	IsMandatory  *bool     `json:"is_mandatory"` // có ý nghĩa là học viên phải hoàn thành bài học này mới được xem các bài học khác trong cùng section hay không
}

type UpdateLessonDTO struct {
	Title        *string `json:"title" validate:"omitempty,min=2,max=255"`
	Description  *string `json:"description"`
	DisplayOrder *int    `json:"display_order" validate:"omitempty,min=0"`
	DurationMins *int    `json:"duration_minutes"`
	IsPreview    *bool   `json:"is_preview"`
	IsMandatory  *bool   `json:"is_mandatory"`
}

type LessonResponseDTO struct {
	ID           uuid.UUID                  `json:"id"`
	SectionID    uuid.UUID                  `json:"section_id"`
	Title        string                     `json:"title"`
	Description  *string                    `json:"description,omitempty"`
	DisplayOrder int                        `json:"display_order"`
	DurationMins int                        `json:"duration_minutes"`
	IsPreview    bool                       `json:"is_preview"`
	IsMandatory  bool                       `json:"is_mandatory"`
	Contents     []LessonContentResponseDTO `json:"contents,omitempty"`
	CreatedAt    time.Time                  `json:"created_at"`
	UpdatedAt    time.Time                  `json:"updated_at"`
}
