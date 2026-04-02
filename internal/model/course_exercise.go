package model

import (
	"time"

	"github.com/google/uuid"
	"github.com/lib/pq"
)

// CourseExercise - Bài tập trong khóa học (khác với LiveAssignment)
// Học viên phải hoàn thành để tiếp tục học
type CourseExercise struct {
	BaseModel
	Title       string         `gorm:"type:varchar(255);not null" json:"title"`
	Description string         `gorm:"type:text;not null" json:"description"`
	Difficulty  string         `gorm:"type:varchar(20);default:'medium';check:difficulty IN ('easy','medium','hard')" json:"difficulty"`
	Language    pq.StringArray `gorm:"type:text[];not null" json:"language"`
	StarterCode string         `gorm:"type:text" json:"starter_code"`
	TimeLimit   int            `gorm:"default:2" json:"time_limit"`   // seconds per test case
	MemoryLimit int            `gorm:"default:256" json:"memory_limit"` // MB

	// Pass criteria
	PassPercentage int `gorm:"default:100" json:"pass_percentage"` // % test cases cần pass

	// Relations
	TestCases   []ExerciseTestCase   `gorm:"foreignKey:ExerciseID;constraint:OnDelete:CASCADE" json:"test_cases,omitempty"`
	Submissions []ExerciseSubmission `gorm:"foreignKey:ExerciseID;constraint:OnDelete:CASCADE" json:"-"`
}

func (CourseExercise) TableName() string {
	return "course_exercises"
}

// ExerciseTestCase - Test case cho bài tập khóa học
type ExerciseTestCase struct {
	ID             uuid.UUID `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	ExerciseID     uuid.UUID `gorm:"type:uuid;not null;index" json:"exercise_id"`
	Input          string    `gorm:"type:text;not null" json:"input"`
	ExpectedOutput string    `gorm:"type:text;not null" json:"expected_output"`
	IsHidden       bool      `gorm:"default:false" json:"is_hidden"`
	DisplayOrder   int       `gorm:"default:0" json:"display_order"`

	CreatedAt time.Time
}

func (ExerciseTestCase) TableName() string {
	return "exercise_test_cases"
}

// ExerciseSubmission - Bài nộp của học viên cho bài tập khóa học
type ExerciseSubmission struct {
	ID              uuid.UUID `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	ExerciseID      uuid.UUID `gorm:"type:uuid;not null;index" json:"exercise_id"`
	UserID          uuid.UUID `gorm:"type:uuid;not null;index" json:"user_id"`
	Language        string    `gorm:"type:varchar(20);not null" json:"language"`
	Code            string    `gorm:"type:text;not null" json:"code"`
	Verdict         string    `gorm:"type:varchar(20);default:'pending'" json:"verdict"` // pending, accepted, wrong_answer, time_limit, etc.
	TestCasesPassed int       `gorm:"default:0" json:"test_cases_passed"`
	TotalTestCases  int       `gorm:"default:0" json:"total_test_cases"`
	ExecutionTimeMs *int      `gorm:"column:execution_time_ms" json:"execution_time_ms,omitempty"`
	MemoryUsedKB    *int      `gorm:"column:memory_used_kb" json:"memory_used_kb,omitempty"`
	ErrorMessage    *string   `gorm:"type:text" json:"error_message,omitempty"`

	CreatedAt time.Time
	UpdatedAt time.Time

	Exercise *CourseExercise `gorm:"foreignKey:ExerciseID" json:"-"`
	User     *User           `gorm:"foreignKey:UserID" json:"-"`
}

func (ExerciseSubmission) TableName() string {
	return "exercise_submissions"
}

// ContentProgress - Track tiến độ học của user theo từng content
type ContentProgress struct {
	ID              uuid.UUID  `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	UserID          uuid.UUID  `gorm:"type:uuid;not null;index;uniqueIndex:idx_user_content" json:"user_id"`
	LessonContentID uuid.UUID  `gorm:"type:uuid;not null;index;uniqueIndex:idx_user_content" json:"lesson_content_id"`
	Status          string     `gorm:"type:varchar(20);default:'not_started';check:status IN ('not_started','in_progress','completed')" json:"status"`

	// Video progress
	VideoWatchedSecs int `gorm:"default:0" json:"video_watched_seconds"`

	// Exercise progress
	ExercisePassed bool `gorm:"default:false" json:"exercise_passed"`

	CompletedAt    *time.Time `json:"completed_at,omitempty"`
	LastAccessedAt time.Time  `gorm:"default:CURRENT_TIMESTAMP" json:"last_accessed_at"`

	CreatedAt time.Time
	UpdatedAt time.Time

	User          *User          `gorm:"foreignKey:UserID" json:"-"`
	LessonContent *LessonContent `gorm:"foreignKey:LessonContentID" json:"-"`
}

func (ContentProgress) TableName() string {
	return "content_progress"
}
