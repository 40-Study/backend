package model

import (
	"time"

	"github.com/google/uuid"
	"github.com/lib/pq"
	"github.com/shopspring/decimal"
	"gorm.io/gorm"
)

type Quiz struct {
	gorm.Model
	ID                 uuid.UUID       `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	LessonID           *uuid.UUID      `gorm:"type:uuid;uniqueIndex" json:"lesson_id,omitempty"`
	CourseID           *uuid.UUID      `gorm:"type:uuid;index" json:"course_id,omitempty"`
	Title              string          `gorm:"type:varchar(255);not null" json:"title"`
	Description        *string         `gorm:"type:text" json:"description,omitempty"`
	TimeLimitMins      *int            `gorm:"column:time_limit_minutes" json:"time_limit_minutes,omitempty"`
	PassPercentage     decimal.Decimal `gorm:"type:decimal(5,2);default:70.00" json:"pass_percentage"`
	MaxAttempts        *int            `gorm:"default:3" json:"max_attempts,omitempty"`
	ShuffleQuestions   bool            `gorm:"default:true" json:"shuffle_questions"`
	ShuffleAnswers     bool            `gorm:"default:true" json:"shuffle_answers"`
	ShowCorrectAnswers bool            `gorm:"default:true" json:"show_correct_answers"`
	IsAIGenerated      bool            `gorm:"default:false" json:"is_ai_generated"`

	// Relationships
	Lesson    *Lesson       `gorm:"foreignKey:LessonID;constraint:OnDelete:CASCADE" json:"-"`
	Course    *Course       `gorm:"foreignKey:CourseID;constraint:OnDelete:CASCADE" json:"-"`
	Questions []Question    `gorm:"foreignKey:QuizID;constraint:OnDelete:CASCADE" json:"-"`
	Attempts  []QuizAttempt `gorm:"foreignKey:QuizID;constraint:OnDelete:CASCADE" json:"-"`
}

func (Quiz) TableName() string {
	return "quizzes"
}

type Question struct {
	gorm.Model
	ID            uuid.UUID       `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	QuizID        uuid.UUID       `gorm:"type:uuid;not null;index" json:"quiz_id"`
	QuestionText  string          `gorm:"type:text;not null" json:"question_text"`
	QuestionType  string          `gorm:"type:varchar(20);not null;check:question_type IN ('single_choice', 'multiple_choice', 'true_false', 'fill_blank', 'essay')" json:"question_type"`
	Explanation   *string         `gorm:"type:text" json:"explanation,omitempty"`
	Points        decimal.Decimal `gorm:"type:decimal(5,2);default:1.00" json:"points"`
	DisplayOrder  int             `gorm:"not null" json:"display_order"`
	ImageURL      *string         `gorm:"type:varchar(500);column:image_url" json:"image_url,omitempty"`
	IsAIGenerated bool            `gorm:"default:false" json:"is_ai_generated"`

	// Relationships
	Quiz    Quiz             `gorm:"foreignKey:QuizID;constraint:OnDelete:CASCADE" json:"-"`
	Answers []QuestionAnswer `gorm:"foreignKey:QuestionID;constraint:OnDelete:CASCADE" json:"-"`
}

func (Question) TableName() string {
	return "questions"
}

type QuestionAnswer struct {
	ID           uuid.UUID `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	CreatedAt    time.Time `json:"created_at"`
	QuestionID   uuid.UUID `gorm:"type:uuid;not null;index" json:"question_id"`
	AnswerText   string    `gorm:"type:text;not null" json:"answer_text"`
	IsCorrect    bool      `gorm:"default:false" json:"is_correct"`
	DisplayOrder int       `gorm:"not null" json:"display_order"`

	// Relationships
	Question Question `gorm:"foreignKey:QuestionID;constraint:OnDelete:CASCADE" json:"-"`
}

func (QuestionAnswer) TableName() string {
	return "question_answers"
}

type QuizAttempt struct {
	ID            uuid.UUID        `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	CreatedAt     time.Time        `json:"created_at"`
	UserID        uuid.UUID        `gorm:"type:uuid;not null;index" json:"user_id"`
	QuizID        uuid.UUID        `gorm:"type:uuid;not null;index" json:"quiz_id"`
	Score         *decimal.Decimal `gorm:"type:decimal(5,2)" json:"score,omitempty"`
	TotalPoints   *decimal.Decimal `gorm:"type:decimal(5,2)" json:"total_points,omitempty"`
	Percentage    *decimal.Decimal `gorm:"type:decimal(5,2)" json:"percentage,omitempty"`
	IsPassed      *bool            `json:"is_passed,omitempty"`
	TimeSpentSecs *int             `gorm:"column:time_spent_seconds" json:"time_spent_seconds,omitempty"`
	StartedAt     time.Time        `gorm:"default:CURRENT_TIMESTAMP" json:"started_at"`
	CompletedAt   *time.Time       `json:"completed_at,omitempty"`

	// Relationships
	User    User                `gorm:"foreignKey:UserID;constraint:OnDelete:CASCADE" json:"-"`
	Quiz    Quiz                `gorm:"foreignKey:QuizID;constraint:OnDelete:CASCADE" json:"-"`
	Answers []QuizAttemptAnswer `gorm:"foreignKey:AttemptID;constraint:OnDelete:CASCADE" json:"-"`
}

func (QuizAttempt) TableName() string {
	return "quiz_attempts"
}

type QuizAttemptAnswer struct {
	ID                uuid.UUID       `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	CreatedAt         time.Time       `json:"created_at"`
	AttemptID         uuid.UUID       `gorm:"type:uuid;not null;index" json:"attempt_id"`
	QuestionID        uuid.UUID       `gorm:"type:uuid;not null;index" json:"question_id"`
	SelectedAnswerIDs pq.StringArray  `gorm:"type:uuid[]" json:"selected_answer_ids"`
	TextAnswer        *string         `gorm:"type:text" json:"text_answer,omitempty"`
	IsCorrect         *bool           `json:"is_correct,omitempty"`
	PointsEarned      decimal.Decimal `gorm:"type:decimal(5,2);default:0" json:"points_earned"`

	// Relationships
	Attempt  QuizAttempt `gorm:"foreignKey:AttemptID;constraint:OnDelete:CASCADE" json:"-"`
	Question Question    `gorm:"foreignKey:QuestionID;constraint:OnDelete:CASCADE" json:"-"`
}

func (QuizAttemptAnswer) TableName() string {
	return "quiz_attempt_answers"
}
