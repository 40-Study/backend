package dto

import (
	"time"

	"github.com/google/uuid"
)

// ===== Phase 1 quản lý người dùng (admin) — contract:
// plans/260927-2055-role-based-ux-qa/admin-features/phase-01-user-management.md =====

// AdminUserListItemDTO — 1 phần tử trong GET /api/users.
type AdminUserListItemDTO struct {
	ID           uuid.UUID  `json:"id"`
	Email        string     `json:"email"`
	UserName     string     `json:"user_name"`
	FullName     *string    `json:"full_name"`
	AvatarURL    *string    `json:"avatar_url"`
	IsActive     bool       `json:"is_active"`
	IsVerified   bool       `json:"is_verified"`
	LockedReason *string    `json:"locked_reason"`
	LockedAt     *time.Time `json:"locked_at"`
	LastLoginAt  *time.Time `json:"last_login_at"`
	CreatedAt    time.Time  `json:"created_at"`
	SystemRoles  []string   `json:"system_roles"`
}

// AdminUserListResponseDTO — envelope "data" của GET /api/users.
type AdminUserListResponseDTO struct {
	Items      []AdminUserListItemDTO `json:"items"`
	TotalCount int64                  `json:"total_count"`
	Page       int                    `json:"page"`
	Limit      int                    `json:"limit"`
	TotalPages int                    `json:"total_pages"`
}

// AdminUserSystemRoleDTO — vai trò hệ thống dạng đầy đủ trong GET /api/users/:id.
type AdminUserSystemRoleDTO struct {
	ID        uuid.UUID `json:"id"`
	Name      string    `json:"name"`
	GrantedAt time.Time `json:"granted_at"`
}

// AdminUserDetailDTO — envelope "data" của GET /api/users/:id.
type AdminUserDetailDTO struct {
	ID           uuid.UUID                `json:"id"`
	Email        string                   `json:"email"`
	UserName     string                   `json:"user_name"`
	FullName     *string                  `json:"full_name"`
	AvatarURL    *string                  `json:"avatar_url"`
	Phone        *string                  `json:"phone"`
	DateOfBirth  *time.Time               `json:"date_of_birth"`
	IsActive     bool                     `json:"is_active"`
	IsVerified   bool                     `json:"is_verified"`
	LockedReason *string                  `json:"locked_reason"`
	LockedAt     *time.Time               `json:"locked_at"`
	LastLoginAt  *time.Time               `json:"last_login_at"`
	CreatedAt    time.Time                `json:"created_at"`
	SystemRoles  []AdminUserSystemRoleDTO `json:"system_roles"`
}

// UpdateUserStatusRequestDTO — body của PUT /api/users/:id/status.
// Reason bắt buộc khi IsActive=false — kiểm bằng tay trong handler (không dùng tag
// required_if của validator ở đây để giữ thông báo lỗi rõ ràng, giống style handler hiện có).
type UpdateUserStatusRequestDTO struct {
	IsActive bool    `json:"is_active"`
	Reason   *string `json:"reason,omitempty" validate:"omitempty,max=500"`
}
