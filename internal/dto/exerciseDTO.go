package dto

import (
	"time"

	"github.com/google/uuid"
)

// ============================================================================
// COURSE EXERCISE
// ============================================================================

type CreateExerciseDTO struct {
	Title          string   `json:"title" validate:"required,min=3,max=255"`
	Description    string   `json:"description" validate:"required"`
	Difficulty     string   `json:"difficulty" validate:"omitempty,oneof=easy medium hard"`
	Language       []string `json:"language" validate:"required,min=1"`
	StarterCode    string   `json:"starter_code,omitempty"`
	TimeLimit      int      `json:"time_limit,omitempty"`
	MemoryLimit    int      `json:"memory_limit,omitempty"`
	PassPercentage int      `json:"pass_percentage,omitempty"`
}

type UpdateExerciseDTO struct {
	Title          *string   `json:"title" validate:"omitempty,min=3,max=255"`
	Description    *string   `json:"description"`
	Difficulty     *string   `json:"difficulty" validate:"omitempty,oneof=easy medium hard"`
	Language       *[]string `json:"language"`
	StarterCode    *string   `json:"starter_code"`
	TimeLimit      *int      `json:"time_limit"`
	MemoryLimit    *int      `json:"memory_limit"`
	PassPercentage *int      `json:"pass_percentage"`
}

type ExerciseResponseDTO struct {
	ID             uuid.UUID                `json:"id"`
	Title          string                   `json:"title"`
	Description    string                   `json:"description"`
	Difficulty     string                   `json:"difficulty"`
	Language       []string                 `json:"language"`
	StarterCode    string                   `json:"starter_code"`
	TimeLimit      int                      `json:"time_limit"`
	MemoryLimit    int                      `json:"memory_limit"`
	PassPercentage int                      `json:"pass_percentage"`
	TestCaseCount  int                      `json:"test_case_count,omitempty"`
	CreatedAt      time.Time                `json:"created_at"`
	UpdatedAt      time.Time                `json:"updated_at"`
}

type ExerciseListDTO struct {
	Data     []ExerciseResponseDTO `json:"data"`
	Total    int64                 `json:"total"`
	Page     int                   `json:"page"`
	PageSize int                   `json:"page_size"`
}

type ExerciseDetailDTO struct {
	ExerciseResponseDTO
	TestCases      []ExerciseTestCaseResponseDTO `json:"test_cases"`
	LastSubmission *ExerciseSubmissionResponseDTO `json:"last_submission,omitempty"`
}

// ============================================================================
// EXERCISE TEST CASE
// ============================================================================

type CreateExerciseTestCaseDTO struct {
	Input          string `json:"input" validate:"required"`
	ExpectedOutput string `json:"expected_output" validate:"required"`
	IsHidden       bool   `json:"is_hidden"`
	DisplayOrder   int    `json:"display_order"`
}

type ImportExerciseTestCasesDTO struct {
	TestCases []CreateExerciseTestCaseDTO `json:"test_cases" validate:"required,min=1,dive"`
}

type ExerciseTestCaseResponseDTO struct {
	ID             uuid.UUID `json:"id"`
	ExerciseID     uuid.UUID `json:"exercise_id"`
	Input          string    `json:"input"`
	ExpectedOutput string    `json:"expected_output,omitempty"`
	IsHidden       bool      `json:"is_hidden"`
	DisplayOrder   int       `json:"display_order"`
}

// ============================================================================
// EXERCISE SUBMISSION
// ============================================================================

type SubmitExerciseDTO struct {
	Language string `json:"language" validate:"required"`
	Code     string `json:"code" validate:"required"`
}

type ExerciseSubmissionResponseDTO struct {
	ID              uuid.UUID `json:"id"`
	ExerciseID      uuid.UUID `json:"exercise_id"`
	UserID          uuid.UUID `json:"user_id"`
	Language        string    `json:"language"`
	Code            string    `json:"code"`
	Verdict         string    `json:"verdict"`
	TestCasesPassed int       `json:"test_cases_passed"`
	TotalTestCases  int       `json:"total_test_cases"`
	ExecutionTimeMs *int      `json:"execution_time_ms,omitempty"`
	MemoryUsedKB    *int      `json:"memory_used_kb,omitempty"`
	ErrorMessage    *string   `json:"error_message,omitempty"`
	CreatedAt       time.Time `json:"created_at"`
}

type ExerciseSubmissionListDTO struct {
	Data     []ExerciseSubmissionResponseDTO `json:"data"`
	Total    int64                           `json:"total"`
	Page     int                             `json:"page"`
	PageSize int                             `json:"page_size"`
}

// ============================================================================
// CONTENT PROGRESS
// ============================================================================

type UpdateContentProgressDTO struct {
	Status           *string `json:"status" validate:"omitempty,oneof=not_started in_progress completed"`
	VideoWatchedSecs *int    `json:"video_watched_seconds"`
	ExercisePassed   *bool   `json:"exercise_passed"`
}

type ContentProgressResponseDTO struct {
	ID               uuid.UUID  `json:"id"`
	UserID           uuid.UUID  `json:"user_id"`
	LessonContentID  uuid.UUID  `json:"lesson_content_id"`
	Status           string     `json:"status"`
	VideoWatchedSecs int        `json:"video_watched_seconds"`
	ExercisePassed   bool       `json:"exercise_passed"`
	CompletedAt      *time.Time `json:"completed_at,omitempty"`
	LastAccessedAt   time.Time  `json:"last_accessed_at"`
}
