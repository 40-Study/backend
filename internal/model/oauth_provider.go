package model

import (
	"time"

	"github.com/google/uuid"
)

type UserOAuthProvider struct {
	ID             uuid.UUID `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	CreatedAt      time.Time `json:"created_at"`
	UserID         uuid.UUID `gorm:"type:uuid;not null;index" json:"user_id"`
	Provider       string    `gorm:"type:varchar(50);not null;check:provider IN ('google', 'facebook', 'github')" json:"provider"`
	ProviderUserID string    `gorm:"type:varchar(255);not null;uniqueIndex:idx_oauth_provider_user,priority:2" json:"provider_user_id"`
	Email          *string   `gorm:"type:varchar(255)" json:"email,omitempty"`
	AvatarURL      *string   `gorm:"type:varchar(500)" json:"avatar_url,omitempty"`

	User User `gorm:"foreignKey:UserID;constraint:OnDelete:CASCADE" json:"-"`
}

func (UserOAuthProvider) TableName() string {
	return "user_oauth_providers"
}
