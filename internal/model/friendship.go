package model

import (
	"time"

	"github.com/google/uuid"
)

// Trạng thái lời mời/quan hệ bạn bè. SSOT cho CHECK constraint chk_friendships_status
// (sinh bằng buildCheckConstraintSQL trong database/migrations.go).
const (
	FriendshipStatusPending  = "PENDING"
	FriendshipStatusAccepted = "ACCEPTED"
	FriendshipStatusDeclined = "DECLINED"
	// FriendshipStatusCancelled: người gửi thu hồi lời mời. Contract nói "xoá hẳn dòng" (người dùng
	// không còn thấy gì) nhưng nếu xoá thật thì gửi-huỷ-gửi-huỷ vòng quanh mọi người sẽ lách hạn mức
	// 20 lời mời/24h và mỗi lần gửi lại một thông báo. Giữ dòng ở trạng thái này để hạn mức và
	// cooldown tính được; API không bao giờ trả trạng thái này ra ngoài (coi như không có quan hệ).
	FriendshipStatusCancelled = "CANCELLED"
)

// FriendshipStatuses — SSOT các trạng thái hợp lệ.
var FriendshipStatuses = []string{
	FriendshipStatusPending,
	FriendshipStatusAccepted,
	FriendshipStatusDeclined,
	FriendshipStatusCancelled,
}

// Friendship — một cặp học viên. Mỗi CẶP chỉ có đúng một dòng bất kể chiều gửi: unique index biểu thức
// uq_friendships_pair (LEAST/GREATEST) tạo trong RunPostMigrations, vì GORM không khai báo được index
// biểu thức. Nhờ vậy hai người gửi cho nhau cùng lúc không tạo được hai dòng.
//
// KHÔNG dùng BaseModel (soft-delete): huỷ kết bạn xoá hẳn dòng để unique index không kẹt lần kết bạn lại.
//
// Cột thời gian:
//   - RequestedAt: lần gửi lời mời gần nhất. Gửi lại sau cooldown ghi đè dòng cũ nên phải có cột riêng,
//     CreatedAt không đủ. Hạn mức 20 lời mời/24h đếm theo cột này.
//   - RespondedAt: lúc chấp nhận/từ chối/thu hồi. Cooldown sau từ chối tính từ đây, và "since" của
//     danh sách bạn bè cũng lấy từ đây.
type Friendship struct {
	ID          uuid.UUID  `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
	RequesterID uuid.UUID  `gorm:"type:uuid;not null;check:chk_friendships_no_self,requester_id <> addressee_id;index:idx_friendships_requester,priority:1" json:"requester_id"`
	AddresseeID uuid.UUID  `gorm:"type:uuid;not null;index:idx_friendships_addressee,priority:1" json:"addressee_id"`
	Status      string     `gorm:"type:varchar(20);not null;default:'PENDING';index:idx_friendships_addressee,priority:2;index:idx_friendships_requester,priority:2" json:"status"`
	RequestedAt time.Time  `gorm:"not null;index:idx_friendships_requester,priority:3" json:"requested_at"`
	RespondedAt *time.Time `json:"responded_at,omitempty"`

	Requester User `gorm:"foreignKey:RequesterID;constraint:OnDelete:CASCADE" json:"-"`
	Addressee User `gorm:"foreignKey:AddresseeID;constraint:OnDelete:CASCADE" json:"-"`
}

func (Friendship) TableName() string {
	return "friendships"
}

// UserBlock — BlockerID chặn BlockedID. Không soft-delete (bỏ chặn xoá hẳn), unique theo cặp có hướng.
// Chặn có hiệu lực hai chiều khi kiểm tra (người bị chặn cũng không gửi được lời mời tới người chặn).
type UserBlock struct {
	ID        uuid.UUID `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	CreatedAt time.Time `json:"created_at"`
	BlockerID uuid.UUID `gorm:"type:uuid;not null;uniqueIndex:uq_user_blocks_pair,priority:1;check:chk_user_blocks_no_self,blocker_id <> blocked_id" json:"blocker_id"`
	BlockedID uuid.UUID `gorm:"type:uuid;not null;uniqueIndex:uq_user_blocks_pair,priority:2;index:idx_user_blocks_blocked" json:"blocked_id"`

	Blocker User `gorm:"foreignKey:BlockerID;constraint:OnDelete:CASCADE" json:"-"`
	Blocked User `gorm:"foreignKey:BlockedID;constraint:OnDelete:CASCADE" json:"-"`
}

func (UserBlock) TableName() string {
	return "user_blocks"
}
