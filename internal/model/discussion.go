package model

import (
	"time"

	"github.com/google/uuid"
)

type Discussion struct {
	BaseModel
	UserID             uuid.UUID  `gorm:"type:uuid;not null;index" json:"user_id"`
	LessonID           *uuid.UUID `gorm:"type:uuid;index" json:"lesson_id,omitempty"`
	ParentID           *uuid.UUID `gorm:"type:uuid;index" json:"parent_id,omitempty"`
	Content            string     `gorm:"type:text;not null" json:"content"`
	Title              *string    `gorm:"type:varchar(255)" json:"title,omitempty"`
	Category           *string    `gorm:"type:varchar(100)" json:"category,omitempty"`
	Slug               *string    `gorm:"type:varchar(300);uniqueIndex" json:"slug,omitempty"`
	VideoTimestampSecs *int       `gorm:"column:video_timestamp_seconds" json:"video_timestamp_seconds,omitempty"`
	UpvoteCount        int        `gorm:"default:0" json:"upvote_count"`
	ReplyCount         int        `gorm:"default:0" json:"reply_count"`
	IsPinned           bool       `gorm:"default:false" json:"is_pinned"`
	IsInstructorAnswer bool       `gorm:"default:false" json:"is_instructor_answer"`
	IsHidden           bool       `gorm:"default:false" json:"is_hidden"`

	// Relationships
	User    User             `gorm:"foreignKey:UserID;constraint:OnDelete:CASCADE" json:"-"`
	Lesson  *Lesson          `gorm:"foreignKey:LessonID;constraint:OnDelete:CASCADE" json:"-"`
	Parent  *Discussion      `gorm:"foreignKey:ParentID;constraint:OnDelete:CASCADE" json:"-"`
	Replies []Discussion     `gorm:"foreignKey:ParentID" json:"-"`
	Votes   []DiscussionVote `gorm:"foreignKey:DiscussionID;constraint:OnDelete:CASCADE" json:"-"`
}

func (Discussion) TableName() string {
	return "discussions"
}

// DiscussionLiveReplyCountSQL: biểu thức vô hướng đếm số trả lời CÒN SỐNG (chưa xoá mềm) của dòng `d` ở câu
// lệnh bao ngoài, ở mọi độ sâu (AddComment cộng vào BÀI GỐC kể cả với trả lời lồng). Đi xuống từng nhánh nên trả
// lời nằm dưới một trả lời đã xoá không được tính. SSOT cho mọi nơi tính lại discussions.reply_count (xoá trả
// lời, bước sửa dữ liệu một lần, seed) — đừng chép lại truy vấn này.
const DiscussionLiveReplyCountSQL = `(WITH RECURSIVE live_replies AS (
		SELECT c.id FROM discussions c WHERE c.parent_id = d.id AND c.deleted_at IS NULL
		UNION
		SELECT c.id FROM discussions c JOIN live_replies lr ON c.parent_id = lr.id WHERE c.deleted_at IS NULL
	) SELECT COUNT(*) FROM live_replies)`

type DiscussionVote struct {
	ID           uuid.UUID `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	CreatedAt    time.Time `json:"created_at"`
	UserID       uuid.UUID `gorm:"type:uuid;not null;index;uniqueIndex:idx_user_discussion_vote" json:"user_id"`
	DiscussionID uuid.UUID `gorm:"type:uuid;not null;index;uniqueIndex:idx_user_discussion_vote" json:"discussion_id"`
	VoteType     string    `gorm:"type:varchar(10);not null;check:vote_type IN ('upvote', 'downvote')" json:"vote_type"`

	// Relationships
	User       User       `gorm:"foreignKey:UserID;constraint:OnDelete:CASCADE" json:"-"`
	Discussion Discussion `gorm:"foreignKey:DiscussionID;constraint:OnDelete:CASCADE" json:"-"`
}

func (DiscussionVote) TableName() string {
	return "discussion_votes"
}
