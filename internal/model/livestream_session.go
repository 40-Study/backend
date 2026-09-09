package model

import (
	"time"

	"github.com/google/uuid"
)

type LivestreamSessionStatus string

const (
	LivestreamStatusScheduled LivestreamSessionStatus = "scheduled"
	LivestreamStatusLive      LivestreamSessionStatus = "live"
	LivestreamStatusEnded     LivestreamSessionStatus = "ended"
	LivestreamStatusCancelled LivestreamSessionStatus = "cancelled"
)

type ParticipantRole string

const (
	ParticipantRoleTeacher   ParticipantRole = "teacher"
	ParticipantRoleAssistant ParticipantRole = "assistant"
	ParticipantRoleStudent   ParticipantRole = "student"
	ParticipantRoleViewer    ParticipantRole = "viewer"
)

type LivestreamSession struct {
	BaseModel
	Title       string                  `gorm:"type:varchar(255);not null" json:"title"`
	Description *string                 `gorm:"type:text" json:"description,omitempty"`
	HostID          uuid.UUID  `gorm:"type:uuid;not null;index" json:"host_id"`
	ClassID         uuid.UUID  `gorm:"type:uuid;not null;index" json:"class_id"`
	CourseID        *uuid.UUID `gorm:"type:uuid;index" json:"course_id,omitempty"`
	LessonContentID *uuid.UUID `gorm:"type:uuid;index" json:"lesson_content_id,omitempty"`
	RoomName    string                  `gorm:"type:varchar(100);uniqueIndex;not null" json:"room_name"`
	Status      LivestreamSessionStatus `gorm:"type:varchar(20);default:'scheduled';index" json:"status"`
	StartedAt   *time.Time              `gorm:"type:timestamp" json:"started_at,omitempty"`
	EndedAt     *time.Time              `gorm:"type:timestamp" json:"ended_at,omitempty"`
	ScheduledAt *time.Time              `gorm:"type:timestamp" json:"scheduled_at,omitempty"`
	MaxViewers  int64                   `gorm:"default:1000" json:"max_viewers"`
	IsRecorded  bool                    `gorm:"default:true" json:"is_recorded"`

	Settings      LivestreamSettings   `gorm:"type:jsonb;serializer:json;default:'{}'" json:"settings"`
	Class  *Class  `gorm:"foreignKey:ClassID" json:"class,omitempty"`
	Course *Course `gorm:"foreignKey:CourseID" json:"course,omitempty"`
	Participants  []Participant        `gorm:"foreignKey:SessionID;constraint:OnDelete:CASCADE" json:"participants,omitempty"`
	Assignments   []Assignment         `gorm:"foreignKey:SessionID;constraint:OnDelete:CASCADE" json:"assignments,omitempty"`
	ChatMessages  []ChatMessage        `gorm:"foreignKey:SessionID;constraint:OnDelete:CASCADE" json:"chat_messages,omitempty"`
	Analytics     *LivestreamAnalytics `gorm:"foreignKey:SessionID" json:"analytics,omitempty"`
}

func (LivestreamSession) TableName() string {
	return "livestream_sessions"
}

type LivestreamSettings struct {
	IsChatEnabled        bool `json:"is_chat_enabled"`         // Cho phép chat trong buổi livestream
	IsQAEnabled          bool `json:"is_qa_enabled"`           // Cho phép hỏi đáp trong buổi livestream
	IsWhiteboardEnabled  bool `json:"is_whiteboard_enabled"`   // Cho phép sử dụng bảng trắng trong buổi livestream
	IsScreenShareEnabled bool `json:"is_screen_share_enabled"` // Cho phép chia sẻ màn hình trong buổi livestream
	IsPollsEnabled       bool `json:"is_polls_enabled"`        // Cho phép tạo khảo sát trong buổi livestream
	WhiteboardLocked     bool `json:"whiteboard_locked"`       // Trạng thái khóa bảng trắng
}

// Participant đại diện cho một người tham gia trong buổi livestream, có thể là giáo viên, trợ giảng, học sinh hoặc người xem
type Participant struct {
	BaseModel
	SessionID uuid.UUID       `gorm:"type:uuid;not null;index" json:"session_id"`
	UserID    uuid.UUID       `gorm:"type:uuid;not null;index" json:"user_id"`
	Role      ParticipantRole `gorm:"type:varchar(20);default:'viewer'" json:"role"`
	JoinedAt  time.Time       `gorm:"type:timestamp;default:CURRENT_TIMESTAMP" json:"joined_at"`
	LeftAt    *time.Time      `gorm:"type:timestamp" json:"left_at,omitempty"`
	IsActive  bool            `gorm:"default:true" json:"is_active"`

	Session *LivestreamSession `gorm:"foreignKey:SessionID" json:"-"`
	User    *User              `gorm:"foreignKey:UserID" json:"-"`
}

func (Participant) TableName() string {
	return "participants"
}

// ChatMessage (H8, data-model audit 260909; vòng 2 xử lý): trước đây khai thêm field
// `DeletedAt *time.Time` — Go field-promotion rule khiến field CÙNG TÊN ở cấp ngoài (ở đây)
// ĐÈ LÊN field `BaseModel.DeletedAt gorm.DeletedAt` (embedded, cấp trong), nên GORM không
// còn nhận diện được model này có soft-delete convention (`gorm.DeletedAt`) nữa — Delete()
// hoá thành HARD DELETE (xoá vĩnh viễn) và mọi query mặc định KHÔNG tự lọc bản ghi đã xoá,
// dù cột DB `deleted_at` vẫn tồn tại. Cùng lúc còn `IsDeleted bool` xử lý thủ công ở
// chat_message_repository.go (GetBySession/.../CountBySession tự thêm WHERE is_deleted=false)
// — 2 cơ chế xoá chồng nhau, cơ chế GORM tự động thì bị vô hiệu hoá âm thầm.
//
// Đã kiểm tra caller (chat_service.go, chat_message_repository.go) trước khi xoá field này:
//   - `Delete()` (hard-delete) trong repo KHÔNG được gọi ở bất kỳ đâu (chat_service.go chỉ
//     gọi SoftDelete/GetByID/GetBySession/Pin/UnPin) — xoá field không đổi hành vi sống nào,
//     chỉ khiến Delete() (nếu sau này có ai gọi) trở thành soft-delete ĐÚNG như tên hàm.
//   - Không nơi nào đọc trực tiếp `.DeletedAt` của ChatMessage (`toResponseDTO` chỉ map
//     ID/SessionID/UserID/UserName/Message/IsPinned/ParentID/CreatedAt).
//   - `IsDeleted`/`DeletedBy` giữ nguyên — vẫn cần cho filter thủ công hiện có và audit "ai
//     xoá"; GORM's tự động "deleted_at IS NULL" giờ cộng thêm vào các query hiện có là dư
//     nhưng vô hại vì SoftDelete() luôn set `is_deleted`+`deleted_at` cùng lúc, atomic.
//   - Cột DB `deleted_at` không đổi tên/kiểu — `gorm.DeletedAt` mặc định map đúng cột đó.
type ChatMessage struct {
	BaseModel
	SessionID uuid.UUID  `gorm:"type:uuid;not null;index" json:"session_id"`
	UserID    uuid.UUID  `gorm:"type:uuid;not null;index" json:"user_id"`
	Message   string     `gorm:"type:text;not null" json:"message"`
	IsPinned  bool       `gorm:"default:false" json:"is_pinned"`
	IsDeleted bool       `gorm:"default:false" json:"is_deleted"`
	DeletedBy *uuid.UUID `gorm:"type:uuid;index" json:"deleted_by,omitempty"`
	ParentID  *uuid.UUID `gorm:"type:uuid;index" json:"parent_id,omitempty"`

	Session *LivestreamSession `gorm:"foreignKey:SessionID" json:"-"`
	User    *User              `gorm:"foreignKey:UserID" json:"-"`
	Parent  *ChatMessage       `gorm:"foreignKey:ParentID" json:"-"`
}

func (ChatMessage) TableName() string {
	return "chat_messages"
}

type LivestreamAnalytics struct {
	BaseModel
	SessionID        uuid.UUID `gorm:"type:uuid;uniqueIndex;not null" json:"session_id"`
	PeakViewers      int       `gorm:"default:0" json:"peak_viewers"`        // Số lượng người xem cao nhất cùng lúc
	TotalViewers     int       `gorm:"default:0" json:"total_viewers"`       // Tổng số người đã xem buổi livestream (bao gồm cả những người đã rời đi)
	TotalMessages    int       `gorm:"default:0" json:"total_messages"`      // Tổng số tin nhắn đã gửi trong buổi livestream
	AvgWatchTimeSecs int       `gorm:"default:0" json:"avg_watch_time_secs"` // Thời gian xem trung bình của người xem (tính bằng giây)

	Session *LivestreamSession `gorm:"foreignKey:SessionID" json:"-"`
}

func (LivestreamAnalytics) TableName() string {
	return "livestream_analytics"
}
