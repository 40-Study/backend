package dto

import (
	"time"

	"github.com/google/uuid"
)

// LessonContent DTOs
// Type: video, livestream, exercise, article, quiz (SSOT: model.LessonContentTypes; contract C1)

type CreateLessonContentDTO struct {
	Type         string     `json:"type" validate:"required,oneof=video livestream exercise article quiz"`
	Title        *string    `json:"title"`
	VideoURL     *string    `json:"video_url"`
	Duration     *int       `json:"duration"`
	ExerciseID   *uuid.UUID `json:"exercise_id"`
	IsMandatory  *bool      `json:"is_mandatory"`
	DisplayOrder *int       `json:"display_order"`
	// SubtitleURL (Phase 1 §4): URL file .vtt đã upload sẵn qua luồng presigned có sẵn.
	SubtitleURL *string `json:"subtitle_url"`
	// ArticleBody (C1): HTML Tiptap của type=article. Bắt buộc, <= 200000 ký tự — service kiểm (có mã lỗi riêng).
	ArticleBody *string `json:"article_body"`
	// QuizID (C1): quiz gắn vào bài của type=quiz. Bắt buộc với type=quiz.
	QuizID *uuid.UUID `json:"quiz_id"`
}

type UpdateLessonContentDTO struct {
	Type         *string    `json:"type" validate:"omitempty,oneof=video livestream exercise article quiz"`
	Title        *string    `json:"title"`
	VideoURL     *string    `json:"video_url"`
	Duration     *int       `json:"duration"`
	ExerciseID   *uuid.UUID `json:"exercise_id"`
	IsMandatory  *bool      `json:"is_mandatory"`
	DisplayOrder *int       `json:"display_order"`
	// SubtitleURL (Phase 1 §4): gửi chuỗi rỗng "" hoặc null để gỡ phụ đề đang có.
	SubtitleURL *string `json:"subtitle_url"`
	// ArticleBody / QuizID (C1): chỉ áp dụng cho nội dung đã là article / quiz. Đổi loại từ/sang
	// article|quiz bị từ chối (CONTENT_TYPE_IMMUTABLE).
	ArticleBody *string    `json:"article_body"`
	QuizID      *uuid.UUID `json:"quiz_id"`
}

type LessonContentResponseDTO struct {
	ID            uuid.UUID  `json:"id"`
	LessonID      uuid.UUID  `json:"lesson_id"`
	Type          string     `json:"type"`
	Title         *string    `json:"title,omitempty"`
	VideoURL      *string    `json:"video_url,omitempty"`
	VideoHLSURL   *string    `json:"video_hls_url,omitempty"`
	VideoUploadID *string    `json:"video_upload_id,omitempty"`
	ThumbnailURL  *string    `json:"thumbnail_url,omitempty"`
	Duration      int        `json:"duration"`
	ExerciseID    *uuid.UUID `json:"exercise_id,omitempty"`
	IsMandatory   bool       `json:"is_mandatory"`
	// LivestreamSessionID (N10, review vòng 2): KHÔNG dùng omitempty — web coi thiếu trường,
	// null, và chuỗi rỗng là tương đương ("phiên chưa sẵn sàng", xem
	// web/src/lib/lesson-content-link.ts), nhưng hợp đồng team-lead yêu cầu rõ là `uuid|null`
	// nên field này luôn xuất hiện trong response, giá trị null khi chưa có phiên.
	LivestreamSessionID *uuid.UUID `json:"livestream_session_id"`
	DisplayOrder        int        `json:"display_order"`
	// SubtitleURL (Phase 1 §4): null khi bài chưa có phụ đề — web tự ẩn panel transcript.
	SubtitleURL *string `json:"subtitle_url"`
	// ArticleBody / ReadingTimeMinutes / QuizID (C1): chỉ xuất hiện khi áp dụng. ReadingTimeMinutes
	// TÍNH lúc trả response từ nội dung bài, không lưu DB.
	ArticleBody        *string    `json:"article_body,omitempty"`
	ReadingTimeMinutes *int       `json:"reading_time_minutes,omitempty"`
	QuizID             *uuid.UUID `json:"quiz_id,omitempty"`
	CreatedAt          time.Time  `json:"created_at"`
	UpdatedAt          time.Time  `json:"updated_at"`
}
