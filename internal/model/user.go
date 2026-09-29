package model

import (
	"time"

	"github.com/google/uuid"
)

type User struct {
	BaseModel
	// Email: json:"-" (S2) — model.User được serialize thẳng ở nhiều quan hệ (Sender, Inviter, Student,
	// Parent...) nên nếu không chặn ở đây thì email lộ ra dưới khoá "Email" cho mọi người xem. Chỗ nào
	// thật sự cần trả email (tài khoản của chính mình, màn admin) dùng DTO riêng có trường email.
	Email        string     `gorm:"type:varchar(255);uniqueIndex:idx_users_email;not null" json:"-"`
	PasswordHash string     `gorm:"type:varchar(255);not null;column:password_hash" json:"-"`
	UserName     string     `gorm:"type:varchar(100);not null;column:user_name" json:"user_name"`
	FullName     *string    `gorm:"type:varchar(255);column:full_name" json:"full_name,omitempty"`
	AvatarURL    *string    `gorm:"type:varchar(500);column:avatar_url" json:"avatar_url,omitempty"`
	Phone        *string    `gorm:"type:varchar(20)" json:"phone,omitempty"`
	ParentPhone  *string    `gorm:"type:varchar(20);column:parent_phone" json:"parent_phone,omitempty"`
	ParentEmail  *string    `gorm:"type:varchar(255);column:parent_email" json:"parent_email,omitempty"`
	DateOfBirth  *time.Time `gorm:"type:date;column:date_of_birth" json:"date_of_birth,omitempty"`
	Bio          *string    `gorm:"type:text" json:"bio,omitempty"`
	IsVerified        bool       `gorm:"default:false;column:is_verified" json:"is_verified"`
	IsActive          bool       `gorm:"default:true;index;column:is_active" json:"is_active"`
	LastLoginAt       *time.Time `gorm:"column:last_login_at" json:"last_login_at,omitempty"`
	PasswordChangedAt *time.Time `gorm:"column:password_changed_at" json:"password_changed_at,omitempty"`
	Timezone          *string    `gorm:"type:varchar(50);default:'Asia/Ho_Chi_Minh'" json:"timezone,omitempty"` // IANA timezone

	// Phase 1 quản lý người dùng (2026-09-28): audit khoá/mở tài khoản — không derive được từ
	// đâu khác (ai khoá, khi nào, lý do) nên không vi phạm "No Derived Fields".
	LockedReason *string    `gorm:"type:text;column:locked_reason" json:"locked_reason,omitempty"`
	LockedAt     *time.Time `gorm:"column:locked_at" json:"locked_at,omitempty"`
	LockedBy     *uuid.UUID `gorm:"type:uuid;column:locked_by" json:"locked_by,omitempty"`

	// Many-to-many relationship with roles
	UserOrganizationRoles []UserOrganizationRole `gorm:"foreignKey:UserID;constraint:OnDelete:CASCADE" json:"-"`
	UserSystemRoles       []UserSystemRole       `gorm:"foreignKey:UserID;constraint:OnDelete:CASCADE" json:"-"`

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
	TeacherProfile    *TeacherProfile     `gorm:"foreignKey:UserID" json:"-"`
}

func (User) TableName() string {
	return "users"
}
