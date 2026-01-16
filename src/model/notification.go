package model

import (
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"gorm.io/gorm"
)

// 27. Notification (Bảng thông báo)
type Notification struct {
	ID               uuid.UUID        `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	UserID           uuid.UUID        `gorm:"type:uuid;not null;index" json:"user_id"`
	NotificationType string           `gorm:"not null" json:"notification_type"`
	Title            string           `gorm:"not null" json:"title"`
	Message          string           `gorm:"not null" json:"message"`
	IconUrl          pgtype.Text      `json:"icon_url"`
	ActionUrl        pgtype.Text      `json:"action_url"`
	IsRead           pgtype.Bool      `gorm:"default:false" json:"is_read"`
	ReadAt           pgtype.Timestamp `json:"read_at"`
	Priority         pgtype.Text      `json:"priority"`
	IsVipOnly        pgtype.Bool      `gorm:"default:false" json:"is_vip_only"`
	RelatedType      pgtype.Text      `json:"related_type"`
	RelatedID        pgtype.Int4      `json:"related_id"`
	ExpiresAt        pgtype.Timestamp `json:"expires_at"`
	CreatedAt        time.Time        `gorm:"autoCreateTime" json:"created_at"`
	DeletedAt        gorm.DeletedAt   `gorm:"index" json:"deleted_at"`
}

func (Notification) TableName() string {
	return "notifications"
}

// 60. NotificationChannel (Bảng kênh thông báo)
type NotificationChannel struct {
	ID          uuid.UUID   `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	ChannelType string      `gorm:"unique;not null" json:"channel_type"`
	Provider    pgtype.Text `json:"provider"`
	IsActive    pgtype.Bool `gorm:"default:true" json:"is_active"`
	CreatedAt   time.Time   `gorm:"autoCreateTime" json:"created_at"`
}

func (NotificationChannel) TableName() string {
	return "notification_channels"
}

// 61. UserNotificationPreference (Bảng tùy chọn thông báo người dùng)
type UserNotificationPreference struct {
	ID               uuid.UUID   `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	UserID           uuid.UUID   `gorm:"type:uuid;not null;index" json:"user_id"`
	ChannelID        uuid.UUID   `gorm:"type:uuid;not null;index" json:"channel_id"`
	NotificationType string      `gorm:"not null" json:"notification_type"`
	IsEnabled        pgtype.Bool `gorm:"default:true" json:"is_enabled"`
	UpdatedAt        time.Time   `gorm:"autoUpdateTime" json:"updated_at"`
}

func (UserNotificationPreference) TableName() string {
	return "user_notification_preferences"
}

// 28. NotificationTemplate (Bảng mẫu thông báo)
type NotificationTemplate struct {
	ID               uuid.UUID              `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	TemplateKey      string                 `gorm:"unique;not null" json:"template_key"`
	TemplateName     string                 `gorm:"not null" json:"template_name"`
	NotificationType string                 `gorm:"not null" json:"notification_type"`
	TitleTemplate    string                 `gorm:"not null" json:"title_template"`
	MessageTemplate  string                 `gorm:"not null" json:"message_template"`
	EmailSubject     pgtype.Text            `json:"email_subject"`
	EmailBody        pgtype.Text            `json:"email_body"`
	SmsTemplate      pgtype.Text            `json:"sms_template"`
	PushTemplate     pgtype.Text            `json:"push_template"`
	Variables        map[string]interface{} `gorm:"type:jsonb;serializer:json" json:"variables"`
	IsActive         pgtype.Bool            `gorm:"default:true" json:"is_active"`
	CreatedAt        time.Time              `gorm:"autoCreateTime" json:"created_at"`
	UpdatedAt        time.Time              `gorm:"autoUpdateTime" json:"updated_at"`
	DeletedAt        gorm.DeletedAt         `gorm:"index" json:"deleted_at"`
}

func (NotificationTemplate) TableName() string {
	return "notification_templates"
}
