package dto

import (
	"time"

	"github.com/google/uuid"
)

// Section DTOs

type CreateSectionDTO struct {
	Title        string  `json:"title" validate:"required,min=2,max=255"`
	Description  *string `json:"description"`
	DisplayOrder int     `json:"display_order"`
}

type UpdateSectionDTO struct {
	Title        *string `json:"title" validate:"omitempty,min=2,max=255"`
	Description  *string `json:"description"`
	DisplayOrder *int    `json:"display_order"`
}

type SectionResponseDTO struct {
	ID           uuid.UUID           `json:"id"`
	CourseID     uuid.UUID           `json:"course_id"`
	Title        string              `json:"title"`
	Description  *string             `json:"description,omitempty"`
	DisplayOrder int                 `json:"display_order"`
	Lessons      []LessonResponseDTO `json:"lessons,omitempty"`
	CreatedAt    time.Time           `json:"created_at"`
	UpdatedAt    time.Time           `json:"updated_at"`
}

type ReorderItemDTO struct {
	ID           uuid.UUID `json:"id" validate:"required"`
	DisplayOrder int       `json:"display_order" validate:"required,min=0"`
}

type ReorderDTO struct {
	Items []ReorderItemDTO `json:"items" validate:"required,min=1"`
}
