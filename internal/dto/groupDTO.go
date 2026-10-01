package dto

import (
	"time"

	"github.com/google/uuid"
)

// ============================================================================
// GROUP DTOs
// ============================================================================

type CreateGroupRequest struct {
	Name           string  `json:"name" validate:"required,min=2,max=200"`
	Description    *string `json:"description,omitempty"`
	Type           string  `json:"type" validate:"omitempty,oneof=STUDY_GROUP CLASS_GROUP COURSE_GROUP CUSTOM"`
	Privacy        string  `json:"privacy" validate:"omitempty,oneof=PUBLIC PRIVATE SECRET"`
	MaxMembers     int     `json:"max_members" validate:"omitempty,min=2,max=1000"`
	OrganizationID *string `json:"organization_id,omitempty"`
}

type UpdateGroupRequest struct {
	Name        *string `json:"name,omitempty" validate:"omitempty,min=2,max=200"`
	Description *string `json:"description,omitempty"`
	Privacy     *string `json:"privacy,omitempty" validate:"omitempty,oneof=PUBLIC PRIVATE SECRET"`
	MaxMembers  *int    `json:"max_members,omitempty" validate:"omitempty,min=2,max=1000"`
}

type GroupResponse struct {
	ID             uuid.UUID          `json:"id"`
	Name           string             `json:"name"`
	Slug           string             `json:"slug"`
	Description    *string            `json:"description,omitempty"`
	AvatarURL      *string            `json:"avatar_url,omitempty"`
	CoverURL       *string            `json:"cover_url,omitempty"`
	Type           string             `json:"type"`
	Privacy        string             `json:"privacy"`
	MaxMembers     int                `json:"max_members"`
	MemberCount    int                `json:"member_count"`
	CreatedBy      uuid.UUID          `json:"created_by"`
	OrganizationID *uuid.UUID         `json:"organization_id,omitempty"`
	MyRole         *string            `json:"my_role,omitempty"`
	CreatedAt      time.Time          `json:"created_at"`
	UpdatedAt      time.Time          `json:"updated_at"`
	Conversation   *ConversationBrief `json:"conversation,omitempty"`
	// MyJoinRequest chỉ có ở GET /groups/:slug, khi người xem đã đăng nhập, chưa là thành viên và đang có
	// yêu cầu xin vào ở trạng thái chờ (contract-api.md §2) — để web phân biệt "Xin tham gia" / "Đã gửi yêu cầu".
	MyJoinRequest *MyJoinRequestBrief `json:"my_join_request,omitempty"`
}

// MyJoinRequestBrief — yêu cầu xin vào nhóm đang chờ của chính người xem.
type MyJoinRequestBrief struct {
	ID     uuid.UUID `json:"id"`
	Status string    `json:"status"`
}

type GroupListResponse struct {
	Groups     []GroupResponse `json:"groups"`
	TotalCount int64           `json:"total_count"`
	Page       int             `json:"page"`
	Limit      int             `json:"limit"`
}

// ============================================================================
// GROUP MEMBER DTOs
// ============================================================================

type GroupMemberResponse struct {
	ID        uuid.UUID  `json:"id"`
	UserID    uuid.UUID  `json:"user_id"`
	UserName  string     `json:"user_name"`
	FullName  *string    `json:"full_name,omitempty"`
	AvatarURL *string    `json:"avatar_url,omitempty"`
	Role      string     `json:"role"`
	Status    string     `json:"status"`
	Nickname  *string    `json:"nickname,omitempty"`
	JoinedAt  *time.Time `json:"joined_at,omitempty"`
}

type GroupMemberListResponse struct {
	Members    []GroupMemberResponse `json:"members"`
	TotalCount int64                 `json:"total_count"`
	Page       int                   `json:"page"`
	Limit      int                   `json:"limit"`
}

type InviteMembersRequest struct {
	UserIDs []uuid.UUID `json:"user_ids" validate:"required,min=1,max=50"`
}

// Mã lỗi nhóm (contract-api.md §2). Dùng cho `code` của lỗi join/approve và `rejected[].code` của lời mời.
const (
	// GroupInviteNotAllowedCode - ma loi khi nguoi duoc moi khong co quan he hop le voi nguoi moi.
	GroupInviteNotAllowedCode = "GROUP_INVITE_NOT_ALLOWED"
	GroupMemberBannedCode     = "GROUP_MEMBER_BANNED" // chỉ trong rejected[] của lời mời
	GroupBannedCode           = "GROUP_BANNED"        // join/approve: người đó đang bị cấm
	GroupFullCode             = "GROUP_FULL"
	GroupAlreadyMemberCode    = "GROUP_ALREADY_MEMBER"
	GroupJoinRequestExists    = "GROUP_JOIN_REQUEST_EXISTS"
)

// InviteRejection - mot nguoi bi tu choi khi moi vao nhom.
type InviteRejection struct {
	UserID uuid.UUID `json:"user_id"`
	Code   string    `json:"code"`
}

// InviteMembersResult - ket qua moi thanh vien: ai da vao nhom, ai bi tu choi (kem ma loi).
type InviteMembersResult struct {
	Invited  []uuid.UUID       `json:"invited"`
	Rejected []InviteRejection `json:"rejected"`
}

type UpdateMemberRoleRequest struct {
	Role string `json:"role" validate:"required,oneof=ADMIN MODERATOR MEMBER"`
}

// ============================================================================
// JOIN REQUEST DTOs
// ============================================================================

type JoinGroupRequest struct {
	Message *string `json:"message,omitempty"`
}

type JoinRequestResponse struct {
	ID              uuid.UUID  `json:"id"`
	GroupID         uuid.UUID  `json:"group_id"`
	UserID          uuid.UUID  `json:"user_id"`
	UserName        string     `json:"user_name"`
	AvatarURL       *string    `json:"avatar_url,omitempty"`
	Message         *string    `json:"message,omitempty"`
	Status          string     `json:"status"`
	ReviewedBy      *uuid.UUID `json:"reviewed_by,omitempty"`
	ReviewedAt      *time.Time `json:"reviewed_at,omitempty"`
	RejectionReason *string    `json:"rejection_reason,omitempty"`
	CreatedAt       time.Time  `json:"created_at"`
}

type JoinRequestListResponse struct {
	Requests   []JoinRequestResponse `json:"requests"`
	TotalCount int64                 `json:"total_count"`
	Page       int                   `json:"page"`
	Limit      int                   `json:"limit"`
}

type RejectRequestBody struct {
	Reason *string `json:"reason,omitempty"`
}
