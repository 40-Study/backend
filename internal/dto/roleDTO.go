package dto

import "github.com/google/uuid"

// M-01 (audit 260909 vòng 2): tag trước đây dùng `binding:"..."` (quy ước của gin) trong khi
// utils.ValidateStruct dùng go-playground/validator với tag mặc định `validate` — `binding`
// hoàn toàn không được đọc, dù handler CÓ gọi ValidateStruct thì cũng như không có tag nào.
// Đổi sang `validate:"..."` để tag thật sự có tác dụng (xem role_handler.go).
type CreateRoleDTO struct {
	Name           string    `json:"name" validate:"required,min=2,max=100"`
	OrganizationID uuid.UUID `json:"organization_id" validate:"required"`
	Description    string    `json:"description" validate:"max=500"`
}

type UpdateRoleDTO struct {
	Name        *string `json:"name" validate:"omitempty,min=2,max=100"`
	Description *string `json:"description" validate:"omitempty,max=500"`
}

type AddPermissionsToRoleDTO struct {
	PermissionIDs []uuid.UUID `json:"permission_ids" validate:"required,min=1,dive,required"`
}

type RemovePermissionsFromRoleDTO struct {
	PermissionIDs []uuid.UUID `json:"permission_ids" validate:"required,min=1,dive,required"`
}

type RoleResponseDTO struct {
	ID             uuid.UUID  `json:"id"`
	Name           string     `json:"name"`
	OrganizationID *uuid.UUID `json:"organization_id,omitempty"`
	Description    *string   `json:"description,omitempty"`
	Status         string    `json:"status"`
	CreatedAt      string    `json:"created_at"`
	UpdatedAt      string    `json:"updated_at"`
}

type RoleDetailResponseDTO struct {
	ID             uuid.UUID               `json:"id"`
	Name           string                  `json:"name"`
	OrganizationID *uuid.UUID              `json:"organization_id,omitempty"`
	Description    *string                 `json:"description,omitempty"`
	Status         string                  `json:"status"`
	Permissions    []PermissionResponseDTO `json:"permissions"`
	CreatedAt      string                  `json:"created_at"`
	UpdatedAt      string                  `json:"updated_at"`
}

type RoleListResponseDTO struct {
	Roles    []RoleResponseDTO `json:"roles"`
	Total    int64             `json:"total"`
	Page     int               `json:"page"`
	PageSize int               `json:"page_size"`
}
