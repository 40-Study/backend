package dto

import "github.com/google/uuid"

// ============ Request DTOs ============

// AssignSystemRolesToUserDTO - Request to assign system roles to user (supports single or multiple)
type AssignSystemRolesToUserDTO struct {
	SystemRoleIDs []uuid.UUID `json:"system_role_ids" validate:"required,min=1,dive,required"`
	Notes         *string     `json:"notes,omitempty" validate:"omitempty,max=500"`
}

// UpdateUserSystemRoleStatusDTO - Request to update user system role status
type UpdateUserSystemRoleStatusDTO struct {
	Status string  `json:"status" validate:"required,oneof=active suspended revoked"`
	Notes  *string `json:"notes,omitempty" validate:"omitempty,max=500"`
}

// ============ Response DTOs ============

// UserSystemRoleResponseDTO - Response for single user system role
type UserSystemRoleResponseDTO struct {
	ID           uuid.UUID              `json:"id"`
	UserID       uuid.UUID              `json:"user_id"`
	SystemRoleID uuid.UUID              `json:"system_role_id"`
	SystemRole   *SystemRoleResponseDTO `json:"system_role,omitempty"`
	GrantedAt    string                 `json:"granted_at"`
	GrantedBy    *uuid.UUID             `json:"granted_by,omitempty"`
	Notes        *string                `json:"notes,omitempty"`
	Status       string                 `json:"status"`
	RevokedBy    *uuid.UUID             `json:"revoked_by,omitempty"`
	RevokedAt    *string                `json:"revoked_at,omitempty"`
	CreatedAt    string                 `json:"created_at"`
	UpdatedAt    string                 `json:"updated_at"`
}

// UserSystemRoleListResponseDTO - Response for list of user system roles
type UserSystemRoleListResponseDTO struct {
	UserSystemRoles []UserSystemRoleResponseDTO `json:"user_system_roles"`
	Total           int64                       `json:"total"`
	Page            int                         `json:"page"`
	PageSize        int                         `json:"page_size"`
}

// UserWithSystemRolesResponseDTO - Response for user with their system roles
type UserWithSystemRolesResponseDTO struct {
	UserID      uuid.UUID                   `json:"user_id"`
	Username    string                      `json:"username"`
	Email       string                      `json:"email"`
	SystemRoles []UserSystemRoleResponseDTO `json:"system_roles"`
}

// UsersWithSystemRoleResponseDTO - Response for users having a specific system role
type UsersWithSystemRoleResponseDTO struct {
	SystemRoleID   uuid.UUID                        `json:"system_role_id"`
	SystemRoleName string                           `json:"system_role_name"`
	Users          []UserWithSystemRolesResponseDTO `json:"users"`
	Total          int64                            `json:"total"`
	Page           int                              `json:"page"`
	PageSize       int                              `json:"page_size"`
}

// ============ Query DTOs ============

// UserSystemRoleQueryDTO - Query params for filtering user system roles
type UserSystemRoleQueryDTO struct {
	UserID       *uuid.UUID `query:"user_id"`
	SystemRoleID *uuid.UUID `query:"system_role_id"`
	Status       string     `query:"status"`
	Page         int        `query:"page" default:"1"`
	PageSize     int        `query:"page_size" default:"20"`
}
