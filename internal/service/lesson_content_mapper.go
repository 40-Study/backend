package service

import (
	"study.com/v1/internal/dto"
	"study.com/v1/internal/model"
)

// lessonContentToDTO là NƠI DUY NHẤT dựng dto.LessonContentResponseDTO từ model.LessonContent (QA 261009 L2, contract
// C1). LessonContentService, SectionService, CourseService và LessonService đều dùng nó, nên một trường mới của nội dung
// (article_body, quiz_id, ...) không thể bị bỏ sót ở curriculum/chi tiết bài như khi mỗi nơi dựng tay.
//
// viewer quyết định phần phụ thuộc người xem (URL video/phụ đề đã ký, xem applyVideoAccess); caller KHÔNG được truyền
// nội dung của bài đang khoá cho người xem không có quyền — đó là việc của ResolveLessonLock ở caller.
func lessonContentToDTO(c *model.LessonContent, viewer videoViewer) dto.LessonContentResponseDTO {
	resp := dto.LessonContentResponseDTO{
		ID:          c.ID,
		LessonID:    c.LessonID,
		Type:        c.Type,
		Title:       c.Title,
		Duration:    c.Duration,
		ExerciseID:  c.ExerciseID,
		IsMandatory: c.IsMandatory,
		// N10 (review vòng 2, từ review web): xem chú thích tại model.LessonContent.
		LivestreamSessionID: c.LivestreamSessionID,
		DisplayOrder:        c.DisplayOrder,
		SubtitleURL:         c.SubtitleURL,
		CreatedAt:           c.CreatedAt,
		UpdatedAt:           c.UpdatedAt,
	}

	// C1: article_body + reading_time_minutes (tính tại đây, không lưu) và quiz_id — omitempty khi không áp dụng.
	if c.Type == model.LessonContentTypeArticle && c.ArticleBody != nil {
		minutes := articleReadingTimeMinutes(*c.ArticleBody)
		resp.ArticleBody = c.ArticleBody
		resp.ReadingTimeMinutes = &minutes
	}
	if c.Type == model.LessonContentTypeQuiz {
		resp.QuizID = c.QuizID
	}

	// Thay URL video đã lưu bằng URL KÝ (video_hls_url), và chỉ chủ khoá/admin mới còn video_url
	// trỏ file gốc — xem applyVideoAccess.
	applyVideoAccess(&resp, c.VideoURL, viewer)
	return resp
}
