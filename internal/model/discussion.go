package model

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type Discussion struct {
	gorm.Model
	ID                 uuid.UUID  `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	UserID             uuid.UUID  `gorm:"type:uuid;not null;index" json:"user_id"`
	LessonID           uuid.UUID  `gorm:"type:uuid;not null;index" json:"lesson_id"`
	ParentID           *uuid.UUID `gorm:"type:uuid;index" json:"parent_id,omitempty"`
	Content            string     `gorm:"type:text;not null" json:"content"`
	VideoTimestampSecs *int       `gorm:"column:video_timestamp_seconds" json:"video_timestamp_seconds,omitempty"`
	UpvoteCount        int        `gorm:"default:0" json:"upvote_count"`
	IsPinned           bool       `gorm:"default:false" json:"is_pinned"`
	IsInstructorAnswer bool       `gorm:"default:false" json:"is_instructor_answer"`
	IsHidden           bool       `gorm:"default:false" json:"is_hidden"`

	// Relationships
	User    User             `gorm:"foreignKey:UserID;constraint:OnDelete:CASCADE" json:"-"`
	Lesson  Lesson           `gorm:"foreignKey:LessonID;constraint:OnDelete:CASCADE" json:"-"`
	Parent  *Discussion      `gorm:"foreignKey:ParentID;constraint:OnDelete:CASCADE" json:"-"`
	Replies []Discussion     `gorm:"foreignKey:ParentID" json:"-"`
	Votes   []DiscussionVote `gorm:"foreignKey:DiscussionID;constraint:OnDelete:CASCADE" json:"-"`
}

func (Discussion) TableName() string {
	return "discussions"
}

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
