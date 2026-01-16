package model

import (
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/shopspring/decimal"
)

// 5. AnalyticsLog (Bảng nhật ký phân tích)
type AnalyticsLog struct {
	ID              uuid.UUID              `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	UserID          pgtype.Int4            `json:"user_id"`
	EventType       string                 `gorm:"not null" json:"event_type"`
	EventName       string                 `gorm:"not null" json:"event_name"`
	EventCategory   pgtype.Text            `json:"event_category"`
	PageUrl         pgtype.Text            `json:"page_url"`
	ReferrerUrl     pgtype.Text            `json:"referrer_url"`
	DeviceType      pgtype.Text            `json:"device_type"`
	Browser         pgtype.Text            `json:"browser"`
	Os              pgtype.Text            `json:"os"`
	IpAddress       pgtype.Text            `json:"ip_address"`
	UserAgent       pgtype.Text            `json:"user_agent"`
	SessionID       pgtype.Text            `json:"session_id"`
	DurationSeconds pgtype.Int4            `json:"duration_seconds"`
	Metadata        map[string]interface{} `gorm:"type:jsonb;serializer:json" json:"metadata"`
	CreatedAt       time.Time              `gorm:"autoCreateTime;index" json:"created_at"` // Partition key usually needs special handling, but GORM index is fine for now.
}

func (AnalyticsLog) TableName() string {
	return "analytics_logs"
}

// 4. AiRecommendation (Bảng khuyến nghị AI)
type AiRecommendation struct {
	ID                 uuid.UUID              `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	UserID             uuid.UUID              `gorm:"type:uuid;not null;index" json:"user_id"`
	RecommendationType string                 `gorm:"not null" json:"recommendation_type"`
	ItemID             uuid.UUID              `gorm:"type:uuid;not null" json:"item_id"`
	ItemType           pgtype.Text            `json:"item_type"`
	Score              *decimal.Decimal       `gorm:"type:numeric" json:"score"`
	Reason             pgtype.Text            `json:"reason"`
	Algorithm          pgtype.Text            `json:"algorithm"`
	ShownCount         pgtype.Int4            `gorm:"default:0" json:"shown_count"`
	ClickedCount       pgtype.Int4            `gorm:"default:0" json:"clicked_count"`
	Converted          pgtype.Bool            `gorm:"default:false" json:"converted"`
	Feedback           pgtype.Text            `json:"feedback"`
	Metadata           map[string]interface{} `gorm:"type:jsonb;serializer:json" json:"metadata"`
	ExpiresAt          pgtype.Timestamp       `json:"expires_at"`
	CreatedAt          time.Time              `gorm:"autoCreateTime" json:"created_at"`
	UpdatedAt          time.Time              `gorm:"autoUpdateTime" json:"updated_at"`
}

func (AiRecommendation) TableName() string {
	return "ai_recommendations"
}
