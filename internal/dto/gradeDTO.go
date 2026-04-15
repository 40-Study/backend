package dto

import (
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

// ============================================================================
// GRADE COLUMN
// ============================================================================

type CreateGradeColumnDTO struct {
	Name         string  `json:"name" validate:"required,min=1,max=100"`
	GradeType    string  `json:"grade_type" validate:"required,oneof=assignment quiz midterm final attendance participation project other"`
	Weight       float64 `json:"weight" validate:"required"`
	MaxScore     float64 `json:"max_score,omitempty"`
	DisplayOrder int     `json:"display_order"`
	IsRequired   *bool   `json:"is_required,omitempty"`
}

type UpdateGradeColumnDTO struct {
	Name         *string  `json:"name" validate:"omitempty,min=1,max=100"`
	GradeType    *string  `json:"grade_type" validate:"omitempty,oneof=assignment quiz midterm final attendance participation project other"`
	Weight       *float64 `json:"weight"`
	MaxScore     *float64 `json:"max_score"`
	DisplayOrder *int     `json:"display_order"`
	IsRequired   *bool    `json:"is_required"`
}

type GradeColumnResponseDTO struct {
	ID           uuid.UUID       `json:"id"`
	ClassID      uuid.UUID       `json:"class_id"`
	Name         string          `json:"name"`
	GradeType    string          `json:"grade_type"`
	Weight       decimal.Decimal `json:"weight"`
	MaxScore     decimal.Decimal `json:"max_score"`
	DisplayOrder int             `json:"display_order"`
	IsRequired   bool            `json:"is_required"`
}

type ReorderGradeColumnsDTO struct {
	ColumnIDs []string `json:"column_ids" validate:"required,min=1"`
}

// ============================================================================
// GRADE
// ============================================================================

type CreateGradeDTO struct {
	StudentID    string  `json:"student_id" validate:"required,uuid"`
	GradeType    string  `json:"grade_type" validate:"required,oneof=assignment quiz midterm final attendance participation project other"`
	Title        string  `json:"title" validate:"required,min=1,max=255"`
	Score        float64 `json:"score" validate:"required"`
	MaxScore     float64 `json:"max_score" validate:"required"`
	Weight       float64 `json:"weight,omitempty"`
	AssignmentID string  `json:"assignment_id,omitempty" validate:"omitempty,uuid"`
	QuizID       string  `json:"quiz_id,omitempty" validate:"omitempty,uuid"`
	SessionID    string  `json:"session_id,omitempty" validate:"omitempty,uuid"`
	Feedback     string  `json:"feedback,omitempty"`
}

type UpdateGradeDTO struct {
	Score    *float64 `json:"score"`
	MaxScore *float64 `json:"max_score"`
	Weight   *float64 `json:"weight"`
	Feedback *string  `json:"feedback"`
	IsFinal  *bool    `json:"is_final"`
}

type BulkCreateGradesDTO struct {
	Grades []CreateGradeDTO `json:"grades" validate:"required,min=1,dive"`
}

type GradeResponseDTO struct {
	ID           uuid.UUID       `json:"id"`
	StudentID    uuid.UUID       `json:"student_id"`
	StudentName  string          `json:"student_name"`
	ClassID      uuid.UUID       `json:"class_id"`
	AssignmentID *uuid.UUID      `json:"assignment_id,omitempty"`
	QuizID       *uuid.UUID      `json:"quiz_id,omitempty"`
	SessionID    *uuid.UUID      `json:"session_id,omitempty"`
	GradeType    string          `json:"grade_type"`
	Title        string          `json:"title"`
	Score        decimal.Decimal `json:"score"`
	MaxScore     decimal.Decimal `json:"max_score"`
	Weight       decimal.Decimal `json:"weight"`
	GradedBy     uuid.UUID       `json:"graded_by"`
	GradedAt     time.Time       `json:"graded_at"`
	Feedback     *string         `json:"feedback,omitempty"`
	IsFinal      bool            `json:"is_final"`
}

type GradeBookDTO struct {
	ClassID  uuid.UUID               `json:"class_id"`
	Columns  []GradeColumnResponseDTO `json:"columns"`
	Students []StudentGradeRowDTO    `json:"students"`
}

type StudentGradeRowDTO struct {
	StudentID   uuid.UUID         `json:"student_id"`
	StudentName string            `json:"student_name"`
	Grades      []GradeResponseDTO `json:"grades"`
}

// ============================================================================
// FINAL GRADE
// ============================================================================

type FinalGradeResponseDTO struct {
	ID              uuid.UUID        `json:"id"`
	StudentID       uuid.UUID        `json:"student_id"`
	StudentName     string           `json:"student_name"`
	ClassID         uuid.UUID        `json:"class_id"`
	WeightedAverage decimal.Decimal  `json:"weighted_average"`
	LetterGrade     *string          `json:"letter_grade,omitempty"`
	GPA             *decimal.Decimal `json:"gpa,omitempty"`
	Rank            *int             `json:"rank,omitempty"`
	Status          string           `json:"status"`
	Notes           *string          `json:"notes,omitempty"`
	CalculatedAt    time.Time        `json:"calculated_at"`
	FinalizedAt     *time.Time       `json:"finalized_at,omitempty"`
}

type UpdateFinalGradeDTO struct {
	LetterGrade *string  `json:"letter_grade"`
	GPA         *float64 `json:"gpa"`
	Notes       *string  `json:"notes"`
}
