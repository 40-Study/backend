package dto

import (
	"time"

	"github.com/google/uuid"
)

// ============================================================================
// REVIEW
// ============================================================================

type CreateReviewDTO struct {
	Rating  int    `json:"rating" validate:"required,min=1,max=5"`
	Comment string `json:"comment,omitempty"`
}

type UpdateReviewDTO struct {
	Rating  *int    `json:"rating" validate:"omitempty,min=1,max=5"`
	Comment *string `json:"comment"`
}

type ReviewResponseDTO struct {
	ID           uuid.UUID            `json:"id"`
	UserID       uuid.UUID            `json:"user_id"`
	UserName     string               `json:"user_name"`
	AvatarURL    *string              `json:"avatar_url,omitempty"`
	CourseID     uuid.UUID            `json:"course_id"`
	Rating       int                  `json:"rating"`
	Comment      *string              `json:"comment,omitempty"`
	CreatedAt    time.Time            `json:"created_at"`
	UpdatedAt    time.Time            `json:"updated_at"`
	HelpfulCount int                  `json:"helpful_count"`
	UserReaction *string              `json:"user_reaction,omitempty"`
}

type ReviewListDTO struct {
	Data         []ReviewResponseDTO `json:"data"`
	Total        int64               `json:"total"`
	Page         int                 `json:"page"`
	PageSize     int                 `json:"page_size"`
	AverageRating float64            `json:"average_rating"`
	RatingCounts  map[int]int64      `json:"rating_counts"`
}

// ============================================================================
// REVIEW REACTION
// ============================================================================

type CreateReviewReactionDTO struct {
	ReactionType string `json:"reaction_type" validate:"required,oneof=helpful not_helpful"`
}
