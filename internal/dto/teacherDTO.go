package dto

import (
	"time"

	"github.com/google/uuid"
)

type CreateTeacherDTO struct {
	Email       string `json:"email" binding:"required,email"`
	Password    string `json:"password" binding:"required,min=6,max=100"`
	UserName    string `json:"user_name" binding:"required,min=2,max=100"`
	FullName    string `json:"full_name" binding:"max=255"`
	Phone       string `json:"phone" binding:"max=20"`
	Bio         string `json:"bio"`
	DateOfBirth string `json:"date_of_birth"`
}

type UpdateTeacherDTO struct {
	UserName    *string `json:"user_name" binding:"omitempty,min=2,max=100"`
	FullName    *string `json:"full_name" binding:"omitempty,max=255"`
	Phone       *string `json:"phone" binding:"omitempty,max=20"`
	Bio         *string `json:"bio"`
	DateOfBirth *string `json:"date_of_birth"`
}

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
