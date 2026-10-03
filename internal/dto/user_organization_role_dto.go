package dto

import "github.com/google/uuid"

// ============ Request DTOs ============

// AssignOrgRolesToUserDTO - Request to assign organization roles to user (supports single or multiple)
type AssignOrgRolesToUserDTO struct {
	RoleIDs        []uuid.UUID `json:"role_ids" validate:"required,min=1,dive,required"`
	OrganizationID uuid.UUID   `json:"organization_id" validate:"required"`
	Notes          *string     `json:"notes,omitempty" validate:"omitempty,max=500"`
}

// UpdateUserOrgRoleStatusDTO - Request to update user organization role status
type UpdateUserOrgRoleStatusDTO struct {
	Status string  `json:"status" validate:"required,oneof=active suspended revoked"`
	Notes  *string `json:"notes,omitempty" validate:"omitempty,max=500"`
}

// ============ Response DTOs ============

// UserOrgRoleResponseDTO - Response for single user organization role
type UserOrgRoleResponseDTO struct {
	ID             uuid.UUID           `json:"id"`
	UserID         uuid.UUID           `json:"user_id"`
	RoleID         uuid.UUID           `json:"role_id"`
	OrganizationID uuid.UUID           `json:"organization_id"`
	Role           *OrgRoleResponseDTO `json:"role,omitempty"`
	Organization   *OrgInfoResponseDTO `json:"organization,omitempty"`
	// User (B-16): người giữ vai trò, để UI hiển thị tên/email thay vì user_id trần. Chỉ có khi repo đã Preload("User").
	User      *OrgMemberUserDTO `json:"user,omitempty"`
	GrantedAt string            `json:"granted_at"`
	GrantedBy *uuid.UUID        `json:"granted_by,omitempty"`
	Notes     *string           `json:"notes,omitempty"`
	Status    string            `json:"status"`
	RevokedBy *uuid.UUID        `json:"revoked_by,omitempty"`
	RevokedAt *string           `json:"revoked_at,omitempty"`
	CreatedAt string            `json:"created_at"`
	UpdatedAt string            `json:"updated_at"`
}

// OrgRoleResponseDTO - Embedded organization role info
type OrgRoleResponseDTO struct {
	ID          uuid.UUID `json:"id"`
	Name        string    `json:"name"`
	Description *string   `json:"description,omitempty"`
	Status      string    `json:"status"`
}

// OrgMemberUserDTO - Embedded user info của một thành viên tổ chức (B-16)
type OrgMemberUserDTO struct {
	ID        uuid.UUID `json:"id"`
	UserName  string    `json:"user_name"`
	FullName  *string   `json:"full_name,omitempty"`
	Email     string    `json:"email"`
	AvatarURL *string   `json:"avatar_url,omitempty"`
}

// OrgInfoResponseDTO - Embedded organization info
type OrgInfoResponseDTO struct {
	ID   uuid.UUID `json:"id"`
	Name string    `json:"name"`
}

// UserOrgRoleListResponseDTO - Response for list of user organization roles
type UserOrgRoleListResponseDTO struct {
	UserOrgRoles []UserOrgRoleResponseDTO `json:"user_organization_roles"`
	Total        int64                    `json:"total"`
	Page         int                      `json:"page"`
	PageSize     int                      `json:"page_size"`
}

// UserWithOrgRolesResponseDTO - Response for user with their organization roles
type UserWithOrgRolesResponseDTO struct {
	UserID   uuid.UUID                `json:"user_id"`
	Username string                   `json:"username"`
	Email    string                   `json:"email"`
	OrgRoles []UserOrgRoleResponseDTO `json:"organization_roles"`
}

// UsersWithOrgRoleResponseDTO - Response for users having a specific organization role
type UsersWithOrgRoleResponseDTO struct {
	RoleID         uuid.UUID                     `json:"role_id"`
	RoleName       string                        `json:"role_name"`
	OrganizationID uuid.UUID                     `json:"organization_id"`
	Users          []UserWithOrgRolesResponseDTO `json:"users"`
	Total          int64                         `json:"total"`
	Page           int                           `json:"page"`
	PageSize       int                           `json:"page_size"`
}

// ============ Query DTOs ============

// UserOrgRoleQueryDTO - Query params for filtering user organization roles
type UserOrgRoleQueryDTO struct {
	UserID         *uuid.UUID `query:"user_id"`
	RoleID         *uuid.UUID `query:"role_id"`
	OrganizationID *uuid.UUID `query:"organization_id"`
	Status         string     `query:"status"`
	Page           int        `query:"page" default:"1"`
	PageSize       int        `query:"page_size" default:"20"`
}
