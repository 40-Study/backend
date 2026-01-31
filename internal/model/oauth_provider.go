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
	ProviderUserID string    `gorm:"type:varchar(255);not null" json:"provider_user_id"`
	AccessToken    *string   `gorm:"type:text" json:"-"`
	RefreshToken   *string   `gorm:"type:text" json:"-"`

	// Relationships
	User User `gorm:"foreignKey:UserID;constraint:OnDelete:CASCADE" json:"-"`
}

func (UserOAuthProvider) TableName() string {
	return "user_oauth_providers"
}

// Composite unique index
func (UserOAuthProvider) BeforeMigrate() {
	// GORM will create unique index on (provider, provider_user_id)
}
