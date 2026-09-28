package dto

import "time"

// ChildSessionAnalysisResponseDto định nghĩa cấu trúc dữ liệu trả về cho màn hình
// phân tích kết quả và nhận xét chi tiết buổi học của con dành cho Phụ huynh.
type ChildSessionAnalysisResponseDto struct {
	SessionID     string `json:"session_id"`
	SessionCode   string `json:"session_code"`   // Mã ca học hiển thị, VD: "TOAN10-B08"
	SessionNumber int    `json:"session_number"` // Thứ tự buổi học, VD: 8
	LessonTitle   string `json:"lesson_title"`   // Tên bài học / chủ đề buổi học
	ClassName     string `json:"class_name"`     // Tên lớp học, VD: "Toán nâng cao 10"
	SessionDate   string `json:"session_date"`   // Ngày học dạng chuỗi YYYY-MM-DD
	Status        string `json:"status"`         // Trạng thái: "completed", "in_progress", "upcoming", "cancelled"
	StatusLabel   string `json:"status_label"`   // Nhãn hiển thị: "Hoàn thành hôm nay", "Sắp diễn ra"

	// 1. Điểm danh & Chuyên cần trong ca học
	Attendance *SessionAttendanceSummaryDto `json:"attendance,omitempty"`

	// 2. Kết quả kiểm tra / Mini-quiz trên lớp (nếu có)
	QuizResult *SessionQuizAnalysisDto `json:"quiz_result,omitempty"`

	// 3. Đánh giá & Nhận xét của giáo viên giảng dạy
	TeacherFeedback *SessionTeacherFeedbackDto `json:"teacher_feedback,omitempty"`

	// 4. Bài tập về nhà được giao sau buổi học
	Homework *SessionHomeworkTaskDto `json:"homework,omitempty"`

	// 5. Video ghi hình xem lại bài giảng (nếu có)
	Recording *SessionRecordingDto `json:"recording,omitempty"`
}

// SessionAttendanceSummaryDto tóm tắt thời lượng tham gia và điểm danh của học sinh
type SessionAttendanceSummaryDto struct {
	Status          string  `json:"status"`           // "present", "late", "absent"
	CheckInTime     *string `json:"check_in_time"`    // Giờ vào lớp dạng "HH:MM", VD: "08:58"
	AttendedMinutes int     `json:"attended_minutes"` // Số phút học thực tế, VD: 58
	TotalMinutes    int     `json:"total_minutes"`    // Tổng thời lượng ca học tính bằng phút, VD: 60
	AttendanceLabel string  `json:"attendance_label"` // Nhãn hiển thị: "Chuyên cần: Đúng giờ (58/60 phút)"
}

// SessionQuizAnalysisDto kết quả bài kiểm tra nhanh / quiz trên lớp
type SessionQuizAnalysisDto struct {
	Title          string  `json:"title"`            // Tiêu đề: "Quiz & Thực hành tính toán nhanh"
	ScoreLabel     string  `json:"score_label"`      // Đánh giá: "Cần rèn luyện thêm", "Xuất sắc", "Đạt yêu cầu"
	Score          float64 `json:"score"`            // Điểm số đạt được (thang 10), VD: 6.0
	MaxScore       float64 `json:"max_score"`        // Thang điểm tối đa, VD: 10.0
	CorrectCount   int     `json:"correct_count"`    // Số câu trả lời đúng, VD: 3
	TotalQuestions int     `json:"total_questions"`  // Tổng số câu hỏi, VD: 5
	Percentage     float64 `json:"percentage"`       // Tỷ lệ hoàn thành %, VD: 60.0
	TimeSpentMins  int     `json:"time_spent_mins"`  // Thời gian làm bài tính theo phút, VD: 18
	TimeLimitMins  int     `json:"time_limit_mins"`  // Thời gian tối đa cho phép làm bài, VD: 25
	CanViewDetail  bool    `json:"can_view_detail"`  // Cho phép xem lại câu trả lời chi tiết
}

// SessionTeacherFeedbackDto đánh giá và nhận xét của giáo viên dành riêng cho con
type SessionTeacherFeedbackDto struct {
	TeacherID   string     `json:"teacher_id"`
	TeacherName string     `json:"teacher_name"`     // Họ và tên giáo viên, VD: "Cô Phạm Hồng Lan"
	TeacherRole *string    `json:"teacher_role"`     // Học vị / chức danh: "ThS. Toán"
	Subject     string     `json:"subject"`          // Bộ môn phụ trách, VD: "Bộ môn Toán"
	AvatarURL   *string    `json:"avatar_url"`       // Ảnh đại diện của giáo viên
	Comment     string     `json:"comment"`          // Lời nhận xét chi tiết của giáo viên
	CommentedAt *time.Time `json:"commented_at"`     // Thời gian giáo viên gửi nhận xét
	CanChat     bool       `json:"can_chat"`         // Cho phép phụ huynh nhắn tin trao đổi
}

// SessionHomeworkTaskDto bài tập về nhà được giao sau buổi học
type SessionHomeworkTaskDto struct {
	AssignmentID string     `json:"assignment_id"`
	Title        string     `json:"title"`         // Tiêu đề bài tập, VD: "Toán 10 — Bài luyện tập 5: Rút gọn phân số có ẩn"
	DueDate      *time.Time `json:"due_date"`      // Hạn chót nộp bài dạng timestamp ISO 8601
	DueDateText  string     `json:"due_date_text"` // Nhãn hiển thị hạn nộp thân thiện, VD: "20:00 tối nay"
	Status       string     `json:"status"`        // Trạng thái: "pending", "submitted", "graded", "overdue"
}

// SessionRecordingDto thông tin video phát lại ghi hình buổi học
type SessionRecordingDto struct {
	DurationMins int    `json:"duration_mins"` // Thời lượng video tính theo phút, VD: 48
	Quality      string `json:"quality"`       // Chất lượng video, VD: "1080p"
	VideoURL     string `json:"video_url"`     // Đường dẫn video phát lại an toàn
}
