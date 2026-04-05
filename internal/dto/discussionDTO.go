package dto

import (
	"time"

	"github.com/google/uuid"
)

// CreateForumPostDTO - create a new forum post
type CreateForumPostDTO struct {
	Title    string `json:"title" validate:"required,min=3,max=255"`
	Content  string `json:"content" validate:"required"`
	Category string `json:"category" validate:"required,oneof=programming design learning-tips project"`
}

// CreateForumCommentDTO - reply to a forum post
type CreateForumCommentDTO struct {
	Content  string  `json:"content" validate:"required"`
	ParentID *string `json:"parent_id,omitempty"`
}

// VoteForumDTO - vote on a discussion
type VoteForumDTO struct {
	VoteType string `json:"vote_type" validate:"required,oneof=upvote downvote"`
}

// ForumPostResponseDTO - single forum post summary
type ForumPostResponseDTO struct {
	ID          uuid.UUID `json:"id"`
	Slug        string    `json:"slug"`
	Title       string    `json:"title"`
	Content     string    `json:"content"`
	Category    string    `json:"category"`
	AuthorID    uuid.UUID `json:"author_id"`
	AuthorName  string    `json:"author_name"`
	AvatarURL   *string   `json:"avatar_url,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
	UpvoteCount int       `json:"upvote_count"`
	ReplyCount  int       `json:"reply_count"`
	UserVote    *string   `json:"user_vote,omitempty"`
}

// ForumCommentResponseDTO - single comment/reply
type ForumCommentResponseDTO struct {
	ID          uuid.UUID                 `json:"id"`
	Content     string                    `json:"content"`
	AuthorID    uuid.UUID                 `json:"author_id"`
	AuthorName  string                    `json:"author_name"`
	AvatarURL   *string                   `json:"avatar_url,omitempty"`
	CreatedAt   time.Time                 `json:"created_at"`
	UpvoteCount int                       `json:"upvote_count"`
	UserVote    *string                   `json:"user_vote,omitempty"`
	Replies     []ForumCommentResponseDTO `json:"replies"`
}

// ForumPostListResponseDTO - paginated post list
type ForumPostListResponseDTO struct {
	Posts    []ForumPostResponseDTO `json:"posts"`
	Total    int64                  `json:"total"`
	Page     int                    `json:"page"`
	PageSize int                    `json:"page_size"`
}

// ForumPostDetailResponseDTO - post with comments
type ForumPostDetailResponseDTO struct {
	ForumPostResponseDTO
	Comments []ForumCommentResponseDTO `json:"comments"`
}
