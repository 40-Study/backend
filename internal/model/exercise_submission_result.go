package model

import (
	"time"

	"github.com/google/uuid"
)

// ExerciseSubmissionResult: kết quả MỘT test case của một bài nộp bài tập khoá học (contract sec. 6).
//
// TestCaseID cố ý là *uuid.UUID trơn, KHÔNG có field association tới ExerciseTestCase: AutoMigrate chỉ sinh FK từ
// association, nên không có FK và lịch sử chấm còn nguyên khi giảng viên xoá test case. DisplayOrder/IsHidden được
// chụp lại lúc chấm vì test case có thể bị sửa/xoá sau đó. SubmissionID có FK ON DELETE CASCADE (association
// Submission bên dưới): xoá bài nộp thì xoá kết quả theo.
type ExerciseSubmissionResult struct {
	ID              uuid.UUID         `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	SubmissionID    uuid.UUID         `gorm:"type:uuid;not null;index" json:"submission_id"`
	TestCaseID      *uuid.UUID        `gorm:"type:uuid" json:"test_case_id"`
	DisplayOrder    int               `gorm:"not null;default:0" json:"display_order"`
	IsHidden        bool              `gorm:"not null;default:false" json:"is_hidden"`
	Verdict         SubmissionVerdict `gorm:"type:varchar(32);not null" json:"verdict"`
	ActualOutput    string            `gorm:"type:text" json:"actual_output"` // cắt ở MaxStoredOutputBytes trước khi ghi
	ErrorOutput     string            `gorm:"type:text" json:"error_output"`  // stderr hoặc compile output, cắt như trên
	ExecutionTimeMs *int              `gorm:"column:execution_time_ms" json:"execution_time_ms,omitempty"`
	MemoryUsedKB    *int              `gorm:"column:memory_used_kb" json:"memory_used_kb,omitempty"`

	CreatedAt time.Time `json:"created_at"`

	Submission *ExerciseSubmission `gorm:"foreignKey:SubmissionID;constraint:OnDelete:CASCADE" json:"-"`
}

func (ExerciseSubmissionResult) TableName() string {
	return "exercise_submission_results"
}
