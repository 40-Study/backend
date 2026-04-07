package dto

import (
	"time"

	"github.com/google/uuid"
)

type NotificationResponseDTO struct {
	ID               uuid.UUID  `json:"id"`
	Title            string     `json:"title"`
	Content          string     `json:"content"`
	NotificationType string     `json:"notification_type"`
	ReferenceType    *string    `json:"reference_type,omitempty"`
	ReferenceID      *uuid.UUID `json:"reference_id,omitempty"`
	IsRead           bool       `json:"is_read"`
	ReadAt           *time.Time `json:"read_at,omitempty"`
	CreatedAt        time.Time  `json:"created_at"`
}

type NotificationListResponseDTO struct {
	Notifications []NotificationResponseDTO `json:"notifications"`
	Total         int64                     `json:"total"`
	UnreadCount   int64                     `json:"unread_count"`
	Page          int                       `json:"page"`
	PageSize      int                       `json:"page_size"`
}

type CreateNotificationDTO struct {
	Title            string      `json:"title" validate:"required"`
	Content          string      `json:"content" validate:"required"`
	NotificationType string      `json:"notification_type" validate:"required"`
	ReferenceType    *string     `json:"reference_type,omitempty"`
	ReferenceID      *uuid.UUID  `json:"reference_id,omitempty"`
	UserIDs          []uuid.UUID `json:"user_ids" validate:"required,min=1"`
}

type NotificationSettingsResponseDTO struct {
	EmailCourseUpdates   bool `json:"email_course_updates"`
	EmailPromotions      bool `json:"email_promotions"`
	EmailRecommendations bool `json:"email_recommendations"`
	PushCourseUpdates    bool `json:"push_course_updates"`
	PushQuizReminders    bool `json:"push_quiz_reminders"`
	PushPromotions       bool `json:"push_promotions"`
	PushAchievements     bool `json:"push_achievements"`
	PushStreakReminders  bool `json:"push_streak_reminders"`
}

type UpdateNotificationSettingsDTO struct {
	EmailCourseUpdates   *bool `json:"email_course_updates,omitempty"`
	EmailPromotions      *bool `json:"email_promotions,omitempty"`
	EmailRecommendations *bool `json:"email_recommendations,omitempty"`
	PushCourseUpdates    *bool `json:"push_course_updates,omitempty"`
	PushQuizReminders    *bool `json:"push_quiz_reminders,omitempty"`
	PushPromotions       *bool `json:"push_promotions,omitempty"`
	PushAchievements     *bool `json:"push_achievements,omitempty"`
	PushStreakReminders  *bool `json:"push_streak_reminders,omitempty"`
}
