package dto

import "github.com/google/uuid"

// Học sinh gửi mời phụ huynh
type InviteParentRequestDto struct {
	Email        string  `json:"email" validate:"required,email,max=255"`                                             // email của phụ huynh được mời
	Relationship string  `json:"relationship" default:"parent" validate:"required,oneof=parent guardian grandparent"` // mối quan hệ với học sinh
	Message      *string `json:"message,omitempty" validate:"omitempty,max=500"`                                      // lời nhắn từ học sinh (optional)
}

type RespondInvitationRequestDto struct {
	Action string `json:"action" validate:"required,oneof=accept reject"`
}

// Response sau khi gửi mời
type InviteParentResponseDto struct {
	InvitationID uuid.UUID `json:"invitation_id"`
	Status       string    `json:"status"`        // "invited" hoặc "pending"
	ParentExists bool      `json:"parent_exists"` // email đã có TK?
	Message      string    `json:"message"`       // thông báo cho FE hiển thị
}

// Phụ huynh xem invitation cần respond
type PendingInvitationDto struct {
	ID              uuid.UUID `json:"id"`                       // ID của invitation
	StudentName     string  `json:"student_name"`             // tên học sinh gửi lời mời
	StudentUsername string  `json:"student_username"`         // username của học sinh, dùng để hiển thị trong link tới profile
	StudentAvatar   *string `json:"student_avatar,omitempty"` // avatar của học sinh
	Relationship    string  `json:"relationship"`             // mối quan hệ: parent, guardian, grandparent
	Message         *string `json:"message,omitempty"`        // lời nhắn từ học sinh
	CreatedAt       string  `json:"created_at"`               // thời gian gửi lời mời
	ExpiresAt       string  `json:"expires_at"`               // thời hạn lời mời
}

// Học sinh xem invitation đã gửi
type SentInvitationDto struct {
	ID           uuid.UUID `json:"id"`                     // ID của invitation
	InviteeEmail string  `json:"invitee_email"`          // email của phụ huynh được mời
	InviteeName  *string `json:"invitee_name,omitempty"` // tên PH nếu đã có TK
	Status       string  `json:"status"`                 // trạng thái: invited, pending, accepted, rejected, expired, revoked
	Relationship string  `json:"relationship"`           // mối quan hệ: parent, guardian, grandparent
	CreatedAt    string  `json:"created_at"`             // thời gian gửi lời mời
	RespondedAt  *string `json:"responded_at,omitempty"` // thời gian PH respond (accept/reject), null nếu chưa respond
}

// Validate invitation token (public endpoint, khi click link trong email)
type ValidateInvitationTokenDto struct {
	Valid        bool      `json:"valid"`                   // true nếu token hợp lệ và chưa expired
	InvitationID uuid.UUID `json:"invitation_id,omitempty"` // ID của invitation nếu token hợp lệ, null nếu token không hợp lệ
	StudentName  string    `json:"student_name,omitempty"`  // tên học sinh gửi lời mời, dùng để hiển thị trong email
	Relationship string    `json:"relationship,omitempty"`  // mối quan hệ: parent, guardian, grandparent
	HasAccount   bool      `json:"has_account"`             // true nếu email đã có TK
	Email        string    `json:"email,omitempty"`         // email để pre-fill form đăng ký
}
