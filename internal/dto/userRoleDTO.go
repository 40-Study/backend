package dto

import "github.com/google/uuid"

type AssignRolesToUserDTO struct {
	RoleIDs []uuid.UUID `json:"role_ids" binding:"required,min=1,dive,required"`
}

type RemoveRolesFromUserDTO struct {
	RoleIDs []uuid.UUID `json:"role_ids" binding:"required,min=1,dive,required"`
}

type UserRoleResponseDTO struct {
	ID         uuid.UUID       `json:"id"`
	UserID     uuid.UUID       `json:"user_id"`
	RoleID     uuid.UUID       `json:"role_id"`
	Role       RoleResponseDTO `json:"role"`
	AssignedAt string          `json:"assigned_at"`
}

type UserWithRolesResponseDTO struct {
	ID        uuid.UUID               `json:"id"`
	Username  string                  `json:"username"`
	Email     string                  `json:"email"`
	Roles     []RoleDetailResponseDTO `json:"roles"`
	CreatedAt string                  `json:"created_at"`
}

type UserRoleListResponseDTO struct {
	Users    []UserWithRolesResponseDTO `json:"users"`
	Total    int64                      `json:"total"`
	Page     int                        `json:"page"`
	PageSize int                        `json:"page_size"`
}

type UserRoleQueryDTO struct {
	RoleID   *uuid.UUID `query:"role_id"`
	Search   string     `query:"search"`
	Page     int        `query:"page" default:"1"`
	PageSize int        `query:"page_size" default:"20"`
}
