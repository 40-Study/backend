package dto

import (
	"time"

	"github.com/google/uuid"
)

// Tag DTOs

type CreateTagDTO struct {
	Name string `json:"name" validate:"required,min=1,max=50"`
}

type UpdateTagDTO struct {
	Name *string `json:"name" validate:"omitempty,min=1,max=50"`
}

type TagResponseDTO struct {
	ID        uuid.UUID `json:"id"`
	Name      string    `json:"name"`
	Slug      string    `json:"slug"`
	CreatedAt time.Time `json:"created_at"`
}

type TagListResponseDTO struct {
	Tags     []TagResponseDTO `json:"tags"`
	Total    int64            `json:"total"`
	Page     int              `json:"page"`
	PageSize int              `json:"page_size"`
}
