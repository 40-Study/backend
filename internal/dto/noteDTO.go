package dto

import (
	"time"

	"github.com/google/uuid"
)

// Note DTOs — ghi chú theo mốc thời gian trong bài học (Phase 1 §3).

// CreateNoteDTO là body của POST /lessons/:lessonId/notes.
type CreateNoteDTO struct {
	TimestampSecs int    `json:"timestamp_seconds" validate:"min=0"`
	Content       string `json:"content" validate:"required,max=2000"`
}

// UpdateNoteDTO là body của PUT /notes/:id — cả hai trường đều optional (sửa một phần).
type UpdateNoteDTO struct {
	TimestampSecs *int    `json:"timestamp_seconds" validate:"omitempty,min=0"`
	Content       *string `json:"content" validate:"omitempty,max=2000"`
}

// NoteResponseDTO — contract §3: {id, lesson_id, lesson_title, section_id, section_title,
// course_id, timestamp_seconds, content, created_at, updated_at}. lesson_title/section_title
// được đính kèm để web hiển thị "Ghi chú trong bài X, chương Y" mà không phải gọi thêm API.
type NoteResponseDTO struct {
	ID            uuid.UUID `json:"id"`
	LessonID      uuid.UUID `json:"lesson_id"`
	LessonTitle   string    `json:"lesson_title"`
	SectionID     uuid.UUID `json:"section_id"`
	SectionTitle  string    `json:"section_title"`
	CourseID      uuid.UUID `json:"course_id"`
	TimestampSecs int       `json:"timestamp_seconds"`
	Content       string    `json:"content"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}
