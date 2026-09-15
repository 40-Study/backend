package dto

import (
	"time"

	"github.com/google/uuid"
)

// Lesson DTOs

type CreateLessonDTO struct {
	Title        string    `json:"title" validate:"required,min=2,max=255"`
	Description  *string   `json:"description"`
	DisplayOrder int       `json:"display_order" validate:"min=0"`
	DurationMins *int      `json:"duration_minutes"`
	IsPreview    *bool     `json:"is_preview"`
	IsMandatory  *bool     `json:"is_mandatory"` // có ý nghĩa là học viên phải hoàn thành bài học này mới được xem các bài học khác trong cùng section hay không
}

type UpdateLessonDTO struct {
	Title        *string `json:"title" validate:"omitempty,min=2,max=255"`
	Description  *string `json:"description"`
	DisplayOrder *int    `json:"display_order" validate:"omitempty,min=0"`
	DurationMins *int    `json:"duration_minutes"`
	IsPreview    *bool   `json:"is_preview"`
	IsMandatory  *bool   `json:"is_mandatory"`
}

type LessonResponseDTO struct {
	ID           uuid.UUID                  `json:"id"`
	SectionID    uuid.UUID                  `json:"section_id"`
	Title        string                     `json:"title"`
	Description  *string                    `json:"description,omitempty"`
	DisplayOrder int                        `json:"display_order"`
	DurationMins int                        `json:"duration_minutes"`
	IsPreview    bool                       `json:"is_preview"`
	IsMandatory  bool                       `json:"is_mandatory"`
	Contents     []LessonContentResponseDTO `json:"contents,omitempty"`
	CreatedAt    time.Time                  `json:"created_at"`
	UpdatedAt    time.Time                  `json:"updated_at"`

	// ——— Phase 1 §2 (khoá học tuần tự) ———
	// KHÔNG dùng omitempty cho Locked/LockReason: contract ghi rõ hai trường này LUÔN xuất
	// hiện trên curriculum (`"locked": false, "lock_reason": null`), web đọc thẳng shape này
	// (web/src/types/lesson.ts). Locked/LockReason chỉ có Ý NGHĨA trên các response tính theo
	// NGƯỜI DÙNG hiện tại (GetAllSections, GetCourseBySlug) — các response khác (tạo/sửa lesson)
	// để mặc định false/nil vì không có "người xem" nào để tính khoá.
	Locked     bool                      `json:"locked"`
	LockReason *string                   `json:"lock_reason"`
	Progress   *LessonProgressSummaryDTO `json:"progress,omitempty"`
}

// LessonProgressSummaryDTO là tiến độ TÓM TẮT gắn kèm mỗi bài trong curriculum (contract §2) —
// khác với dto.LessonProgressStateDTO (contract §1, trả về từ chính request ghi tiến độ).
type LessonProgressSummaryDTO struct {
	Status              string  `json:"status"`
	WatchedPct          float64 `json:"watched_pct"`
	LastPositionSeconds int     `json:"last_position_seconds"`
}
