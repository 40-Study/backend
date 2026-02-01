package model

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type User struct {
	ID           uuid.UUID      `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	CreatedAt    time.Time      `json:"created_at"`
	UpdatedAt    time.Time      `json:"updated_at"`
	DeletedAt    gorm.DeletedAt `gorm:"index" json:"deleted_at,omitempty"`
	Email string `gorm:"type:varchar(255);uniqueIndex:idx_users_email;not null"`
	PasswordHash string         `gorm:"type:varchar(255);not null;column:password_hash" json:"-"`
	UserName     string         `gorm:"type:varchar(100);not null;column:user_name" json:"user_name"`
	FullName     *string        `gorm:"type:varchar(255);column:full_name" json:"full_name,omitempty"`
	AvatarURL    *string        `gorm:"type:varchar(500);column:avatar_url" json:"avatar_url,omitempty"`
	Phone        *string        `gorm:"type:varchar(20)" json:"phone,omitempty"`
	ParentPhone  *string        `gorm:"type:varchar(20);column:parent_phone" json:"parent_phone,omitempty"`
	ParentEmail  *string        `gorm:"type:varchar(255);column:parent_email" json:"parent_email,omitempty"`
	DateOfBirth  *time.Time     `gorm:"type:date;column:date_of_birth" json:"date_of_birth,omitempty"`
	Bio          *string        `gorm:"type:text" json:"bio,omitempty"`
	IsVerified   bool           `gorm:"default:false;column:is_verified" json:"is_verified"`
	IsActive     bool           `gorm:"default:true;index;column:is_active" json:"is_active"`
	LastLoginAt  *time.Time     `gorm:"column:last_login_at" json:"last_login_at,omitempty"`

	// Many-to-many relationship with roles via user_roles junction table
	UserRoles []UserRole `gorm:"foreignKey:UserID;constraint:OnDelete:CASCADE" json:"-"`

	// Relationships
	VerificationCodes []VerificationCode  `gorm:"foreignKey:UserID;constraint:OnDelete:CASCADE" json:"-"`
	OAuthProviders    []UserOAuthProvider `gorm:"foreignKey:UserID;constraint:OnDelete:CASCADE" json:"-"`
	Courses           []Course            `gorm:"foreignKey:InstructorID" json:"-"`
	Enrollments       []Enrollment        `gorm:"foreignKey:UserID;constraint:OnDelete:CASCADE" json:"-"`
	Orders            []Order             `gorm:"foreignKey:UserID" json:"-"`
	Points            *UserPoint          `gorm:"foreignKey:UserID" json:"-"`
	Streak            *UserStreak         `gorm:"foreignKey:UserID" json:"-"`
	Achievements      []UserAchievement   `gorm:"foreignKey:UserID" json:"-"`
	Preference        *UserPreference     `gorm:"foreignKey:UserID" json:"-"`
}

func (User) TableName() string {
	return "users"
}
