package model

import (
	"time"

	"github.com/google/uuid"
)

// Các loại thông báo. SSOT cho CHECK chk_notifications_notification_type: tag `check:` của
// Notification.NotificationType phải khớp slice này (TestNotificationTypeTagMatchesSSOT) và
// RunPostMigrations đồng bộ constraint trên DB cũ theo slice (buildCheckConstraintSQL, NOT VALID
// rồi VALIDATE nên rollback về bản cũ không làm backend chết lúc khởi động).
const (
	NotificationTypeFriendRequest  = "friend_request"
	NotificationTypeFriendAccepted = "friend_accepted"
	NotificationTypeGroupAdded     = "group_added"
)

var NotificationTypes = []string{
	"course_update", "new_lesson", "quiz_reminder", "certificate_earned", "payment_success",
	"payment_failed", "promotion", "system", "achievement", "streak", "point_earned",
	NotificationTypeFriendRequest, NotificationTypeFriendAccepted, NotificationTypeGroupAdded,
}

type Notification struct {
	ID               uuid.UUID  `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	CreatedAt        time.Time  `json:"created_at"`
	UserID           uuid.UUID  `gorm:"type:uuid;not null;index" json:"user_id"`
	Title            string     `gorm:"type:varchar(255);not null" json:"title"`
	Content          string     `gorm:"type:text;not null" json:"content"`
	NotificationType string     `gorm:"type:varchar(30);not null;check:notification_type IN ('course_update', 'new_lesson', 'quiz_reminder', 'certificate_earned', 'payment_success', 'payment_failed', 'promotion', 'system', 'achievement', 'streak', 'point_earned', 'friend_request', 'friend_accepted', 'group_added')" json:"notification_type"`
	ReferenceType    *string    `gorm:"type:varchar(30)" json:"reference_type,omitempty"`
	ReferenceID      *uuid.UUID `gorm:"type:uuid;index" json:"reference_id,omitempty"`
	IsRead           bool       `gorm:"default:false;index" json:"is_read"`
	ReadAt           *time.Time `json:"read_at,omitempty"`

	// Relationships
	User User `gorm:"foreignKey:UserID;constraint:OnDelete:CASCADE" json:"-"`
}

func (Notification) TableName() string {
	return "notifications"
}

type NotificationSettings struct {
	BaseModel
	UserID               uuid.UUID `gorm:"type:uuid;uniqueIndex;not null" json:"user_id"`
	EmailCourseUpdates   bool      `gorm:"default:true" json:"email_course_updates"`
	EmailPromotions      bool      `gorm:"default:true" json:"email_promotions"`
	EmailRecommendations bool      `gorm:"default:true" json:"email_recommendations"`
	PushCourseUpdates    bool      `gorm:"default:true" json:"push_course_updates"`
	PushQuizReminders    bool      `gorm:"default:true" json:"push_quiz_reminders"`
	PushPromotions       bool      `gorm:"default:false" json:"push_promotions"`
	PushAchievements     bool      `gorm:"default:true" json:"push_achievements"`
	PushStreakReminders  bool      `gorm:"default:true" json:"push_streak_reminders"`

	// Relationships
	User User `gorm:"foreignKey:UserID;constraint:OnDelete:CASCADE" json:"-"`
}

func (NotificationSettings) TableName() string {
	return "notification_settings"
}
