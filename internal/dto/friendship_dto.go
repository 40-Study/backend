package dto

import (
	"time"

	"github.com/google/uuid"
)

// Kiểu dữ liệu của contract-api.md §1 (Bạn bè). JSON snake_case, KHÔNG có email/số điện thoại.

// Giá trị RelationStatus của GET /friends/relationship/:userId và kết quả tìm kiếm.
const (
	RelationNone        = "NONE"
	RelationFriends     = "FRIENDS"
	RelationPendingOut  = "PENDING_OUT"
	RelationPendingIn   = "PENDING_IN"
	RelationBlockedByMe = "BLOCKED_BY_ME"
	RelationSelf        = "SELF"
)

// FriendUserDTO — thông tin tối thiểu về một học viên.
type FriendUserDTO struct {
	UserID    uuid.UUID `json:"user_id"`
	UserName  string    `json:"user_name"`
	FullName  *string   `json:"full_name,omitempty"`
	AvatarURL *string   `json:"avatar_url,omitempty"`
}

// FriendTargetRequest — body của POST /friends/requests và POST /friends/blocks.
// user_id để dạng chuỗi để trả ERR_INVALID_ID (thay vì lỗi parse JSON chung) khi không phải UUID.
type FriendTargetRequest struct {
	UserID string `json:"user_id"`
}

type FriendItemDTO struct {
	FriendshipID uuid.UUID     `json:"friendship_id"`
	User         FriendUserDTO `json:"user"`
	Since        time.Time     `json:"since"`
}

type FriendListResponse struct {
	Friends    []FriendItemDTO `json:"friends"`
	TotalCount int64           `json:"total_count"`
	Page       int             `json:"page"`
	Limit      int             `json:"limit"`
}

type FriendSummaryResponse struct {
	FriendsCount     int64 `json:"friends_count"`
	IncomingRequests int64 `json:"incoming_requests"`
	OutgoingRequests int64 `json:"outgoing_requests"`
}

type FriendRequestItemDTO struct {
	ID        uuid.UUID     `json:"id"`
	Direction string        `json:"direction"`
	User      FriendUserDTO `json:"user"`
	CreatedAt time.Time     `json:"created_at"`
}

type FriendRequestListResponse struct {
	Requests   []FriendRequestItemDTO `json:"requests"`
	TotalCount int64                  `json:"total_count"`
	Page       int                    `json:"page"`
	Limit      int                    `json:"limit"`
}

// FriendRequestResultDTO — kết quả gửi/chấp nhận: {id, status, user}.
type FriendRequestResultDTO struct {
	ID     uuid.UUID     `json:"id"`
	Status string        `json:"status"`
	User   FriendUserDTO `json:"user"`
}

// FriendDeclineResultDTO — kết quả từ chối: {id, status:"DECLINED"}.
type FriendDeclineResultDTO struct {
	ID     uuid.UUID `json:"id"`
	Status string    `json:"status"`
}

// FriendSearchUserDTO — FriendUser + relationship (nhúng nên JSON phẳng như contract). RequestID chỉ có khi
// relationship là PENDING_OUT/PENDING_IN: web thu hồi/chấp nhận ngay từ kết quả tìm kiếm, không phải gọi
// thêm /friends/relationship/:userId.
type FriendSearchUserDTO struct {
	FriendUserDTO
	Relationship string     `json:"relationship"`
	RequestID    *uuid.UUID `json:"request_id,omitempty"`
}

type FriendSearchResponse struct {
	Users []FriendSearchUserDTO `json:"users"`
}

type FriendRelationshipResponse struct {
	Status    string     `json:"status"`
	RequestID *uuid.UUID `json:"request_id,omitempty"`
}

type FriendBlockItemDTO struct {
	User      FriendUserDTO `json:"user"`
	CreatedAt time.Time     `json:"created_at"`
}

type FriendBlockListResponse struct {
	Blocks     []FriendBlockItemDTO `json:"blocks"`
	TotalCount int64                `json:"total_count"`
	Page       int                  `json:"page"`
	Limit      int                  `json:"limit"`
}
