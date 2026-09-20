package dto

import (
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

// Enrollment DTOs

// PendingAssignmentDTO: bai tap chua hoan thanh (chua co submission accepted).
type PendingAssignmentDTO struct {
	ID         uuid.UUID  `json:"id"`
	Title      string     `json:"title"`
	DueDate    *string    `json:"due_date,omitempty"`
	CourseName string     `json:"course_name"`
	LessonID   *uuid.UUID `json:"lesson_id,omitempty"`
}

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
	WatchedSeconds     int                    `json:"watched_seconds"`
	CompletedLessons   int                    `json:"completed_lessons"`
	TotalLessons       int                    `json:"total_lessons"`
	PendingAssignments []PendingAssignmentDTO `json:"pending_assignments"`
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

// PlayedRangeDTO / PlayedRangesDTO nam trong file rieng (played_range_dto.go) vi chung can
// custom JSON marshaling: contract §1 bieu dien moi khoang bang MOT CAP SO `[start, end]`,
// khong phai object.

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

	// ——— Phase 1 §1 (chong tua) ———
	// Ca ba field deu OPTIONAL: client cu (chi gui status + video_watched_seconds) van chay
	// nguyen ven, chi la khong duoc huong phan chong tua.
	//
	// PositionSeconds: vi tri hien tai cua dau phat (giay). Luu vao last_position_seconds de
	// lan sau mo lai bai resume dung cho.
	PositionSeconds *int `json:"position_seconds" validate:"omitempty,min=0,max=86400"`
	// DurationSeconds: tong thoi luong client doc duoc. La MAU SO cua watched_pct.
	DurationSeconds *int `json:"duration_seconds" validate:"omitempty,min=0,max=86400"`
	// PlayedRanges: cac khoang VUA PHAT ke tu lan gui truoc, dang cap so `[[start, end], ...]`.
	// KHONG validate bang struct tag — contract yeu cau khoang khong hop le bi BO QUA chu khong
	// lam hong ca request, nen viec kiem tra nam o service (MergePlayedRanges).
	PlayedRanges PlayedRangesDTO `json:"played_ranges"`
}

// BeaconProgressDTO la body ma `navigator.sendBeacon("/api/progress", ...)` gui
// tu trinh phat video (web: hooks/use-video-progress.ts -> lib/played-ranges.ts
// buildHeartbeatPayload).
//
// HAI THE HE CLIENT cung gui vao route nay, nen DTO phai doc duoc CA HAI:
//
//  1. Ban Phase 1 (snake_case, contract §1): {lesson_id, position_seconds, duration_seconds,
//     played_ranges}. Day la shape `buildHeartbeatPayload` sinh ra va gui y nguyen qua
//     JSON.stringify — ca cho beacon lan cho PUT.
//  2. Ban truoc Phase 1 (camelCase): {lessonId, status, videoWatchedSeconds}. Giu lai de mot
//     tab dang mo tu ban cu khong lam mat tien do khi dong tab.
//
// LessonID nam TRONG body (khong phai path param) o CA HAI ban vi sendBeacon chi nhan mot URL
// co dinh nen khong the chen :lessonId vao duong dan.
//
// Rang buoc min/max giong UpdateLessonProgressDTO: cot nay chi TANG nen mot lan
// nhan gia tri rac se khong bao gio bi ghi de boi request nho hon.
type BeaconProgressDTO struct {
	LessonID string `json:"lesson_id" validate:"omitempty,uuid"`
	Status   *string `json:"status" validate:"omitempty,oneof=not_started in_progress completed"`
	VideoWatchedSecs *int `json:"video_watched_seconds" validate:"omitempty,min=0,max=86400"`

	// ——— Phase 1 §1 ———
	PositionSeconds *int             `json:"position_seconds" validate:"omitempty,min=0,max=86400"`
	DurationSeconds *int             `json:"duration_seconds" validate:"omitempty,min=0,max=86400"`
	PlayedRanges    PlayedRangesDTO  `json:"played_ranges"`

	// ——— Tuong thich nguoc (camelCase, ban truoc Phase 1) ———
	LessonIDCamel    string  `json:"lessonId" validate:"omitempty,uuid"`
	VideoWatchedSecsCamel *int `json:"videoWatchedSeconds" validate:"omitempty,min=0,max=86400"`
}

// ResolvedLessonID tra ve id bai hoc du client gui bang casing nao (snake_case truoc, camelCase
// sau). Rong nghia la client bo sot — handler phai tra 400, KHONG duoc coi la "khong doi gi".
func (b BeaconProgressDTO) ResolvedLessonID() string {
	if b.LessonID != "" {
		return b.LessonID
	}
	return b.LessonIDCamel
}

// ToUpdateDTO chuan hoa beacon ve dung DTO cua duong ghi, de ca hai route di qua DUNG MOT
// ham service (khong co ban sao thu hai cua logic ghi tien do).
func (b BeaconProgressDTO) ToUpdateDTO() UpdateLessonProgressDTO {
	out := UpdateLessonProgressDTO{
		Status:           b.Status,
		VideoWatchedSecs: b.VideoWatchedSecs,
		PositionSeconds:  b.PositionSeconds,
		DurationSeconds:  b.DurationSeconds,
		PlayedRanges:     b.PlayedRanges,
	}
	if out.VideoWatchedSecs == nil {
		out.VideoWatchedSecs = b.VideoWatchedSecsCamel
	}
	return out
}

// LessonProgressStateDTO la `data` cua PUT /lessons/:lessonId/progress va POST /progress
// (contract §1). Web doc thang shape nay trong services/enrollment.service.ts
// (LessonProgressResponse) nen doi ten/them bot truong o day la pha vo web.
type LessonProgressStateDTO struct {
	LessonID           uuid.UUID `json:"lesson_id"`
	Status             string    `json:"status"`
	WatchedSeconds     int       `json:"watched_seconds"`
	WatchedPct         float64   `json:"watched_pct"`
	LastPositionSeconds int      `json:"last_position_seconds"`
	// completed_at: KHONG omitempty — contract ghi ro `"completed_at": "...|null"`, va web khai
	// `completed_at: string | null`. Thieu truong (omitempty) khac voi null o phia client.
	CompletedAt        *string `json:"completed_at"`
	NextLessonUnlocked bool    `json:"next_lesson_unlocked"`
	// CourseCompleted: true khi day la bai cuoi va khoa hoc dat 100% sau khi complete bai nay.
	// FE dung de hien man hinh chuc mung hoan thanh khoa hoc.
	CourseCompleted bool `json:"course_completed"`
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

	// Phase 1 §1: hai truong nay duoc bo sung de man chi tiet ghi danh
	// (GET /enrollments/:id) hien duoc tien do xem video ma khong phai goi them API.
	WatchedPct          decimal.Decimal `json:"watched_pct"`
	LastPositionSeconds int             `json:"last_position_seconds"`
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
