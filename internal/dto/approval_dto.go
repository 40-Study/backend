package dto

import "github.com/google/uuid"

// Phase 3 duyệt khoá học + duyệt giáo viên (2026-09-28) — contract:
// plans/260927-2055-role-based-ux-qa/admin-features/phase-03-course-teacher-approval.md

// ReviewReasonRequestDTO — body của POST .../reject (khoá học lẫn hồ sơ giáo viên).
type ReviewReasonRequestDTO struct {
	Reason string `json:"reason"`
}

// AdminCourseReviewItemDTO — 1 dòng hàng chờ duyệt khoá: CourseResponseDTO + người gửi. Tách
// DTO riêng (không thêm vào CourseResponseDTO) để email giảng viên KHÔNG lộ ra route công khai
// GET /courses.
type AdminCourseReviewItemDTO struct {
	CourseResponseDTO
	InstructorName  string `json:"instructor_name"`
	InstructorEmail string `json:"instructor_email"`
}

// AdminCourseReviewListDTO giữ đúng shape CourseListResponseDTO (courses/total/page/page_size).
type AdminCourseReviewListDTO struct {
	Courses  []AdminCourseReviewItemDTO `json:"courses"`
	Total    int64                      `json:"total"`
	Page     int                        `json:"page"`
	PageSize int                        `json:"page_size"`
}

type CourseReviewResultDTO struct {
	ID          uuid.UUID `json:"id"`
	Status      string    `json:"status"`
	SubmittedAt *string   `json:"submitted_at,omitempty"`
}

type TeacherApplicationItemDTO struct {
	UserID            uuid.UUID `json:"user_id"`
	ProfileID         uuid.UUID `json:"profile_id"`
	Email             string    `json:"email"`
	FullName          *string   `json:"full_name,omitempty"`
	Specialization    *string   `json:"specialization,omitempty"`
	Education         *string   `json:"education,omitempty"`
	ExperienceYears   *int      `json:"experience_years,omitempty"`
	CertificateInfo   *string   `json:"certificate_info,omitempty"`
	Department        *string   `json:"department,omitempty"`
	ApprovalStatus    string    `json:"approval_status"`
	RejectionReason   *string   `json:"rejection_reason,omitempty"`
	ResubmissionCount int       `json:"resubmission_count"`
	CreatedAt         string    `json:"created_at"`
	UpdatedAt         string    `json:"updated_at"`
	ReviewedAt        *string   `json:"reviewed_at,omitempty"`
}

// TeacherApplicationListDTO — mẫu phân trang mới của 4 phase (items/total_count/page/limit/total_pages).
type TeacherApplicationListDTO struct {
	Items      []TeacherApplicationItemDTO `json:"items"`
	TotalCount int64                       `json:"total_count"`
	Page       int                         `json:"page"`
	Limit      int                         `json:"limit"`
	TotalPages int                         `json:"total_pages"`
}

type TeacherApplicationResultDTO struct {
	UserID         uuid.UUID `json:"user_id"`
	ApprovalStatus string    `json:"approval_status"`
}

// MyTeacherApplicationDTO — GET /teacher-profiles/me: hồ sơ + thông tin nộp lại để web quyết
// định hiện nút "Nộp lại" hay thông báo liên hệ hỗ trợ (quyết định #5).
type MyTeacherApplicationDTO struct {
	TeacherProfileResponseDTO
	MaxResubmissions int  `json:"max_resubmissions"`
	CanResubmit      bool `json:"can_resubmit"`
}
