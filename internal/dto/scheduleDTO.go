package dto

import (
	"time"

	"github.com/google/uuid"
)

// ============================================================================
// CLASS SCHEDULE
// ============================================================================

type CreateClassScheduleDTO struct {
	DayOfWeek      int    `json:"day_of_week" validate:"required,min=0,max=6"`
	StartTime      string `json:"start_time" validate:"required"`
	EndTime        string `json:"end_time" validate:"required"`
	Room           string `json:"room,omitempty"`
	EffectiveFrom  string `json:"effective_from" validate:"required"`
	EffectiveUntil string `json:"effective_until,omitempty"`
}

type UpdateClassScheduleDTO struct {
	DayOfWeek      *int    `json:"day_of_week" validate:"omitempty,min=0,max=6"`
	StartTime      *string `json:"start_time"`
	EndTime        *string `json:"end_time"`
	Room           *string `json:"room"`
	IsActive       *bool   `json:"is_active"`
	EffectiveFrom  *string `json:"effective_from"`
	EffectiveUntil *string `json:"effective_until"`
}

type ClassScheduleResponseDTO struct {
	ID             uuid.UUID  `json:"id"`
	ClassID        uuid.UUID  `json:"class_id"`
	DayOfWeek      int        `json:"day_of_week"`
	StartTime      string     `json:"start_time"`
	EndTime        string     `json:"end_time"`
	Room           *string    `json:"room,omitempty"`
	IsActive       bool       `json:"is_active"`
	EffectiveFrom  time.Time  `json:"effective_from"`
	EffectiveUntil *time.Time `json:"effective_until,omitempty"`
	CreatedAt      time.Time  `json:"created_at"`
	// Warnings (QA5 Q5-T14, contract C5): chỉ có ý nghĩa ở phản hồi tạo/sửa lịch — lịch này trùng lịch tuần đang hoạt
	// động của lớp khác mà giảng viên cũng dạy. Luôn là mảng (rỗng khi không trùng), không bao giờ chặn thao tác.
	Warnings []ScheduleOverlapWarningDTO `json:"warnings"`
}

// ScheduleOverlapWarningDTO: một lịch tuần của lớp khác trùng giờ (contract C5).
type ScheduleOverlapWarningDTO struct {
	ClassID   uuid.UUID `json:"class_id"`
	ClassName string    `json:"class_name"`
	Weekday   int       `json:"weekday"` // 0=Chủ nhật ... 6=Thứ bảy, cùng quy ước day_of_week
	StartTime string    `json:"start_time"`
	EndTime   string    `json:"end_time"`
}

// ============================================================================
// CLASS SESSION
// ============================================================================

type CreateClassSessionDTO struct {
	Date      string `json:"date" validate:"required"`
	StartTime string `json:"start_time" validate:"required"`
	EndTime   string `json:"end_time" validate:"required"`
	Topic     string `json:"topic,omitempty"`
	Notes     string `json:"notes,omitempty"`
}

type UpdateClassSessionDTO struct {
	Date         *string `json:"date"`
	StartTime    *string `json:"start_time"`
	EndTime      *string `json:"end_time"`
	Status       *string `json:"status" validate:"omitempty,oneof=scheduled in_progress completed cancelled"`
	Topic        *string `json:"topic"`
	Notes        *string `json:"notes"`
	CancelReason *string `json:"cancel_reason"`
}

type GenerateSessionsDTO struct {
	StartDate string `json:"start_date" validate:"required"`
	EndDate   string `json:"end_date" validate:"required"`
}

// Lý do một ngày bị bỏ qua khi sinh buổi (QA4-T06): trùng với buổi của lớp/giảng viên khác, lớp đã có đúng buổi đó, hoặc
// buổi ngày đó đã bị huỷ.
const (
	SkippedSessionOverlap   = "overlap"
	SkippedSessionDuplicate = "duplicate"
	// SkippedSessionCancelled: buổi của lịch lặp ngày đó đã bị huỷ có chủ ý, không sinh lại.
	SkippedSessionCancelled = "cancelled"
)

// SkippedSessionDTO (QA4-T06): một buổi không sinh được, trả về để web báo "bỏ qua 1 buổi trùng lịch (30/11)".
type SkippedSessionDTO struct {
	Date   string `json:"date"` // YYYY-MM-DD
	Reason string `json:"reason"`
}

type ClassSessionResponseDTO struct {
	ID                  uuid.UUID  `json:"id"`
	ClassID             uuid.UUID  `json:"class_id"`
	ScheduleID          *uuid.UUID `json:"schedule_id,omitempty"`
	SessionNumber       int        `json:"session_number"`
	Date                string     `json:"date"`
	StartTime           string     `json:"start_time"`
	EndTime             string     `json:"end_time"`
	Status              string     `json:"status"`
	Topic               *string    `json:"topic,omitempty"`
	Notes               *string    `json:"notes,omitempty"`
	LivestreamSessionID *uuid.UUID `json:"livestream_session_id,omitempty"`
	CancelledAt         *time.Time `json:"cancelled_at,omitempty"`
	CancelReason        *string    `json:"cancel_reason,omitempty"`
	AttendanceCount     int        `json:"attendance_count,omitempty"`
	CreatedAt           time.Time  `json:"created_at"`
}

type ClassSessionListDTO struct {
	Data     []ClassSessionResponseDTO `json:"data"`
	Total    int64                     `json:"total"`
	Page     int                       `json:"page"`
	PageSize int                       `json:"page_size"`
}

// ============================================================================
// SESSION ATTENDANCE
// ============================================================================

type MarkAttendanceDTO struct {
	StudentID string `json:"student_id" validate:"required,uuid"`
	Status    string `json:"status" validate:"required,oneof=present absent late excused"`
	Note      string `json:"note,omitempty"`
}

type BulkMarkAttendanceDTO struct {
	Attendances []MarkAttendanceDTO `json:"attendances" validate:"required,min=1,dive"`
}

type UpdateSessionAttendanceDTO struct {
	Status        *string `json:"status" validate:"omitempty,oneof=present absent late excused"`
	Note          *string `json:"note"`
	LateMinutes   *int    `json:"late_minutes"`
}

type SessionAttendanceResponseDTO struct {
	ID                uuid.UUID  `json:"id"`
	SessionID         uuid.UUID  `json:"session_id"`
	StudentID         uuid.UUID  `json:"student_id"`
	StudentName       string     `json:"student_name"`
	// ClassName/SessionTitle (QA5 R2): lớp và buổi của dòng điểm danh, chỉ điền ở GET /me/attendances
	// (nơi buổi học được preload); các endpoint điểm danh khác để vắng.
	ClassName         string     `json:"class_name,omitempty"`
	SessionTitle      string     `json:"session_title,omitempty"`
	// SessionStartTime (QA7-S7-01, C15): giờ bắt đầu theo lịch của buổi (RFC3339, múi giờ Việt Nam), chỉ điền ở GET /me/attendances.
	// Cột "Thời gian" của học viên là lúc BUỔI HỌC diễn ra, không phải lúc giáo viên ghi điểm danh (created_at).
	SessionStartTime  *time.Time `json:"session_start_time,omitempty"`
	Status            string     `json:"status"`
	CheckInTime       *time.Time `json:"check_in_time,omitempty"`
	CheckOutTime      *time.Time `json:"check_out_time,omitempty"`
	LateMinutes       int        `json:"late_minutes"`
	EarlyLeaveMinutes int        `json:"early_leave_minutes"`
	Note              *string    `json:"note,omitempty"`
	CreatedAt         time.Time  `json:"created_at"`
}

// ============================================================================
// REMINDER SETTING
// ============================================================================

type UpdateReminderSettingDTO struct {
	EventType           string  `json:"event_type" validate:"required,oneof=livestream assignment class quiz"`
	RemindBeforeMinutes []int32 `json:"remind_before_minutes" validate:"required"`
	Channels            []string `json:"channels" validate:"required"`
	IsEnabled           *bool   `json:"is_enabled"`
}

type ReminderSettingResponseDTO struct {
	ID                  uuid.UUID `json:"id"`
	EventType           string    `json:"event_type"`
	RemindBeforeMinutes []int32   `json:"remind_before_minutes"`
	Channels            []string  `json:"channels"`
	IsEnabled           bool      `json:"is_enabled"`
}

// ============================================================================
// TIMETABLE
// ============================================================================

type TimetableEntryDTO struct {
	SessionID   *uuid.UUID `json:"session_id,omitempty"`
	ScheduleID  *uuid.UUID `json:"schedule_id,omitempty"`
	ClassName   string     `json:"class_name"`
	ClassID     uuid.UUID  `json:"class_id"`
	// TeacherName (QA5 R3): giảng viên chính của lớp; không có giảng viên chính thì vắng.
	TeacherName string `json:"teacher_name,omitempty"`
	// OrganizationID/OrganizationName: tổ chức sở hữu lớp. Không có = lớp cá nhân của giảng viên (dạy tự do).
	// Tổ chức đã xoá mềm vẫn trả id nhưng không trả tên, để client hiện "Tổ chức" chung chung.
	OrganizationID   *uuid.UUID `json:"organization_id,omitempty"`
	OrganizationName *string    `json:"organization_name,omitempty"`
	DayOfWeek   int        `json:"day_of_week"`
	Date        *string    `json:"date,omitempty"`
	StartTime   string     `json:"start_time"`
	EndTime     string     `json:"end_time"`
	Room        *string    `json:"room,omitempty"`
	Topic       *string    `json:"topic,omitempty"`
	Status      string     `json:"status"`
	// EffectiveFrom/EffectiveUntil (B-08): khoảng ngày hiệu lực của lịch lặp tuần (YYYY-MM-DD), để web
	// không vẽ lịch đã hết hạn mãi mãi. Chỉ có ở mục lịch lặp (schedule_id), không có ở buổi cụ thể.
	EffectiveFrom  *string `json:"effective_from,omitempty"`
	EffectiveUntil *string `json:"effective_until,omitempty"`
}

// TimetableOccurrenceDTO: một ngày của lịch lặp tuần không còn buổi học vì buổi sinh ra đã bị huỷ.
type TimetableOccurrenceDTO struct {
	ScheduleID uuid.UUID `json:"schedule_id"`
	Date       string    `json:"date"` // YYYY-MM-DD
}

type TimetableResponseDTO struct {
	Entries []TimetableEntryDTO `json:"entries"`
	Week    string              `json:"week,omitempty"`
	// CancelledOccurrences chỉ có khi hỏi kèm khoảng ngày (sessions_from/sessions_to). Buổi huỷ không nằm trong
	// Entries (học viên không cần thấy), nhưng lịch lặp của giảng viên cần biết ngày nào bị huỷ để không vẽ lại.
	CancelledOccurrences []TimetableOccurrenceDTO `json:"cancelled_occurrences,omitempty"`
}

// TimetableOrgFilter: lọc GET /me/timetable theo nguồn lớp. Rỗng = mọi lớp; OrganizationID = chỉ lớp của tổ
// chức đó; PersonalOnly = chỉ lớp cá nhân (organization_id NULL). Không bao giờ đặt cả hai cùng lúc.
type TimetableOrgFilter struct {
	OrganizationID *uuid.UUID
	PersonalOnly   bool
}

// CacheSuffix: phần đuôi khoá cache của thời khoá biểu, để mỗi bộ lọc có cache riêng.
func (f TimetableOrgFilter) CacheSuffix() string {
	switch {
	case f.OrganizationID != nil:
		return ":org:" + f.OrganizationID.String()
	case f.PersonalOnly:
		return ":personal"
	default:
		return ""
	}
}
