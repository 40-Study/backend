package dto

import (
	"time"

	"github.com/google/uuid"
)

// DTO của hợp đồng code editor (plans/261015-student-code-editor/contract.md, sec. 1, 4, 5). Tập key JSON được khoá
// bởi internal/service/exercise_contract_test.go và fixture web; đổi key ở đây là đổi hợp đồng.

// UpdateExerciseTestCaseDTO: PUT /exercises/:id/testcases/:testCaseId (E10), mọi field tuỳ chọn (nil = giữ nguyên).
type UpdateExerciseTestCaseDTO struct {
	Input          *string `json:"input"`
	ExpectedOutput *string `json:"expected_output"`
	IsHidden       *bool   `json:"is_hidden"`
	DisplayOrder   *int    `json:"display_order"`
}

// SubmissionUserSummaryDTO: người nộp trên danh sách của người quản lý (E7). Không có email (model.User ẩn email).
type SubmissionUserSummaryDTO struct {
	ID       uuid.UUID `json:"id"`
	UserName string    `json:"user_name"`
	FullName string    `json:"full_name"`
}

// ExerciseSubmissionResultDTO: kết quả một test case (E6). Với người không quản lý bài tập, ca ẩn bỏ Input,
// ExpectedOutput, ActualOutput, ErrorOutput (nil -> không có key). TestCaseID null khi ca đã bị xoá.
type ExerciseSubmissionResultDTO struct {
	TestCaseID      *uuid.UUID `json:"test_case_id"`
	DisplayOrder    int        `json:"display_order"`
	IsHidden        bool       `json:"is_hidden"`
	Verdict         string     `json:"verdict"`
	Input           *string    `json:"input,omitempty"`
	ExpectedOutput  *string    `json:"expected_output,omitempty"`
	ActualOutput    *string    `json:"actual_output,omitempty"`
	ErrorOutput     *string    `json:"error_output,omitempty"`
	ExecutionTimeMs *int       `json:"execution_time_ms,omitempty"`
	MemoryUsedKB    *int       `json:"memory_used_kb,omitempty"`
}

// ExerciseSubmissionDetailDTO: GET /exercises/:id/submissions/:submissionId (E6). Results luôn là mảng (rỗng khi
// chưa chấm xong hoặc judge_unavailable), theo display_order.
type ExerciseSubmissionDetailDTO struct {
	ExerciseSubmissionResponseDTO
	Results []ExerciseSubmissionResultDTO `json:"results"`
}

// CodeDraftDTO: GET /code-drafts?scope= (E12) và kết quả PUT /code-drafts (E13).
type CodeDraftDTO struct {
	Scope     string    `json:"scope"`
	Language  string    `json:"language"`
	Code      string    `json:"code"`
	UpdatedAt time.Time `json:"updated_at"`
}

// CodeDraftSummaryDTO: một dòng của GET /code-drafts/list?prefix= (E14), không có code; Size là số byte của code.
type CodeDraftSummaryDTO struct {
	Scope     string    `json:"scope"`
	Language  string    `json:"language"`
	Size      int       `json:"size"`
	UpdatedAt time.Time `json:"updated_at"`
}

// SaveCodeDraftDTO: body của PUT /code-drafts (E13, upsert theo user + scope). Cú pháp scope và language do service
// kiểm (ErrInvalidScope, ErrUnsupportedLanguage, ErrCodeTooLarge).
type SaveCodeDraftDTO struct {
	Scope    string `json:"scope"`
	Language string `json:"language"`
	Code     string `json:"code"`
}

// RunnableLanguageDTO: một dòng của GET /submissions/languages (E2).
type RunnableLanguageDTO struct {
	Key   string `json:"key"`
	Label string `json:"label"`
}

// APIErrorDTO: body lỗi của code editor (contract sec. 1). Code UPPER_SNAKE (web đọc ApiError.code), Error là
// lower_snake của Code, Message là câu tiếng Việt cho người dùng. Không bao giờ chứa URL, host, cổng hay lỗi Go.
type APIErrorDTO struct {
	Code    string `json:"code"`
	Error   string `json:"error"`
	Message string `json:"message"`
}
