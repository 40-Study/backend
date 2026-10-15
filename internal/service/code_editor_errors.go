package service

import "errors"

// Sentinel lỗi của code editor (contract sec. 1). Handler đổi chúng sang HTTP + {code,error,message} qua
// respondCodeEditorError (internal/handler/code_editor_error_response.go); service bọc thêm ngữ cảnh bằng %w.
var (
	// ErrJudgeUnavailable: Judge0 không trả lời được (lỗi kết nối, timeout, HTTP 5xx, 4xx khác 422) -> 503.
	ErrJudgeUnavailable = errors.New("judge unavailable")
	// ErrUnsupportedLanguage: khoá ngôn ngữ không chạy được (contract sec. 3) hoặc không thuộc bài tập -> 400.
	ErrUnsupportedLanguage = errors.New("unsupported language")
	// ErrCodeTooLarge: code vượt MaxSourceCodeBytes -> 413.
	ErrCodeTooLarge = errors.New("code too large")
	// ErrNoTestCases: nộp bài cho bài tập chưa có test case -> 400.
	ErrNoTestCases = errors.New("exercise has no test cases")
	// ErrInvalidScope: scope/prefix bản nháp sai cú pháp (contract sec. 4) -> 400.
	ErrInvalidScope = errors.New("invalid draft scope")
	// ErrDraftLimitReached: người dùng đã có MaxDraftsPerUser bản nháp -> 409.
	ErrDraftLimitReached = errors.New("draft limit reached")
)
