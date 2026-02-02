package dto

import (
	"time"

	"github.com/google/uuid"
)

type TeacherResponseDTO struct {
	ID          uuid.UUID  `json:"id"`
	Email       string     `json:"email"`
	UserName    string     `json:"user_name"`
	FullName    *string    `json:"full_name,omitempty"`
	AvatarURL   *string    `json:"avatar_url,omitempty"`
	Phone       *string    `json:"phone,omitempty"`
	Bio         *string    `json:"bio,omitempty"`
	DateOfBirth *time.Time `json:"date_of_birth,omitempty"`
	IsVerified  bool       `json:"is_verified"`
	IsActive    bool       `json:"is_active"`
	CreatedAt   string     `json:"created_at"`
	UpdatedAt   string     `json:"updated_at"`
}

type TeacherListResponseDTO struct {
	Teachers []TeacherResponseDTO `json:"teachers"`
	Total    int64                `json:"total"`
	Page     int                  `json:"page"`
	PageSize int                  `json:"page_size"`
}
