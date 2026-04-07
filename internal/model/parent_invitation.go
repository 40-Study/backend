package model

import (
	"time"

	"github.com/google/uuid"
)

const (
	ParentInvitationStatusInvited  = "invited"
	ParentInvitationStatusPending  = "pending"
	ParentInvitationStatusAccepted = "accepted"
	ParentInvitationStatusRejected = "rejected"
	ParentInvitationStatusExpired  = "expired"
	ParentInvitationStatusRevoked  = "revoked"
)

const (
	ParentStudentRelationStatusActive = "active"
)

type ParentInvitation struct {
	ID        uuid.UUID `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`

	// Học sinh gửi lời mời
	StudentUserID uuid.UUID `gorm:"type:uuid;not null;index:idx_pi_student" json:"student_user_id"`

	// Email phụ huynh được mời (có thể chưa có tài khoản)
	InviteeEmail string `gorm:"type:varchar(255);not null;index:idx_pi_email" json:"invitee_email"`

	// NULL nếu email chưa có TK trong hệ thống
	// Được set khi:
	//   - Email đã tồn tại lúc tạo invitation
	//   - Hoặc phụ huynh tạo TK mới sau đó (LinkInvitationToNewUser)
	InviteeUserID *uuid.UUID `gorm:"type:uuid;index:idx_pi_invitee" json:"invitee_user_id"`

	// Loại quan hệ: parent, guardian, grandparent
	Relationship string `gorm:"type:varchar(50);not null;default:'parent'" json:"relationship"`

	// Trạng thái: invited, pending, accepted, rejected, expired, revoked
	Status string `gorm:"type:varchar(20);not null;default:'invited';index:idx_pi_status" json:"status"`

	// Token ngẫu nhiên gửi qua email, dùng để validate khi click link
	// Sinh bằng crypto/rand, 32 bytes hex encode (64 chars)
	TokenHash string `gorm:"type:varchar(100);uniqueIndex;not null" json:"token_hash"`

	// Thời hạn lời mời (7 ngày kể từ khi tạo)
	ExpiresAt time.Time `gorm:"not null" json:"expires_at"`

	// Thời gian phụ huynh respond (accept hoặc reject)
	RespondedAt *time.Time `json:"responded_at,omitempty"`

	// Lời nhắn từ học sinh (optional)
	Message      *string    `gorm:"type:text" json:"message,omitempty"`
	InvitationID *uuid.UUID `gorm:"type:uuid" json:"invitation_id,omitempty"` // ID của invitation gốc nếu đây là lời mời lại  nghĩa là cái này sẽ
	// link trực tiếp tới model này nếu là lời mời lại, còn nếu là lời mời mới thì sẽ để null

	// Relationships
	Student *User `gorm:"foreignKey:StudentUserID;constraint:OnDelete:CASCADE" json:"student,omitempty"`
	Invitee *User `gorm:"foreignKey:InviteeUserID;constraint:OnDelete:SET NULL" json:"invitee,omitempty"`
}

func (ParentInvitation) TableName() string {
	return "parent_invitations"
}
