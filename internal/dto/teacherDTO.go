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

type TeacherStudentDTO struct {
	ID          uuid.UUID  `json:"id"`
	Name        string     `json:"name"`
	Email       string     `json:"email"`
	Avatar      *string    `json:"avatar,omitempty"`
	StudentID   *string    `json:"student_id,omitempty"`
	ParentName  *string    `json:"parent_name,omitempty"`
	ParentPhone *string    `json:"parent_phone,omitempty"`
	ClassID     uuid.UUID  `json:"class_id"`
	ClassName   string     `json:"class_name"`
	CourseID    *uuid.UUID `json:"course_id,omitempty"`
	CourseName  *string    `json:"course_name,omitempty"`
	Status      string     `json:"status"`
	Progress    *float64   `json:"progress,omitempty"`
	LastActive  *time.Time `json:"last_active,omitempty"`
	EnrolledAt  time.Time  `json:"enrolled_at"`
}

type TeacherStudentListResponseDTO struct {
	Students []TeacherStudentDTO `json:"students"`
	Total    int64               `json:"total"`
	Page     int                 `json:"page"`
	PageSize int                 `json:"page_size"`
}
