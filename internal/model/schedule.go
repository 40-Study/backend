package model

import (
	"time"

	"github.com/google/uuid"
	"github.com/lib/pq"
)

// ============================================================================
// CLASS SCHEDULE - Lich hoc dinh ky
// ============================================================================

// ClassSchedule - Lich hoc dinh ky cua lop (VD: Thu 2, 14:00-15:30)
type ClassSchedule struct {
	BaseModel
	ClassID        uuid.UUID  `gorm:"type:uuid;not null;index" json:"class_id"`
	DayOfWeek      int        `gorm:"not null;check:day_of_week >= 0 AND day_of_week <= 6" json:"day_of_week"` // 0=Sunday, 1=Monday, ..., 6=Saturday
	StartTime      string     `gorm:"type:time;not null" json:"start_time"`                                    // VD: "14:00:00"
	EndTime        string     `gorm:"type:time;not null" json:"end_time"`                                      // VD: "15:30:00"
	Room           *string    `gorm:"type:varchar(100)" json:"room,omitempty"`
	IsActive       bool       `gorm:"default:true" json:"is_active"`
	EffectiveFrom  time.Time  `gorm:"type:date;not null" json:"effective_from"`
	EffectiveUntil *time.Time `gorm:"type:date" json:"effective_until,omitempty"`

	// Relationships
	Class    Class          `gorm:"foreignKey:ClassID;constraint:OnDelete:CASCADE" json:"-"`
	Sessions []ClassSession `gorm:"foreignKey:ScheduleID" json:"-"`
}

func (ClassSchedule) TableName() string {
	return "class_schedules"
}

// ============================================================================
// CLASS SESSION - Buoi hoc cu the
// ============================================================================

type ClassSessionStatus string

const (
	SessionScheduled  ClassSessionStatus = "scheduled"
	SessionInProgress ClassSessionStatus = "in_progress"
	SessionCompleted  ClassSessionStatus = "completed"
	SessionCancelled  ClassSessionStatus = "cancelled"
)

// ClassSession - Buoi hoc cu the (VD: Buoi 1 - 2026-04-10)
type ClassSession struct {
	BaseModel
	ClassID             uuid.UUID          `gorm:"type:uuid;not null;index" json:"class_id"`
	ScheduleID          *uuid.UUID         `gorm:"type:uuid;index" json:"schedule_id,omitempty"` // null = buoi hoc ngoai lich
	SessionNumber       int                `gorm:"not null" json:"session_number"`              // Buoi thu may
	Date                time.Time          `gorm:"type:date;not null;index" json:"date"`
	StartTime           string             `gorm:"type:time;not null" json:"start_time"`
	EndTime             string             `gorm:"type:time;not null" json:"end_time"`
	Status              ClassSessionStatus `gorm:"type:varchar(20);default:'scheduled';check:status IN ('scheduled','in_progress','completed','cancelled')" json:"status"`
	Topic               *string            `gorm:"type:varchar(500)" json:"topic,omitempty"`
	Notes               *string            `gorm:"type:text" json:"notes,omitempty"`
	Materials           *string            `gorm:"type:jsonb" json:"materials,omitempty"`                // JSON array of materials
	LivestreamSessionID *uuid.UUID         `gorm:"type:uuid" json:"livestream_session_id,omitempty"`     // Link toi livestream neu co
	CancelledAt         *time.Time         `json:"cancelled_at,omitempty"`
	CancelReason        *string            `gorm:"type:text" json:"cancel_reason,omitempty"`

	// Relationships
	Class             Class              `gorm:"foreignKey:ClassID;constraint:OnDelete:CASCADE" json:"-"`
	Schedule          *ClassSchedule     `gorm:"foreignKey:ScheduleID" json:"-"`
	LivestreamSession *LivestreamSession `gorm:"foreignKey:LivestreamSessionID" json:"-"`
	Attendances       []SessionAttendance `gorm:"foreignKey:SessionID" json:"-"`
}

func (ClassSession) TableName() string {
	return "class_sessions"
}

// ============================================================================
// SESSION ATTENDANCE - Diem danh theo buoi hoc
// ============================================================================

type AttendanceStatus string

const (
	AttendancePresent AttendanceStatus = "present"
	AttendanceAbsent  AttendanceStatus = "absent"
	AttendanceLate    AttendanceStatus = "late"
	AttendanceExcused AttendanceStatus = "excused"
)

// SessionAttendance - Diem danh chi tiet theo buoi hoc (thay the Attendance cu)
type SessionAttendance struct {
	ID               uuid.UUID        `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	SessionID        uuid.UUID        `gorm:"type:uuid;not null;uniqueIndex:idx_session_student,priority:1" json:"session_id"`
	StudentID        uuid.UUID        `gorm:"type:uuid;not null;uniqueIndex:idx_session_student,priority:2" json:"student_id"`
	Status           AttendanceStatus `gorm:"type:varchar(20);not null;check:status IN ('present','absent','late','excused')" json:"status"`
	CheckInTime      *time.Time       `json:"check_in_time,omitempty"`
	CheckOutTime     *time.Time       `json:"check_out_time,omitempty"`
	ExpectedTime     *string          `gorm:"type:time" json:"expected_time,omitempty"`
	LateMinutes      int              `gorm:"default:0" json:"late_minutes"`
	EarlyLeaveMinutes int             `gorm:"default:0" json:"early_leave_minutes"`
	Location         *string          `gorm:"type:varchar(50)" json:"location,omitempty"` // online, offline, room_name
	DeviceInfo       *string          `gorm:"type:jsonb" json:"device_info,omitempty"`
	VerifiedBy       *uuid.UUID       `gorm:"type:uuid" json:"verified_by,omitempty"`
	Note             *string          `gorm:"type:text" json:"note,omitempty"`
	CreatedAt        time.Time        `gorm:"autoCreateTime" json:"created_at"`
	UpdatedAt        time.Time        `gorm:"autoUpdateTime" json:"updated_at"`

	// Relationships
	Session  ClassSession `gorm:"foreignKey:SessionID;constraint:OnDelete:CASCADE" json:"-"`
	Student  User         `gorm:"foreignKey:StudentID;constraint:OnDelete:CASCADE" json:"-"`
	Verifier *User        `gorm:"foreignKey:VerifiedBy" json:"-"`
}

func (SessionAttendance) TableName() string {
	return "session_attendances"
}

// ============================================================================
// REMINDER SETTING - Cai dat nhac nho
// ============================================================================

// ReminderSetting - Cai dat nhac nho cho tung user
type ReminderSetting struct {
	ID                  uuid.UUID      `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	UserID              uuid.UUID      `gorm:"type:uuid;not null;uniqueIndex:idx_user_event_type,priority:1" json:"user_id"`
	EventType           string         `gorm:"type:varchar(30);not null;uniqueIndex:idx_user_event_type,priority:2;check:event_type IN ('livestream','assignment','class','quiz')" json:"event_type"`
	RemindBeforeMinutes pq.Int32Array  `gorm:"type:int[]" json:"remind_before_minutes"` // VD: [30, 60, 1440]
	Channels            pq.StringArray `gorm:"type:text[]" json:"channels"`             // VD: ["email", "push", "sms"]
	IsEnabled           bool           `gorm:"default:true" json:"is_enabled"`
	CreatedAt           time.Time      `gorm:"autoCreateTime" json:"created_at"`
	UpdatedAt           time.Time      `gorm:"autoUpdateTime" json:"updated_at"`

	// Relationships
	User User `gorm:"foreignKey:UserID;constraint:OnDelete:CASCADE" json:"-"`
}

func (ReminderSetting) TableName() string {
	return "reminder_settings"
}

// ============================================================================
// EXTENSION REQUEST - Xin gia han deadline
// ============================================================================

type ExtensionRequestStatus string

const (
	ExtensionPending  ExtensionRequestStatus = "pending"
	ExtensionApproved ExtensionRequestStatus = "approved"
	ExtensionRejected ExtensionRequestStatus = "rejected"
)

// ExtensionRequest - Yeu cau gia han deadline
type ExtensionRequest struct {
	BaseModel
	AssignmentID   uuid.UUID              `gorm:"type:uuid;not null;index" json:"assignment_id"`
	StudentID      uuid.UUID              `gorm:"type:uuid;not null;index" json:"student_id"`
	Reason         string                 `gorm:"type:text;not null" json:"reason"`
	RequestedDays  int                    `gorm:"not null" json:"requested_days"`
	RequestedUntil *time.Time             `gorm:"type:timestamp" json:"requested_until,omitempty"`
	Status         ExtensionRequestStatus `gorm:"type:varchar(20);default:'pending';check:status IN ('pending','approved','rejected')" json:"status"`
	ReviewedBy     *uuid.UUID             `gorm:"type:uuid" json:"reviewed_by,omitempty"`
	ReviewedAt     *time.Time             `json:"reviewed_at,omitempty"`
	ReviewNote     *string                `gorm:"type:text" json:"review_note,omitempty"`
	ApprovedUntil  *time.Time             `gorm:"type:timestamp" json:"approved_until,omitempty"` // Deadline moi neu approved

	// Relationships
	Assignment *Assignment `gorm:"foreignKey:AssignmentID;constraint:OnDelete:CASCADE" json:"-"`
	Student    User        `gorm:"foreignKey:StudentID;constraint:OnDelete:CASCADE" json:"-"`
	Reviewer   *User       `gorm:"foreignKey:ReviewedBy" json:"-"`
}

func (ExtensionRequest) TableName() string {
	return "extension_requests"
}
