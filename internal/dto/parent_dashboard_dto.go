package dto

import "time"

// ChildOverviewDto - Tổng quan thông tin con
type ChildOverviewDto struct {
	ID           string  `json:"id"`
	Username     string  `json:"username"`
	FullName     *string `json:"full_name,omitempty"`
	AvatarURL    *string `json:"avatar_url,omitempty"`
	Email        string  `json:"email"`
	Relationship string  `json:"relationship"`

	// Stats
	TotalXP           int `json:"total_xp"`
	CurrentStreak     int `json:"current_streak"`
	EnrolledCourses   int `json:"enrolled_courses"`
	CompletedCourses  int `json:"completed_courses"`
	TotalStudyMinutes int `json:"total_study_minutes"`

	// Permissions
	CanViewProgress   bool `json:"can_view_progress"`
	CanViewGrades     bool `json:"can_view_grades"`
	CanViewAttendance bool `json:"can_view_attendance"`
}

// ChildCourseDto - Khóa học của con
type ChildCourseDto struct {
	ID              string     `json:"id"`
	CourseID        string     `json:"course_id"`
	CourseName      string     `json:"course_name"`
	CourseThumbnail *string    `json:"course_thumbnail,omitempty"`
	InstructorName  string     `json:"instructor_name"`
	ProgressPercent float64    `json:"progress_percent"`
	LastAccessedAt  *time.Time `json:"last_accessed_at,omitempty"`
	CompletedAt     *time.Time `json:"completed_at,omitempty"`
	EnrolledAt      time.Time  `json:"enrolled_at"`
}

// ChildCoursesResponseDto - Response cho danh sách khóa học
type ChildCoursesResponseDto struct {
	Courses  []ChildCourseDto `json:"courses"`
	Total    int64            `json:"total"`
	Page     int              `json:"page"`
	PageSize int              `json:"page_size"`
}

// ChildGradeDto - Điểm của con
type ChildGradeDto struct {
	ID         string    `json:"id"`
	ClassID    string    `json:"class_id"`
	ClassName  string    `json:"class_name"`
	GradeType  string    `json:"grade_type"` // assignment, quiz, midterm, final, etc.
	Title      string    `json:"title"`
	Score      float64   `json:"score"`
	MaxScore   float64   `json:"max_score"`
	Percentage float64   `json:"percentage"`
	Weight     float64   `json:"weight"`
	GradedAt   time.Time `json:"graded_at"`
}

// ChildFinalGradeDto - Điểm tổng kết
type ChildFinalGradeDto struct {
	ClassID         string  `json:"class_id"`
	ClassName       string  `json:"class_name"`
	WeightedAverage float64 `json:"weighted_average"`
	LetterGrade     string  `json:"letter_grade"`
	GPA             float64 `json:"gpa"`
	Rank            *int    `json:"rank,omitempty"`
	Status          string  `json:"status"` // in_progress, passed, failed
}

// ChildGradesResponseDto - Response cho điểm
type ChildGradesResponseDto struct {
	Grades      []ChildGradeDto      `json:"grades"`
	FinalGrades []ChildFinalGradeDto `json:"final_grades"`
}

// ChildScheduleItemDto - Một buổi học trong tuần
type ChildScheduleItemDto struct {
	ClassID   string `json:"class_id"`
	ClassName string `json:"class_name"`
	DayOfWeek int    `json:"day_of_week"` // 0=Sunday, 1=Monday, etc.
	StartTime string `json:"start_time"`  // HH:MM
	EndTime   string `json:"end_time"`    // HH:MM
	Room      string `json:"room,omitempty"`
}

// ChildUpcomingSessionDto - Buổi học sắp tới
type ChildUpcomingSessionDto struct {
	ID            string    `json:"id"`
	ClassID       string    `json:"class_id"`
	ClassName     string    `json:"class_name"`
	SessionNumber int       `json:"session_number"`
	Topic         string    `json:"topic,omitempty"`
	Date          time.Time `json:"date"`
	StartTime     string    `json:"start_time"`
	EndTime       string    `json:"end_time"`
	Room          string    `json:"room,omitempty"`
}

// ChildScheduleResponseDto - Response cho lịch học
type ChildScheduleResponseDto struct {
	WeeklySchedule   []ChildScheduleItemDto    `json:"weekly_schedule"`
	UpcomingSessions []ChildUpcomingSessionDto `json:"upcoming_sessions"`
}

// ChildAttendanceDto - Điểm danh
type ChildAttendanceDto struct {
	ID          string    `json:"id"`
	ClassID     string    `json:"class_id"`
	ClassName   string    `json:"class_name"`
	SessionNum  int       `json:"session_number"`
	Date        time.Time `json:"date"`
	Status      string    `json:"status"` // present, absent, late, excused
	CheckInTime *string   `json:"check_in_time,omitempty"`
	LateMinutes int       `json:"late_minutes"`
}

// ChildAttendanceStatsDto - Thống kê điểm danh
type ChildAttendanceStatsDto struct {
	TotalSessions   int     `json:"total_sessions"`
	PresentCount    int     `json:"present_count"`
	AbsentCount     int     `json:"absent_count"`
	LateCount       int     `json:"late_count"`
	ExcusedCount    int     `json:"excused_count"`
	AttendanceRate  float64 `json:"attendance_rate"` // percentage
}

// ChildAttendanceResponseDto - Response cho điểm danh
type ChildAttendanceResponseDto struct {
	Records  []ChildAttendanceDto    `json:"records"`
	Stats    ChildAttendanceStatsDto `json:"stats"`
	Total    int64                   `json:"total"`
	Page     int                     `json:"page"`
	PageSize int                     `json:"page_size"`
}

// ChildAssignmentDto - Bài tập
type ChildAssignmentDto struct {
	ID              string     `json:"id"`
	Title           string     `json:"title"`
	Type            string     `json:"type"` // homework, project, live_coding
	Difficulty      string     `json:"difficulty"`
	ClassName       string     `json:"class_name,omitempty"`
	DueDate         *time.Time `json:"due_date,omitempty"`
	Status          string     `json:"status"` // not_started, in_progress, completed, overdue
	BestVerdict     *string    `json:"best_verdict,omitempty"`
	TestCasesPassed int        `json:"test_cases_passed"`
	TotalTestCases  int        `json:"total_test_cases"`
	SubmissionCount int        `json:"submission_count"`
	LastSubmittedAt *time.Time `json:"last_submitted_at,omitempty"`
}

// ChildAssignmentsResponseDto - Response cho bài tập
type ChildAssignmentsResponseDto struct {
	Assignments []ChildAssignmentDto `json:"assignments"`
	Stats       AssignmentStatsDto   `json:"stats"`
	Total       int64                `json:"total"`
	Page        int                  `json:"page"`
	PageSize    int                  `json:"page_size"`
}

// AssignmentStatsDto - Thống kê bài tập
type AssignmentStatsDto struct {
	TotalAssignments int `json:"total_assignments"`
	Completed        int `json:"completed"`
	InProgress       int `json:"in_progress"`
	NotStarted       int `json:"not_started"`
	Overdue          int `json:"overdue"`
}
