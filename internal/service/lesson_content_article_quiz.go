package service

// lesson_content_article_quiz.go — nội dung bài học loại ARTICLE (bài đọc HTML) và QUIZ (liên kết tới quiz)
// của contract C1 (plans/261008-qa-followup-features/contract.md). Tách khỏi lesson_content_service.go để
// file đó không phình thêm: ở đây chỉ có luật kiểm dữ liệu, mã lỗi và phép tính thời gian đọc.

import (
	"context"
	"html"
	"net/http"
	"regexp"
	"strings"
	"unicode/utf8"

	"github.com/google/uuid"
	"study.com/v1/internal/model"
)

const (
	// maxArticleBodyChars — trần độ dài article_body (ký tự Unicode, không phải byte) của C1.
	maxArticleBodyChars = 200000
	// articleWordsPerMinute — tốc độ đọc dùng để tính reading_time_minutes.
	articleWordsPerMinute = 200
)

// LessonContentRuleError — lỗi nghiệp vụ của nội dung bài học kèm status + mã máy đọc (C1). Handler dịch
// thẳng ra envelope {message, code}; message tiếng Việt có dấu nên web hiện nguyên văn.
type LessonContentRuleError struct {
	Status  int
	Code    string
	Message string
}

func (e *LessonContentRuleError) Error() string { return e.Message }

var (
	ErrArticleBodyRequired  = &LessonContentRuleError{http.StatusBadRequest, "ARTICLE_BODY_REQUIRED", "Nội dung bài đọc không được để trống."}
	ErrArticleBodyTooLong   = &LessonContentRuleError{http.StatusBadRequest, "ARTICLE_BODY_TOO_LONG", "Nội dung bài đọc quá dài (tối đa 200.000 ký tự)."}
	ErrQuizIDRequired       = &LessonContentRuleError{http.StatusBadRequest, "QUIZ_ID_REQUIRED", "Vui lòng chọn bài kiểm tra cần gắn vào bài học."}
	ErrContentQuizNotFound  = &LessonContentRuleError{http.StatusNotFound, "QUIZ_NOT_FOUND", "Không tìm thấy bài kiểm tra này."}
	ErrQuizLessonMismatch   = &LessonContentRuleError{http.StatusConflict, "QUIZ_LESSON_MISMATCH", "Bài kiểm tra này thuộc bài học khác."}
	ErrQuizAlreadyLinked    = &LessonContentRuleError{http.StatusConflict, "QUIZ_ALREADY_LINKED", "Bài kiểm tra này đã được gắn làm nội dung bài học."}
	ErrContentTypeImmutable = &LessonContentRuleError{http.StatusBadRequest, "CONTENT_TYPE_IMMUTABLE", "Không thể đổi loại nội dung từ hoặc sang bài đọc / bài kiểm tra."}
)

var (
	articleScriptStyleRE = regexp.MustCompile(`(?is)<(script|style)\b.*?</(script|style)\s*>`)
	articleTagRE         = regexp.MustCompile(`(?s)<[^>]*>`)
	articleImageRE       = regexp.MustCompile(`(?i)<img\b`)
)

// isArticleOrQuiz — hai loại không đổi sang/từ được (C1: CONTENT_TYPE_IMMUTABLE), vì cột article_body /
// quiz_id gắn chặt với loại.
func isArticleOrQuiz(contentType string) bool {
	return contentType == model.LessonContentTypeArticle || contentType == model.LessonContentTypeQuiz
}

// articlePlainText — văn bản nhìn thấy của HTML bài đọc: bỏ script/style, bỏ thẻ, giải mã thực thể. Chỉ dùng
// để đếm từ và nhận ra bài rỗng, KHÔNG phải bước làm sạch XSS (render phía web đi qua sanitize-html.ts).
func articlePlainText(body string) string {
	text := articleScriptStyleRE.ReplaceAllString(body, " ")
	text = articleTagRE.ReplaceAllString(text, " ")
	return html.UnescapeString(text)
}

// validateArticleBody — bắt buộc có nội dung nhìn thấy (chữ hoặc ảnh), không rỗng sau khi cắt khoảng trắng
// (kể cả Tiptap rỗng "<p></p>"), tối đa maxArticleBodyChars ký tự.
func validateArticleBody(body *string) error {
	if body == nil {
		return ErrArticleBodyRequired
	}
	if utf8.RuneCountInString(*body) > maxArticleBodyChars {
		return ErrArticleBodyTooLong
	}
	if strings.TrimSpace(articlePlainText(*body)) == "" && !articleImageRE.MatchString(*body) {
		return ErrArticleBodyRequired
	}
	return nil
}

// articleReadingTimeMinutes = max(1, ceil(số từ / 200)). TÍNH khi trả response, không lưu: sửa bài là số
// phút đúng ngay, không cần bước đồng bộ.
func articleReadingTimeMinutes(body string) int {
	words := len(strings.Fields(articlePlainText(body)))
	minutes := (words + articleWordsPerMinute - 1) / articleWordsPerMinute
	if minutes < 1 {
		return 1
	}
	return minutes
}

// resolveQuizLink — kiểm quiz có gắn được vào bài `lessonID` không: tồn tại (chưa xoá mềm), thuộc đúng bài
// này, và chưa là nội dung của dòng khác (ngoại trừ `selfContentID` khi đang sửa chính dòng đó).
//
// Quyền: caller đã qua requireLessonCourseOwnerOrAdmin cho bài này, và quiz.LessonID == bài này nên quiz
// thuộc cùng khoá — chủ khoá/admin của bài là chủ khoá/admin của quiz (CreateQuiz đã buộc lesson_id và
// course_id cùng khoá, xem quiz_course_guard.go), không cần dò lại khoá thứ hai.
func (s *LessonContentService) resolveQuizLink(ctx context.Context, lessonID uuid.UUID, quizID *uuid.UUID, selfContentID *uuid.UUID) error {
	if quizID == nil || *quizID == uuid.Nil {
		return ErrQuizIDRequired
	}
	quiz, err := s.lessonRepo.GetQuizForContent(ctx, *quizID)
	if err != nil {
		return err
	}
	if quiz == nil {
		return ErrContentQuizNotFound
	}
	if quiz.LessonID == nil || *quiz.LessonID != lessonID {
		return ErrQuizLessonMismatch
	}
	holder, err := s.lessonRepo.GetContentByQuizID(ctx, *quizID)
	if err != nil {
		return err
	}
	if holder != nil && (selfContentID == nil || holder.ID != *selfContentID) {
		return ErrQuizAlreadyLinked
	}
	return nil
}

// applyCreateTypeFields điền các cột riêng của loại article/quiz lên content mới. Trường không thuộc loại
// (article_body của video, quiz_id của bài đọc...) không được lưu.
func (s *LessonContentService) applyCreateTypeFields(ctx context.Context, content *model.LessonContent, articleBody *string, quizID *uuid.UUID) error {
	switch content.Type {
	case model.LessonContentTypeArticle:
		if err := validateArticleBody(articleBody); err != nil {
			return err
		}
		content.ArticleBody = articleBody
	case model.LessonContentTypeQuiz:
		if err := s.resolveQuizLink(ctx, content.LessonID, quizID, nil); err != nil {
			return err
		}
		content.QuizID = quizID
	}
	return nil
}

// applyUpdateTypeFields áp article_body / quiz_id của request sửa lên nội dung ĐÃ là article / quiz.
// Loại không đổi được (đã chặn ở caller), nên chỉ cần xét theo content.Type hiện có.
func (s *LessonContentService) applyUpdateTypeFields(ctx context.Context, content *model.LessonContent, articleBody *string, quizID *uuid.UUID) error {
	switch content.Type {
	case model.LessonContentTypeArticle:
		if articleBody == nil {
			return nil
		}
		if err := validateArticleBody(articleBody); err != nil {
			return err
		}
		content.ArticleBody = articleBody
	case model.LessonContentTypeQuiz:
		if quizID == nil || (content.QuizID != nil && *quizID == *content.QuizID) {
			return nil
		}
		if err := s.resolveQuizLink(ctx, content.LessonID, quizID, &content.ID); err != nil {
			return err
		}
		content.QuizID = quizID
	}
	return nil
}

// mapQuizLinkWriteError — hai request cùng gắn một quiz: cả hai qua được resolveQuizLink, bên thua gặp vi phạm
// unique uq_lesson_contents_quiz_id. Trả về 409 nghiệp vụ thay vì 500.
func mapQuizLinkWriteError(content *model.LessonContent, err error) error {
	if err != nil && content.Type == model.LessonContentTypeQuiz && isDuplicateKey(err) {
		return ErrQuizAlreadyLinked
	}
	return err
}
