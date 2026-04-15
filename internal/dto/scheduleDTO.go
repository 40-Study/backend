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
	DayOfWeek   int        `json:"day_of_week"`
	Date        *string    `json:"date,omitempty"`
	StartTime   string     `json:"start_time"`
	EndTime     string     `json:"end_time"`
	Room        *string    `json:"room,omitempty"`
	Topic       *string    `json:"topic,omitempty"`
	Status      string     `json:"status"`
}

type TimetableResponseDTO struct {
	Entries []TimetableEntryDTO `json:"entries"`
	Week    string              `json:"week,omitempty"`
}
