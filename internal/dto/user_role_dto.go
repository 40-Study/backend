package dto

import "github.com/google/uuid"

// UserAllRolesResponseDTO - Tổng hợp toàn bộ roles của 1 user (cả system lẫn org)
type UserAllRolesResponseDTO struct {
	UserID       uuid.UUID                    `json:"user_id"`
	SystemRoles  []UserSystemRoleResponseDTO  `json:"system_roles"`
	OrgRoles     []UserOrgRoleResponseDTO     `json:"org_roles"`
}
