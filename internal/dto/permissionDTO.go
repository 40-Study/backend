package dto

import "github.com/google/uuid"

type UpdatePermissionDTO struct {
	Description string `json:"description" binding:"required,min=1,max=500"`
}

type PermissionResponseDTO struct {
	ID          uuid.UUID `json:"id"`
	Name        string    `json:"name"`
	Description *string   `json:"description,omitempty"`
	CreatedAt   string    `json:"created_at"`
	UpdatedAt   string    `json:"updated_at"`
}

type PermissionListResponseDTO struct {
	Permissions []PermissionResponseDTO `json:"permissions"`
	Total       int64                   `json:"total"`
	Page        int                     `json:"page"`
	PageSize    int                     `json:"page_size"`
}
