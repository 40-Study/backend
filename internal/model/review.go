package model

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type Review struct {
	ID        uuid.UUID      `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
	DeletedAt gorm.DeletedAt `gorm:"index" json:"-"`
	UserID    uuid.UUID      `gorm:"type:uuid;not null;index" json:"user_id"`
	CourseID  uuid.UUID      `gorm:"type:uuid;not null;index;uniqueIndex:idx_user_course_review" json:"course_id"`
	Rating    int            `gorm:"type:smallint;not null;check:rating >= 1 AND rating <= 5" json:"rating"`
	Comment   *string        `gorm:"type:text" json:"comment,omitempty"`

	// Relationships
	User      User             `gorm:"foreignKey:UserID;constraint:OnDelete:CASCADE" json:"-"`
	Course    Course           `gorm:"foreignKey:CourseID;constraint:OnDelete:CASCADE" json:"-"`
	Reactions []ReviewReaction `gorm:"foreignKey:ReviewID" json:"reactions,omitempty"`
}

func (Review) TableName() string {
	return "reviews"
}

type ReviewReaction struct {
	ID           uuid.UUID `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	CreatedAt    time.Time `json:"created_at"`
	ReviewID     uuid.UUID `gorm:"type:uuid;not null;index;uniqueIndex:idx_review_user_reaction" json:"review_id"`
	UserID       uuid.UUID `gorm:"type:uuid;not null;index;uniqueIndex:idx_review_user_reaction" json:"user_id"`
	ReactionType string    `gorm:"type:varchar(20);not null;check:reaction_type IN ('helpful', 'not_helpful')" json:"reaction_type"`

	// Relationships
	Review Review `gorm:"foreignKey:ReviewID;constraint:OnDelete:CASCADE" json:"-"`
	User   User   `gorm:"foreignKey:UserID;constraint:OnDelete:CASCADE" json:"-"`
}

func (ReviewReaction) TableName() string {
	return "review_reactions"
}
