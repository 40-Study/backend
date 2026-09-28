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

	// Phase 3 duyệt giáo viên
	ApprovalStatus    string  `json:"approval_status"`
	RejectionReason   *string `json:"rejection_reason,omitempty"`
	ReviewedAt        *string `json:"reviewed_at,omitempty"`
	ResubmissionCount int     `json:"resubmission_count"`
}

// PublicTeacherProfileDTO — hồ sơ trả qua route CÔNG KHAI (GET /teacher-profiles, /:id, không
// cần đăng nhập). Review PR #73 (MAJOR #1): KHÔNG chứa trạng thái duyệt/lý do từ chối/số lần nộp
// lại — người lạ không được biết ai đang chờ duyệt hay bị từ chối vì sao. Chủ hồ sơ xem qua
// GET /teacher-profiles/me, admin qua /admin/teacher-applications.
type PublicTeacherProfileDTO struct {
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
	TeacherProfiles []PublicTeacherProfileDTO `json:"teacher_profiles"`
	Total           int64                       `json:"total"`
	Page            int                         `json:"page"`
	PageSize        int                         `json:"page_size"`
}
