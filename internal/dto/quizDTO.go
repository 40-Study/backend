package dto

import (
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

// ============================================================================
// QUIZ
// ============================================================================

type CreateQuizDTO struct {
	LessonID           string `json:"lesson_id" validate:"omitempty,uuid"`
	CourseID           string `json:"course_id" validate:"omitempty,uuid"`
	SessionID          string `json:"session_id" validate:"omitempty,uuid"`
	Title              string `json:"title" validate:"required,min=3,max=255"`
	Description        string `json:"description,omitempty"`
	TimeLimitMins      *int   `json:"time_limit_minutes,omitempty"`
	PassPercentage     *float64 `json:"pass_percentage,omitempty"`
	MaxAttempts        *int   `json:"max_attempts,omitempty"`
	TriggerType        string `json:"trigger_type" validate:"omitempty,oneof=manual scheduled ai_triggered video_checkpoint"`
	ScheduledAt        string `json:"scheduled_at,omitempty"`
	VideoTimestamp     *int   `json:"video_timestamp,omitempty"`
	ShuffleQuestions   *bool  `json:"shuffle_questions,omitempty"`
	ShuffleAnswers     *bool  `json:"shuffle_answers,omitempty"`
	ShowCorrectAnswers *bool  `json:"show_correct_answers,omitempty"`
}

type UpdateQuizDTO struct {
	Title              *string  `json:"title" validate:"omitempty,min=3,max=255"`
	Description        *string  `json:"description"`
	TimeLimitMins      *int     `json:"time_limit_minutes"`
	PassPercentage     *float64 `json:"pass_percentage"`
	MaxAttempts        *int     `json:"max_attempts"`
	TriggerType        *string  `json:"trigger_type" validate:"omitempty,oneof=manual scheduled ai_triggered video_checkpoint"`
	ScheduledAt        *string  `json:"scheduled_at"`
	VideoTimestamp     *int     `json:"video_timestamp"`
	ShuffleQuestions   *bool    `json:"shuffle_questions"`
	ShuffleAnswers     *bool    `json:"shuffle_answers"`
	ShowCorrectAnswers *bool    `json:"show_correct_answers"`
}

type QuizResponseDTO struct {
	ID                 uuid.UUID        `json:"id"`
	LessonID           *uuid.UUID       `json:"lesson_id,omitempty"`
	CourseID           *uuid.UUID       `json:"course_id,omitempty"`
	SessionID          *uuid.UUID       `json:"session_id,omitempty"`
	Title              string           `json:"title"`
	Description        *string          `json:"description,omitempty"`
	TimeLimitMins      *int             `json:"time_limit_minutes,omitempty"`
	PassPercentage     decimal.Decimal  `json:"pass_percentage"`
	MaxAttempts        *int             `json:"max_attempts,omitempty"`
	TriggerType        string           `json:"trigger_type"`
	ScheduledAt        *time.Time       `json:"scheduled_at,omitempty"`
	VideoTimestamp     *int             `json:"video_timestamp,omitempty"`
	ShuffleQuestions   bool             `json:"shuffle_questions"`
	ShuffleAnswers     bool             `json:"shuffle_answers"`
	ShowCorrectAnswers bool             `json:"show_correct_answers"`
	QuestionCount      int              `json:"question_count"`
	AttemptCount       int              `json:"attempt_count,omitempty"`
	CreatedAt          time.Time        `json:"created_at"`
	UpdatedAt          time.Time        `json:"updated_at"`
}

type QuizListDTO struct {
	Data     []QuizResponseDTO `json:"data"`
	Total    int64             `json:"total"`
	Page     int               `json:"page"`
	PageSize int               `json:"page_size"`
}

type QuizDetailDTO struct {
	QuizResponseDTO
	Questions []QuestionResponseDTO `json:"questions"`
}

// ============================================================================
// QUESTION
// ============================================================================

type CreateQuestionDTO struct {
	QuestionText string              `json:"question_text" validate:"required"`
	QuestionType string              `json:"question_type" validate:"required,oneof=single_choice multiple_choice true_false fill_blank essay"`
	Explanation  string              `json:"explanation,omitempty"`
	Points       *float64            `json:"points,omitempty"`
	DisplayOrder int                 `json:"display_order"`
	ImageURL     string              `json:"image_url,omitempty"`
	Answers      []CreateAnswerDTO   `json:"answers" validate:"required_unless=QuestionType essay,dive"`
}

type UpdateQuestionDTO struct {
	QuestionText *string           `json:"question_text"`
	QuestionType *string           `json:"question_type" validate:"omitempty,oneof=single_choice multiple_choice true_false fill_blank essay"`
	Explanation  *string           `json:"explanation"`
	Points       *float64          `json:"points"`
	DisplayOrder *int              `json:"display_order"`
	ImageURL     *string           `json:"image_url"`
	Answers      []CreateAnswerDTO `json:"answers,omitempty"`
}

type CreateAnswerDTO struct {
	AnswerText   string `json:"answer_text" validate:"required"`
	IsCorrect    bool   `json:"is_correct"`
	DisplayOrder int    `json:"display_order"`
}

type QuestionResponseDTO struct {
	ID           uuid.UUID           `json:"id"`
	QuizID       uuid.UUID           `json:"quiz_id"`
	QuestionText string              `json:"question_text"`
	QuestionType string              `json:"question_type"`
	Explanation  *string             `json:"explanation,omitempty"`
	Points       decimal.Decimal     `json:"points"`
	DisplayOrder int                 `json:"display_order"`
	ImageURL     *string             `json:"image_url,omitempty"`
	Answers      []AnswerResponseDTO `json:"answers"`
}

type AnswerResponseDTO struct {
	ID           uuid.UUID `json:"id"`
	AnswerText   string    `json:"answer_text"`
	IsCorrect    bool      `json:"is_correct,omitempty"`
	DisplayOrder int       `json:"display_order"`
}

type ReorderQuestionsDTO struct {
	QuestionIDs []string `json:"question_ids" validate:"required,min=1"`
}

type BulkCreateQuestionsDTO struct {
	Questions []CreateQuestionDTO `json:"questions" validate:"required,min=1,dive"`
}

// ============================================================================
// QUIZ ATTEMPT
// ============================================================================

type StartQuizResponseDTO struct {
	AttemptID     uuid.UUID             `json:"attempt_id"`
	QuizID        uuid.UUID             `json:"quiz_id"`
	Title         string                `json:"title"`
	TimeLimitMins *int                  `json:"time_limit_minutes,omitempty"`
	Questions     []AttemptQuestionDTO  `json:"questions"`
	StartedAt     time.Time             `json:"started_at"`
}

type AttemptQuestionDTO struct {
	ID           uuid.UUID           `json:"id"`
	QuestionText string              `json:"question_text"`
	QuestionType string              `json:"question_type"`
	Points       decimal.Decimal     `json:"points"`
	DisplayOrder int                 `json:"display_order"`
	ImageURL     *string             `json:"image_url,omitempty"`
	Answers      []AttemptAnswerDTO  `json:"answers"`
}

type AttemptAnswerDTO struct {
	ID           uuid.UUID `json:"id"`
	AnswerText   string    `json:"answer_text"`
	DisplayOrder int       `json:"display_order"`
}

type SubmitQuizDTO struct {
	Answers []SubmitAnswerDTO `json:"answers" validate:"required"`
}

type SubmitAnswerDTO struct {
	QuestionID        string   `json:"question_id" validate:"required,uuid"`
	SelectedAnswerIDs []string `json:"selected_answer_ids,omitempty"`
	TextAnswer        string   `json:"text_answer,omitempty"`
}

type SaveAnswerDTO struct {
	QuestionID        string   `json:"question_id" validate:"required,uuid"`
	SelectedAnswerIDs []string `json:"selected_answer_ids,omitempty"`
	TextAnswer        string   `json:"text_answer,omitempty"`
}

type QuizAttemptResponseDTO struct {
	ID            uuid.UUID        `json:"id"`
	UserID        uuid.UUID        `json:"user_id"`
	QuizID        uuid.UUID        `json:"quiz_id"`
	Score         *decimal.Decimal `json:"score,omitempty"`
	TotalPoints   *decimal.Decimal `json:"total_points,omitempty"`
	Percentage    *decimal.Decimal `json:"percentage,omitempty"`
	IsPassed      *bool            `json:"is_passed,omitempty"`
	TimeSpentSecs *int             `json:"time_spent_seconds,omitempty"`
	StartedAt     time.Time        `json:"started_at"`
	CompletedAt   *time.Time       `json:"completed_at,omitempty"`
}

type QuizAttemptDetailDTO struct {
	QuizAttemptResponseDTO
	Answers []QuizAttemptAnswerDTO `json:"answers"`
}

type QuizAttemptAnswerDTO struct {
	ID                uuid.UUID       `json:"id"`
	QuestionID        uuid.UUID       `json:"question_id"`
	QuestionText      string          `json:"question_text"`
	SelectedAnswerIDs []string        `json:"selected_answer_ids"`
	TextAnswer        *string         `json:"text_answer,omitempty"`
	IsCorrect         *bool           `json:"is_correct,omitempty"`
	PointsEarned      decimal.Decimal `json:"points_earned"`
	CorrectAnswerIDs  []string        `json:"correct_answer_ids,omitempty"`
	Explanation       *string         `json:"explanation,omitempty"`
}

type QuizResultsDTO struct {
	QuizID       uuid.UUID                `json:"quiz_id"`
	Title        string                   `json:"title"`
	TotalStudents int                     `json:"total_students"`
	Attempts     []QuizAttemptResponseDTO `json:"attempts"`
}

type QuizStatisticsDTO struct {
	QuizID          uuid.UUID       `json:"quiz_id"`
	Title           string          `json:"title"`
	TotalAttempts   int             `json:"total_attempts"`
	AverageScore    decimal.Decimal `json:"average_score"`
	HighestScore    decimal.Decimal `json:"highest_score"`
	LowestScore     decimal.Decimal `json:"lowest_score"`
	PassRate        decimal.Decimal `json:"pass_rate"`
	AverageTimeSecs int             `json:"average_time_seconds"`
}
