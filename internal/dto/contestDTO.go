package dto

import (
	"time"

	"github.com/google/uuid"
)

// ============================================================================
// CONTEST DTOs
// ============================================================================

type CreateContestRequest struct {
	Title           string    `json:"title" validate:"required,min=2,max=255"`
	Description     *string   `json:"description,omitempty"`
	Type            string    `json:"type" validate:"required,oneof=CODING QUIZ MIXED"`
	StartTime       time.Time `json:"start_time" validate:"required"`
	EndTime         time.Time `json:"end_time" validate:"required"`
	Duration        *int      `json:"duration_minutes,omitempty"`
	MaxParticipants int       `json:"max_participants" validate:"omitempty,min=0"`
	IsPublic        *bool     `json:"is_public,omitempty"`
}

type UpdateContestRequest struct {
	Title           *string    `json:"title,omitempty" validate:"omitempty,min=2,max=255"`
	Description     *string    `json:"description,omitempty"`
	Type            *string    `json:"type,omitempty" validate:"omitempty,oneof=CODING QUIZ MIXED"`
	StartTime       *time.Time `json:"start_time,omitempty"`
	EndTime         *time.Time `json:"end_time,omitempty"`
	Duration        *int       `json:"duration_minutes,omitempty"`
	MaxParticipants *int       `json:"max_participants,omitempty" validate:"omitempty,min=0"`
	IsPublic        *bool      `json:"is_public,omitempty"`
}

type ContestResponse struct {
	ID               uuid.UUID                   `json:"id"`
	Title            string                      `json:"title"`
	Slug             string                      `json:"slug"`
	Description      *string                     `json:"description,omitempty"`
	BannerURL        *string                     `json:"banner_url,omitempty"`
	Type             string                      `json:"type"`
	Status           string                      `json:"status"`
	StartTime        time.Time                   `json:"start_time"`
	EndTime          time.Time                   `json:"end_time"`
	Duration         *int                        `json:"duration_minutes,omitempty"`
	MaxParticipants  int                         `json:"max_participants"`
	ParticipantCount int                         `json:"participant_count"`
	IsPublic         bool                        `json:"is_public"`
	CreatedBy        uuid.UUID                   `json:"created_by"`
	OrganizationID   *uuid.UUID                  `json:"organization_id,omitempty"`
	MyParticipation  *ContestParticipantResponse `json:"my_participation,omitempty"`
	CreatedAt        time.Time                   `json:"created_at"`
	UpdatedAt        time.Time                   `json:"updated_at"`
}

type ContestListResponse struct {
	Contests   []ContestResponse `json:"contests"`
	TotalCount int64             `json:"total_count"`
	Page       int               `json:"page"`
	Limit      int               `json:"limit"`
}

// ============================================================================
// CONTEST PROBLEM DTOs
// ============================================================================

type CreateProblemRequest struct {
	Title        string      `json:"title" validate:"required,min=2,max=255"`
	Description  string      `json:"description" validate:"required"`
	Type         string      `json:"type" validate:"required,oneof=CODE MULTIPLE_CHOICE SHORT_ANSWER"`
	Difficulty   string      `json:"difficulty" validate:"required,oneof=EASY MEDIUM HARD"`
	Points       int         `json:"points" validate:"required,min=1"`
	DisplayOrder int         `json:"display_order" validate:"omitempty,min=0"`
	TimeLimit    *int        `json:"time_limit_seconds,omitempty"`
	MemoryLimit  *int        `json:"memory_limit_mb,omitempty"`
	InputFormat  *string     `json:"input_format,omitempty"`
	OutputFormat *string     `json:"output_format,omitempty"`
	SampleInput  *string     `json:"sample_input,omitempty"`
	SampleOutput *string     `json:"sample_output,omitempty"`
	TestCases    interface{} `json:"test_cases,omitempty"`
	Options      interface{} `json:"options,omitempty"`
	CorrectAnswer *string    `json:"correct_answer,omitempty"`
}

type UpdateProblemRequest struct {
	Title        *string     `json:"title,omitempty" validate:"omitempty,min=2,max=255"`
	Description  *string     `json:"description,omitempty"`
	Type         *string     `json:"type,omitempty" validate:"omitempty,oneof=CODE MULTIPLE_CHOICE SHORT_ANSWER"`
	Difficulty   *string     `json:"difficulty,omitempty" validate:"omitempty,oneof=EASY MEDIUM HARD"`
	Points       *int        `json:"points,omitempty" validate:"omitempty,min=1"`
	DisplayOrder *int        `json:"display_order,omitempty" validate:"omitempty,min=0"`
	TimeLimit    *int        `json:"time_limit_seconds,omitempty"`
	MemoryLimit  *int        `json:"memory_limit_mb,omitempty"`
	InputFormat  *string     `json:"input_format,omitempty"`
	OutputFormat *string     `json:"output_format,omitempty"`
	SampleInput  *string     `json:"sample_input,omitempty"`
	SampleOutput *string     `json:"sample_output,omitempty"`
	TestCases    interface{} `json:"test_cases,omitempty"`
	Options      interface{} `json:"options,omitempty"`
	CorrectAnswer *string    `json:"correct_answer,omitempty"`
}

type ContestProblemResponse struct {
	ID           uuid.UUID `json:"id"`
	ContestID    uuid.UUID `json:"contest_id"`
	Title        string    `json:"title"`
	Description  string    `json:"description"`
	Type         string    `json:"type"`
	Difficulty   string    `json:"difficulty"`
	Points       int       `json:"points"`
	DisplayOrder int       `json:"display_order"`
	TimeLimit    *int      `json:"time_limit_seconds,omitempty"`
	MemoryLimit  *int      `json:"memory_limit_mb,omitempty"`
	InputFormat  *string   `json:"input_format,omitempty"`
	OutputFormat *string   `json:"output_format,omitempty"`
	SampleInput  *string   `json:"sample_input,omitempty"`
	SampleOutput *string   `json:"sample_output,omitempty"`
	TestCases    interface{} `json:"test_cases,omitempty"`
	Options      interface{} `json:"options,omitempty"`
	CorrectAnswer *string  `json:"correct_answer,omitempty"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

// ============================================================================
// CONTEST PARTICIPANT DTOs
// ============================================================================

type ContestParticipantResponse struct {
	ID         uuid.UUID  `json:"id"`
	ContestID  uuid.UUID  `json:"contest_id"`
	UserID     uuid.UUID  `json:"user_id"`
	UserName   string     `json:"user_name"`
	AvatarURL  *string    `json:"avatar_url,omitempty"`
	TotalScore int        `json:"total_score"`
	Rank       *int       `json:"rank,omitempty"`
	StartedAt  *time.Time `json:"started_at,omitempty"`
	FinishedAt *time.Time `json:"finished_at,omitempty"`
	CreatedAt  time.Time  `json:"created_at"`
}

type ContestLeaderboardResponse struct {
	Participants []ContestParticipantResponse `json:"participants"`
	TotalCount   int64                        `json:"total_count"`
}

// ============================================================================
// CONTEST SUBMISSION DTOs
// ============================================================================

type SubmitAnswerRequest struct {
	Code     *string `json:"code,omitempty"`
	Language *string `json:"language,omitempty"`
	Answer   *string `json:"answer,omitempty"`
}

type ContestSubmissionResponse struct {
	ID            uuid.UUID  `json:"id"`
	ContestID     uuid.UUID  `json:"contest_id"`
	ProblemID     uuid.UUID  `json:"problem_id"`
	UserID        uuid.UUID  `json:"user_id"`
	Code          *string    `json:"code,omitempty"`
	Language      *string    `json:"language,omitempty"`
	Answer        *string    `json:"answer,omitempty"`
	Score         int        `json:"score"`
	MaxScore      int        `json:"max_score"`
	Status        string     `json:"status"`
	ExecutionTime *int       `json:"execution_time_ms,omitempty"`
	MemoryUsed    *int       `json:"memory_used_kb,omitempty"`
	Output        *string    `json:"output,omitempty"`
	CreatedAt     time.Time  `json:"created_at"`
}
