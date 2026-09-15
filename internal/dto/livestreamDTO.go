package dto

import "github.com/google/uuid"

// CreateLivestreamDTO — KHONG con truong host_id (finding review 260915, PR web #16): host cua
// phien phai la nguoi dang goi API (lay tu access token), khong duoc client tu khai bao trong
// body — truoc day handler nhan thang host_id tu body va dung nguyen de tao session, nen bat ky
// user dang nhap nao cung co the tao livestream mang ten mot user KHAC bang cach doan/lay UUID
// cua ho. hostID gio la tham so rieng cua LivestreamServiceInterface.Create, lay tu
// c.Locals("user_id") o handler — xem LivestreamHandler.Create.
type CreateLivestreamDTO struct {
	Title           string `json:"title" validate:"required,min=3,max=255"`
	Description     string `json:"description"`
	ClassID         string `json:"class_id" validate:"required,uuid"`
	CourseID        string `json:"course_id" validate:"omitempty,uuid"`
	LessonContentID string `json:"lesson_content_id" validate:"omitempty,uuid"`
	MaxViewers      int64  `json:"max_viewers"`
	IsRecorded      bool   `json:"is_recorded"`
	// N9 (review vong 2, 260915): truoc day livestream_service.go nuot loi parse RFC3339 cua
	// truong nay (`if err == nil { session.ScheduledAt = &scheduledTime }`) — client go sai dinh
	// dang van nhan 200, phien duoc tao KHONG lich, KHONG enqueue reminder, ma khong he biet.
	// Validate ngay o DTO de handler tra 400 truoc khi toi service, thay vi im lang bo qua.
	ScheduledAt string `json:"scheduled_at" validate:"omitempty,datetime=2006-01-02T15:04:05Z07:00"`
}

type UpdateLivestreamDTO struct {
	Title       *string `json:"title" validate:"omitempty,min=3,max=255"`
	Description *string `json:"description"`
	MaxViewers  *int64  `json:"max_viewers"`
}

type StartLivestreamDTO struct {
	RoomName string `json:"room_name" validate:"required"`
}

// JoinLivestreamDTO — KHONG con user_id va role (finding review V3-6, issue #58): nguoi tham gia
// phien PHAI la nguoi dang goi API (lay tu access token), va vai tro do SERVER suy ra tu quan he
// that (host / GV lop / instructor khoa / hoc sinh da enroll) chu khong do client tu khai.
//
// Truoc day handler nhan thang `user_id` tu body roi dung lam ca (a) danh tinh nguoi tham gia va
// (b) `Identity` cua LiveKit token — nen bat ky user dang nhap nao cung join duoc DUOI TEN nguoi
// khac chi bang cach khai user_id cua ho; `role` thi nhan nguyen gia tri client gui
// (teacher/assistant/student/viewer) => tu phong minh len teacher, va `IsHost` cua token duoc set
// theo role do. Ca hai duong deu da bi go: userID la tham so rieng cua
// LivestreamServiceInterface.Join, lay tu c.Locals("user_id") o handler; role do service tinh.
//
// `name` duoc giu lai vi chi la ten HIEN THI (LiveKit display name), khong mang quyen.
type JoinLivestreamDTO struct {
	Name string `json:"name" validate:"required"`
}

type LivestreamResponseDTO struct {
	ID              uuid.UUID  `json:"id"`
	Title           string     `json:"title"`
	Description     string     `json:"description"`
	HostID          uuid.UUID  `json:"host_id"`
	ClassID         uuid.UUID  `json:"class_id"`
	CourseID        *uuid.UUID `json:"course_id,omitempty"`
	LessonContentID *uuid.UUID `json:"lesson_content_id,omitempty"`
	RoomName        string     `json:"room_name"`
	Status          string     `json:"status"`
	StartedAt       *string    `json:"started_at,omitempty"`
	EndedAt         *string    `json:"ended_at,omitempty"`
	ScheduledAt     *string    `json:"scheduled_at,omitempty"`
	MaxViewers      int64      `json:"max_viewers"`
	IsRecorded      bool       `json:"is_recorded"`
	Settings        string     `json:"settings"`
	CreatedAt       string     `json:"created_at"`
}

type LivestreamListDTO struct {
	Data     []LivestreamResponseDTO `json:"data"`
	Total    int64                   `json:"total"`
	Page     int                     `json:"page"`
	PageSize int                     `json:"page_size"`
}

type LivekitParticipantDTO struct {
	Identity     string `json:"identity"`
	Name         string `json:"name"`
	State        string `json:"state"`
	JoinedAt     int64  `json:"joined_at"`
	NumTracks    int    `json:"num_tracks"`
	IsPublishing bool   `json:"is_publishing"`
}

type LivestreamDetailDTO struct {
	LivestreamResponseDTO
	// LiveKit real-time data (only present when status=live)
	ActiveParticipants int                     `json:"active_participants"`
	NumPublishers      int                     `json:"num_publishers"`
	ActiveRecording    bool                    `json:"active_recording"`
	Participants       []LivekitParticipantDTO `json:"participants,omitempty"`
}

type ParticipantResponseDTO struct {
	ID        uuid.UUID `json:"id"`
	SessionID uuid.UUID `json:"session_id"`
	UserID    uuid.UUID `json:"user_id"`
	Role      string    `json:"role"`
	JoinedAt  string    `json:"joined_at"`
	LeftAt    *string   `json:"left_at,omitempty"`
	IsActive  bool      `json:"is_active"`
	Token     string    `json:"token,omitempty"`
	ServerURL string    `json:"server_url,omitempty"`
	RoomName  string    `json:"room_name,omitempty"`
}

// ScreenShareDTO — KHONG con user_id (finding review V3-6, issue #58): nguoi bat/tat chia se man
// hinh la nguoi dang goi API, khong phai mot user_id client tu khai. Body cu
// (`{user_id, action}`) van duoc Fiber parse binh thuong — `user_id` bi bo qua lang le, khong doi
// contract response (xem API_DOCUMENTATION.md).
// ScreenShareDTO (D3, issue #58 review vòng 2): UserID là ĐỐI TƯỢNG được host/GV DUYỆT chia sẻ
// màn hình — rỗng = actor tự chia sẻ (host tự bật cam/màn hình của chính mình), khác rỗng = actor
// (đã được canManageSession xác nhận là host/GV lớp/instructor khoá/admin) cấp/thu quyền publish
// cho một học sinh cụ thể. Không phải danh tính người gọi — actor luôn lấy từ access token.
type ScreenShareDTO struct {
	Action string `json:"action" validate:"required,oneof=start stop"`
	UserID string `json:"user_id" validate:"omitempty,uuid"`
}

// ModerationActionDTO — `user_id` o day la DOI TUONG bi tac dong (mute/kick ai), khong phai danh
// tinh nguoi goi; nguoi goi lay tu access token (finding review V3-6, issue #58). Vi vay field nay
// duoc giu nguyen, chi khac truoc la no khong con bi hieu nham thanh nguoi thuc hien.
type ModerationActionDTO struct {
	UserID   string `json:"user_id" validate:"required,uuid"`
	Action   string `json:"action" validate:"required,oneof=mute kick"`
	Reason   string `json:"reason"`
	Duration int    `json:"duration"`
}

type WhiteboardLockDTO struct {
	Locked bool `json:"locked"`
}
