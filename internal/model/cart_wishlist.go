package model

import (
	"time"

	"github.com/google/uuid"
)

type Wishlist struct {
	ID        uuid.UUID `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	CreatedAt time.Time `json:"created_at"`
	UserID    uuid.UUID `gorm:"type:uuid;not null;index;uniqueIndex:idx_user_course_wishlist" json:"user_id"`
	CourseID  uuid.UUID `gorm:"type:uuid;not null;index;uniqueIndex:idx_user_course_wishlist" json:"course_id"`

	// Relationships
	User   User   `gorm:"foreignKey:UserID;constraint:OnDelete:CASCADE" json:"-"`
	Course Course `gorm:"foreignKey:CourseID;constraint:OnDelete:CASCADE" json:"-"`
}

func (Wishlist) TableName() string {
	return "wishlists"
}

type CartItem struct {
	ID        uuid.UUID `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	CreatedAt time.Time `json:"created_at"`
	UserID    uuid.UUID `gorm:"type:uuid;not null;index;uniqueIndex:idx_user_course_cart" json:"user_id"`
	CourseID  uuid.UUID `gorm:"type:uuid;not null;index;uniqueIndex:idx_user_course_cart" json:"course_id"`

	// Relationships
	User   User   `gorm:"foreignKey:UserID;constraint:OnDelete:CASCADE" json:"-"`
	Course Course `gorm:"foreignKey:CourseID;constraint:OnDelete:CASCADE" json:"-"`
}

func (CartItem) TableName() string {
	return "cart_items"
}
