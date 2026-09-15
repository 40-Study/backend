package dto

import (
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

// Enrollment DTOs

type EnrollmentResponseDTO struct {
	ID              uuid.UUID       `json:"id"`
	UserID          uuid.UUID       `json:"user_id"`
	CourseID        uuid.UUID       `json:"course_id"`
	EnrolledAt      string          `json:"enrolled_at"`
	ProgressPercent decimal.Decimal `json:"progress_percentage"`
	CompletedAt     *string         `json:"completed_at,omitempty"`
	LastAccessedAt  *string         `json:"last_accessed_at,omitempty"`
	CourseTitle     string          `json:"course_title,omitempty"`
	CourseSlug      string          `json:"course_slug,omitempty"`
	CourseThumbnail *string         `json:"course_thumbnail,omitempty"`
	CourseCategory  string          `json:"course_category,omitempty"`

	// WatchedSeconds: tong so giay video da xem cua ghi danh nay, cong don tu
	// lesson_progress.video_watched_seconds (du lieu that do trinh phat video ghi len qua
	// PUT /lessons/:lessonId/progress). Truoc day web hardcode "1h 45m" o trang
	// "Khoa hoc cua toi" vi khong co truong nay.
	WatchedSeconds int `json:"watched_seconds"`
}

type EnrollmentDetailDTO struct {
	EnrollmentResponseDTO
	LessonProgress []LessonProgressResponseDTO `json:"lesson_progress,omitempty"`
}

type EnrollmentListResponseDTO struct {
	Enrollments []EnrollmentResponseDTO `json:"enrollments"`
	Total       int64                   `json:"total"`
	Page        int                     `json:"page"`
	PageSize    int                     `json:"page_size"`
}

// LessonProgress DTOs

type UpdateLessonProgressDTO struct {
	Status           *string          `json:"status" validate:"omitempty,oneof=not_started in_progress completed"`
	ProgressPercent  *decimal.Decimal `json:"progress_percentage"`
	// min=0: truoc day khong co rang buoc nao — client gui -999999 duoc luu thang vao DB va
	// lam chi so "thoi gian hoc" tren web am. Da tai hien duoc tren server that.
	// max=86400 (24 gio): min=0 chan so am nhung khong chan gia tri rac. Cot nay chi TANG (xem
	// UpdateLessonProgress), nen mot lan gui 2000000000 (~63 nam) se khong bao gio bi ghi de boi
	// cac request nho hon nua => hong vinh vien "thoi gian hoc" tren web va
	// total_study_time_minutes o trang ho so, chi sua duoc bang UPDATE tay trong DB.
	VideoWatchedSecs *int `json:"video_watched_seconds" validate:"omitempty,min=0,max=86400"`
}

// BeaconProgressDTO la body ma `navigator.sendBeacon("/api/progress", ...)` gui
// tu trinh phat video (web: app/courses/[slug]/learn/player-client.tsx).
//
// Hai khac biet BAT BUOC so voi UpdateLessonProgressDTO, dung thieu mot cai nao:
//  1. Ten field la camelCase (lessonId / videoWatchedSeconds) vi client
//     JSON.stringify thang object cua no, khong di qua lop service snake_case.
//  2. LessonID nam TRONG body, khong phai path param — sendBeacon chi nhan
//     mot URL co dinh nen khong the chen :lessonId vao duong dan.
//
// Rang buoc min/max giong UpdateLessonProgressDTO: cot nay chi TANG nen mot lan
// nhan gia tri rac se khong bao gio bi ghi de boi request nho hon.
type BeaconProgressDTO struct {
	LessonID         string  `json:"lessonId" validate:"required,uuid"`
	Status           *string `json:"status" validate:"omitempty,oneof=not_started in_progress completed"`
	VideoWatchedSecs *int    `json:"videoWatchedSeconds" validate:"omitempty,min=0,max=86400"`
}

type LessonProgressResponseDTO struct {
	ID               uuid.UUID       `json:"id"`
	UserID           uuid.UUID       `json:"user_id"`
	LessonID         uuid.UUID       `json:"lesson_id"`
	EnrollmentID     uuid.UUID       `json:"enrollment_id"`
	Status           string          `json:"status"`
	ProgressPercent  decimal.Decimal `json:"progress_percentage"`
	VideoWatchedSecs int             `json:"video_watched_seconds"`
	CompletedAt      *string         `json:"completed_at,omitempty"`
	LastAccessedAt   string          `json:"last_accessed_at"`
}

// Course Enrollments (Instructor view)

type CourseEnrollmentItemDTO struct {
	ID              uuid.UUID       `json:"id"`
	UserID          uuid.UUID       `json:"user_id"`
	UserEmail       string          `json:"user_email"`
	UserName        string          `json:"user_name"`
	EnrolledAt      string          `json:"enrolled_at"`
	ProgressPercent decimal.Decimal `json:"progress_percentage"`
	CompletedAt     *string         `json:"completed_at,omitempty"`
}

type CourseEnrollmentListDTO struct {
	Enrollments []CourseEnrollmentItemDTO `json:"enrollments"`
	Total       int64                     `json:"total"`
	Page        int                       `json:"page"`
	PageSize    int                       `json:"page_size"`
}

// Debug DTO (includes soft-deleted)

type DebugEnrollmentDTO struct {
	ID         uuid.UUID `json:"id"`
	UserID     uuid.UUID `json:"user_id"`
	UserEmail  string    `json:"user_email"`
	CourseID   uuid.UUID `json:"course_id"`
	EnrolledAt string    `json:"enrolled_at"`
	IsDeleted  bool      `json:"is_deleted"`
	DeletedAt  *string   `json:"deleted_at,omitempty"`
}
