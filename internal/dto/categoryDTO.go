package dto

import (
	"time"

	"github.com/google/uuid"
)

// Category DTOs

type CreateCategoryDTO struct {
	ParentID     *uuid.UUID `json:"parent_id"`
	Name         string     `json:"name" validate:"required,min=2,max=100"`
	Description  *string    `json:"description"`
	IconURL      *string    `json:"icon_url"`
	DisplayOrder *int       `json:"display_order"`
}

type UpdateCategoryDTO struct {
	ParentID     *uuid.UUID `json:"parent_id"`
	Name         *string    `json:"name" validate:"omitempty,min=2,max=100"`
	Description  *string     `json:"description"`
	IconURL      *string     `json:"icon_url"`
	DisplayOrder *int        `json:"display_order"`
	IsActive     *bool       `json:"is_active"`
}

type CategoryResponseDTO struct {
	ID           uuid.UUID             `json:"id"`
	ParentID     *uuid.UUID            `json:"parent_id,omitempty"`
	Name         string                `json:"name"`
	Slug         string                `json:"slug"`
	Description  *string                `json:"description,omitempty"`
	IconURL      *string               `json:"icon_url,omitempty"`
	DisplayOrder int                   `json:"display_order"`
	IsActive     bool                  `json:"is_active"`
	Children     []CategoryResponseDTO `json:"children,omitempty"`
	CreatedAt    time.Time             `json:"created_at"`
	UpdatedAt    time.Time             `json:"updated_at"`
}

type CategoryListResponseDTO struct {
	Categories []CategoryResponseDTO `json:"categories"`
	Total      int64                 `json:"total"`
}
