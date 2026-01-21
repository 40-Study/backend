package model

import (
	"time"

	"github.com/google/uuid"
)

type VerificationCode struct {
	ID        uuid.UUID `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	CreatedAt time.Time `json:"created_at"`
	UserID    uuid.UUID `gorm:"type:uuid;not null;index" json:"user_id"`
	Code      string    `gorm:"type:varchar(10);not null" json:"-"`
	Type      string    `gorm:"type:varchar(20);not null;check:type IN ('email', 'phone', 'password_reset')" json:"type"`
	ExpiresAt time.Time `gorm:"not null" json:"expires_at"`
	IsUsed    bool      `gorm:"default:false" json:"is_used"`

	// Relationships
	User User `gorm:"foreignKey:UserID;constraint:OnDelete:CASCADE" json:"-"`
}

func (VerificationCode) TableName() string {
	return "verification_codes"
}
