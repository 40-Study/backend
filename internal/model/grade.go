package model

import (
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

// ============================================================================
// GRADE - Quan ly diem so
// ============================================================================

type GradeType string

const (
	GradeAssignment    GradeType = "assignment"
	GradeQuiz          GradeType = "quiz"
	GradeMidterm       GradeType = "midterm"
	GradeFinal         GradeType = "final"
	GradeAttendance    GradeType = "attendance"
	GradeParticipation GradeType = "participation"
	GradeProject       GradeType = "project"
	GradeOther         GradeType = "other"
)

// Grade - Bang diem hoc sinh
type Grade struct {
	BaseModel
	StudentID    uuid.UUID       `gorm:"type:uuid;not null;index" json:"student_id"`
	ClassID      uuid.UUID       `gorm:"type:uuid;not null;index" json:"class_id"`
	AssignmentID *uuid.UUID      `gorm:"type:uuid;index" json:"assignment_id,omitempty"`
	QuizID       *uuid.UUID      `gorm:"type:uuid;index" json:"quiz_id,omitempty"`
	SessionID    *uuid.UUID      `gorm:"type:uuid;index" json:"session_id,omitempty"` // Buoi hoc
	GradeType    GradeType       `gorm:"type:varchar(20);not null;check:grade_type IN ('assignment','quiz','midterm','final','attendance','participation','project','other')" json:"grade_type"`
	Title        string          `gorm:"type:varchar(255);not null" json:"title"` // Ten cot diem
	Score        decimal.Decimal `gorm:"type:decimal(5,2);not null" json:"score"`
	MaxScore     decimal.Decimal `gorm:"type:decimal(5,2);not null" json:"max_score"`
	Weight       decimal.Decimal `gorm:"type:decimal(5,4);default:1.0" json:"weight"` // Trong so (VD: 0.3 = 30%)
	GradedBy     uuid.UUID       `gorm:"type:uuid;not null" json:"graded_by"`
	GradedAt     time.Time       `gorm:"default:CURRENT_TIMESTAMP" json:"graded_at"`
	Feedback     *string         `gorm:"type:text" json:"feedback,omitempty"`
	IsFinal      bool            `gorm:"default:false" json:"is_final"` // Da chot diem

	// Relationships
	Student    User          `gorm:"foreignKey:StudentID;constraint:OnDelete:CASCADE" json:"-"`
	Class      Class         `gorm:"foreignKey:ClassID;constraint:OnDelete:CASCADE" json:"-"`
	Assignment *Assignment   `gorm:"foreignKey:AssignmentID" json:"-"`
	Quiz       *Quiz         `gorm:"foreignKey:QuizID" json:"-"`
	Session    *ClassSession `gorm:"foreignKey:SessionID" json:"-"`
	Grader     User          `gorm:"foreignKey:GradedBy" json:"-"`
}

func (Grade) TableName() string {
	return "grades"
}

// ============================================================================
// GRADE COLUMN - Dinh nghia cot diem trong lop
// ============================================================================

// GradeColumn - Dinh nghia cau truc cot diem cua lop (VD: Giua ky 30%, Cuoi ky 50%, Chuyen can 20%)
type GradeColumn struct {
	BaseModel
	ClassID      uuid.UUID       `gorm:"type:uuid;not null;index" json:"class_id"`
	Name         string          `gorm:"type:varchar(100);not null" json:"name"`
	GradeType    GradeType       `gorm:"type:varchar(20);not null" json:"grade_type"`
	Weight       decimal.Decimal `gorm:"type:decimal(5,4);not null" json:"weight"` // Trong so (VD: 0.3 = 30%)
	MaxScore     decimal.Decimal `gorm:"type:decimal(5,2);default:10.00" json:"max_score"`
	DisplayOrder int             `gorm:"default:0" json:"display_order"`
	IsRequired   bool            `gorm:"default:true" json:"is_required"` // Bat buoc co diem

	// Relationships
	Class Class `gorm:"foreignKey:ClassID;constraint:OnDelete:CASCADE" json:"-"`
}

func (GradeColumn) TableName() string {
	return "grade_columns"
}

// ============================================================================
// FINAL GRADE - Diem tong ket
// ============================================================================

// FinalGrade - Diem tong ket cua hoc sinh trong lop
type FinalGrade struct {
	ID              uuid.UUID        `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	StudentID       uuid.UUID        `gorm:"type:uuid;not null;uniqueIndex:idx_student_class,priority:1" json:"student_id"`
	ClassID         uuid.UUID        `gorm:"type:uuid;not null;uniqueIndex:idx_student_class,priority:2" json:"class_id"`
	WeightedAverage decimal.Decimal  `gorm:"type:decimal(5,2)" json:"weighted_average"` // Diem trung binh co trong so
	LetterGrade     *string          `gorm:"type:varchar(5)" json:"letter_grade,omitempty"` // A, B+, C, etc.
	GPA             *decimal.Decimal `gorm:"type:decimal(3,2)" json:"gpa,omitempty"`        // 4.0 scale
	Rank            *int             `json:"rank,omitempty"`                               // Xep hang trong lop
	Status          string           `gorm:"type:varchar(20);default:'in_progress';check:status IN ('in_progress','passed','failed','incomplete')" json:"status"`
	Notes           *string          `gorm:"type:text" json:"notes,omitempty"`
	CalculatedAt    time.Time        `json:"calculated_at"`
	FinalizedAt     *time.Time       `json:"finalized_at,omitempty"`
	FinalizedBy     *uuid.UUID       `gorm:"type:uuid" json:"finalized_by,omitempty"`
	CreatedAt       time.Time        `gorm:"autoCreateTime" json:"created_at"`
	UpdatedAt       time.Time        `gorm:"autoUpdateTime" json:"updated_at"`

	// Relationships
	Student   User  `gorm:"foreignKey:StudentID;constraint:OnDelete:CASCADE" json:"-"`
	Class     Class `gorm:"foreignKey:ClassID;constraint:OnDelete:CASCADE" json:"-"`
	Finalizer *User `gorm:"foreignKey:FinalizedBy" json:"-"`
}

func (FinalGrade) TableName() string {
	return "final_grades"
}
