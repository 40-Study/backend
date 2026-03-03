package dto

import "github.com/google/uuid"

type CreateTeacherProfileDTO struct {
	UserID          uuid.UUID `json:"user_id" binding:"required"`
	Specialization  *string   `json:"specialization" binding:"omitempty,max=255"`
	Education       *string   `json:"education" binding:"omitempty,max=255"`
	ExperienceYears *int      `json:"experience_years" binding:"omitempty,min=0"`
	CertificateInfo *string   `json:"certificate_info"`
	Department      *string   `json:"department" binding:"omitempty,max=255"`
}

type UpdateTeacherProfileDTO struct {
	Specialization  *string `json:"specialization" binding:"omitempty,max=255"`
	Education       *string `json:"education" binding:"omitempty,max=255"`
	ExperienceYears *int    `json:"experience_years" binding:"omitempty,min=0"`
	CertificateInfo *string `json:"certificate_info"`
	Department      *string `json:"department" binding:"omitempty,max=255"`
}

type TeacherProfileResponseDTO struct {
	ID              uuid.UUID `json:"id"`
	UserID          uuid.UUID `json:"user_id"`
	Specialization  *string   `json:"specialization,omitempty"`
	Education       *string   `json:"education,omitempty"`
	ExperienceYears *int      `json:"experience_years,omitempty"`
	CertificateInfo *string   `json:"certificate_info,omitempty"`
	Department      *string   `json:"department,omitempty"`
	CreatedAt       string    `json:"created_at"`
	UpdatedAt       string    `json:"updated_at"`
}

type TeacherProfileListResponseDTO struct {
	TeacherProfiles []TeacherProfileResponseDTO `json:"teacher_profiles"`
	Total           int64                       `json:"total"`
	Page            int                         `json:"page"`
	PageSize        int                         `json:"page_size"`
}
