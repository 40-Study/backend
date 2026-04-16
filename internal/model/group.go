package model

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/datatypes"
)

// ============================================================================
// GROUP ENUMS
// ============================================================================

type GroupType string

const (
	GroupTypeStudy  GroupType = "STUDY_GROUP"
	GroupTypeClass  GroupType = "CLASS_GROUP"
	GroupTypeCourse GroupType = "COURSE_GROUP"
	GroupTypeCustom GroupType = "CUSTOM"
)

type GroupPrivacy string

const (
	GroupPrivacyPublic  GroupPrivacy = "PUBLIC"
	GroupPrivacyPrivate GroupPrivacy = "PRIVATE"
	GroupPrivacySecret  GroupPrivacy = "SECRET"
)

type GroupMemberRole string

const (
	GroupRoleOwner     GroupMemberRole = "OWNER"
	GroupRoleAdmin     GroupMemberRole = "ADMIN"
	GroupRoleModerator GroupMemberRole = "MODERATOR"
	GroupRoleMember    GroupMemberRole = "MEMBER"
)

type GroupMemberStatus string

const (
	GroupMemberActive  GroupMemberStatus = "ACTIVE"
	GroupMemberPending GroupMemberStatus = "PENDING"
	GroupMemberInvited GroupMemberStatus = "INVITED"
	GroupMemberBanned  GroupMemberStatus = "BANNED"
	GroupMemberLeft    GroupMemberStatus = "LEFT"
)

type JoinRequestStatus string

const (
	JoinRequestPending  JoinRequestStatus = "PENDING"
	JoinRequestApproved JoinRequestStatus = "APPROVED"
	JoinRequestRejected JoinRequestStatus = "REJECTED"
)

// ============================================================================
// GROUP
// ============================================================================

// Group represents a user group for study/class/course
type Group struct {
	BaseModel
	OrganizationID *uuid.UUID     `gorm:"type:uuid;index" json:"organization_id,omitempty"`
	Name           string         `gorm:"type:varchar(200);not null" json:"name"`
	Slug           string         `gorm:"type:varchar(200);not null;uniqueIndex" json:"slug"`
	Description    *string        `gorm:"type:text" json:"description,omitempty"`
	AvatarURL      *string        `gorm:"type:varchar(500)" json:"avatar_url,omitempty"`
	CoverURL       *string        `gorm:"type:varchar(500)" json:"cover_url,omitempty"`
	Type           GroupType      `gorm:"type:varchar(20);default:'CUSTOM'" json:"type"`
	Privacy        GroupPrivacy   `gorm:"type:varchar(20);default:'PRIVATE'" json:"privacy"`
	MaxMembers     int            `gorm:"default:100" json:"max_members"`
	MemberCount    int            `gorm:"default:0" json:"member_count"`
	ReferenceType  *string        `gorm:"type:varchar(50)" json:"reference_type,omitempty"`
	ReferenceID    *uuid.UUID     `gorm:"type:uuid" json:"reference_id,omitempty"`
	Settings       datatypes.JSON `gorm:"type:jsonb;default:'{}'" json:"settings"`
	CreatedBy      uuid.UUID      `gorm:"type:uuid;not null" json:"created_by"`

	// Relationships
	Organization *Organization   `gorm:"foreignKey:OrganizationID" json:"organization,omitempty"`
	Creator      User            `gorm:"foreignKey:CreatedBy" json:"-"`
	Members      []GroupMember   `gorm:"foreignKey:GroupID" json:"members,omitempty"`
	Conversation *Conversation   `gorm:"foreignKey:GroupID" json:"conversation,omitempty"`
}

func (Group) TableName() string {
	return "groups"
}

// ============================================================================
// GROUP MEMBER
// ============================================================================

// GroupMember represents membership in a group
type GroupMember struct {
	BaseModel
	GroupID             uuid.UUID         `gorm:"type:uuid;not null;uniqueIndex:idx_group_member_unique,priority:1" json:"group_id"`
	UserID              uuid.UUID         `gorm:"type:uuid;not null;uniqueIndex:idx_group_member_unique,priority:2" json:"user_id"`
	Role                GroupMemberRole   `gorm:"type:varchar(20);default:'MEMBER'" json:"role"`
	Status              GroupMemberStatus `gorm:"type:varchar(20);default:'ACTIVE'" json:"status"`
	Nickname            *string           `gorm:"type:varchar(100)" json:"nickname,omitempty"`
	InvitedBy           *uuid.UUID        `gorm:"type:uuid" json:"invited_by,omitempty"`
	JoinedAt            *time.Time        `json:"joined_at,omitempty"`
	LastReadAt          *time.Time        `json:"last_read_at,omitempty"`
	NotificationEnabled bool              `gorm:"default:true" json:"notification_enabled"`

	// Relationships
	Group   Group `gorm:"foreignKey:GroupID;constraint:OnDelete:CASCADE" json:"-"`
	User    User  `gorm:"foreignKey:UserID;constraint:OnDelete:CASCADE" json:"-"`
	Inviter *User `gorm:"foreignKey:InvitedBy" json:"inviter,omitempty"`
}

func (GroupMember) TableName() string {
	return "group_members"
}

// ============================================================================
// GROUP JOIN REQUEST
// ============================================================================

// GroupJoinRequest represents a request to join a private group
type GroupJoinRequest struct {
	ID              uuid.UUID         `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	CreatedAt       time.Time         `json:"created_at"`
	UpdatedAt       time.Time         `json:"updated_at"`
	GroupID         uuid.UUID         `gorm:"type:uuid;not null;uniqueIndex:idx_group_join_request_unique,priority:1" json:"group_id"`
	UserID          uuid.UUID         `gorm:"type:uuid;not null;uniqueIndex:idx_group_join_request_unique,priority:2" json:"user_id"`
	Message         *string           `gorm:"type:text" json:"message,omitempty"`
	Status          JoinRequestStatus `gorm:"type:varchar(20);default:'PENDING'" json:"status"`
	ReviewedBy      *uuid.UUID        `gorm:"type:uuid" json:"reviewed_by,omitempty"`
	ReviewedAt      *time.Time        `json:"reviewed_at,omitempty"`
	RejectionReason *string           `gorm:"type:text" json:"rejection_reason,omitempty"`

	// Relationships
	Group    Group `gorm:"foreignKey:GroupID;constraint:OnDelete:CASCADE" json:"-"`
	User     User  `gorm:"foreignKey:UserID;constraint:OnDelete:CASCADE" json:"-"`
	Reviewer *User `gorm:"foreignKey:ReviewedBy" json:"reviewer,omitempty"`
}

func (GroupJoinRequest) TableName() string {
	return "group_join_requests"
}
