package dto

import "github.com/google/uuid"

type CreateRoleDTO struct {
	Name        string `json:"name" binding:"required,min=2,max=100"`
	Description string `json:"description" binding:"max=500"`
}

type UpdateRoleDTO struct {
	Name        *string `json:"name" binding:"omitempty,min=2,max=100"`
	Description *string `json:"description" binding:"omitempty,max=500"`
}

type AddPermissionsToRoleDTO struct {
	PermissionIDs []uuid.UUID `json:"permission_ids" binding:"required,min=1,dive,required"`
}

type RemovePermissionsFromRoleDTO struct {
	PermissionIDs []uuid.UUID `json:"permission_ids" binding:"required,min=1,dive,required"`
}

type RoleResponseDTO struct {
	ID          uuid.UUID `json:"id"`
	Name        string    `json:"name"`
	Description *string   `json:"description,omitempty"`
	CreatedAt   string    `json:"created_at"`
	UpdatedAt   string    `json:"updated_at"`
}

type RoleDetailResponseDTO struct {
	ID          uuid.UUID               `json:"id"`
	Name        string                  `json:"name"`
	Description *string                 `json:"description,omitempty"`
	Permissions []PermissionResponseDTO `json:"permissions"`
	CreatedAt   string                  `json:"created_at"`
	UpdatedAt   string                  `json:"updated_at"`
}

type RoleListResponseDTO struct {
	Roles    []RoleResponseDTO `json:"roles"`
	Total    int64             `json:"total"`
	Page     int               `json:"page"`
	PageSize int               `json:"page_size"`
}

type RoleQueryDTO struct {
	Search   string `query:"search"`
	Page     int    `query:"page" default:"1"`
	PageSize int    `query:"page_size" default:"20"`
}
