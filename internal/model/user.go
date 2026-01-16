package model

import (
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"gorm.io/gorm"
)

// 40. User (Bảng người dùng)
type User struct {
	ID           uuid.UUID        `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	Username     string           `gorm:"not null" json:"username"`
	Email        string           `gorm:"unique;not null" json:"email"`
	PasswordHash string           `gorm:"not null" json:"-"`
	Phone        pgtype.Text      `json:"phone"`
	AvatarUrl    pgtype.Text      `json:"avatar_url"`
	DateOfBirth  pgtype.Timestamp `json:"date_of_birth"`
	Gender       pgtype.Text      `gorm:"type:text;check:gender IN ('male', 'female', 'other')" json:"gender"`
	Address      pgtype.Text      `json:"address"`
	Status       pgtype.Text      `gorm:"default:'active'" json:"status"`
	CreatedAt    time.Time        `gorm:"autoCreateTime" json:"created_at"`
	UpdatedAt    time.Time        `gorm:"autoUpdateTime" json:"updated_at"`
	DeletedAt    gorm.DeletedAt   `gorm:"index" json:"deleted_at"`

	// Relations
	Preferences UserPreference `gorm:"foreignKey:UserID;constraint:OnUpdate:CASCADE,OnDelete:CASCADE;" json:"preferences"`
	Points      UserPoint      `gorm:"foreignKey:UserID;constraint:OnUpdate:CASCADE,OnDelete:CASCADE;" json:"points"`
	Roles       []Role         `gorm:"many2many:user_roles;" json:"roles"`
}

func (User) TableName() string {
	return "users"
}

// 41. UserPreference (Bảng sở thích người dùng)
type UserPreference struct {
	ID                     uuid.UUID              `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	UserID                 uuid.UUID              `gorm:"type:uuid;unique;not null" json:"user_id"`
	Theme                  pgtype.Text            `json:"theme"`
	Language               pgtype.Text            `json:"language"`
	NotificationEmail      pgtype.Bool            `gorm:"default:true" json:"notification_email"`
	NotificationPush       pgtype.Bool            `gorm:"default:true" json:"notification_push"`
	NotificationSms        pgtype.Bool            `gorm:"default:false" json:"notification_sms"`
	NewsletterSubscribed   pgtype.Bool            `gorm:"default:false" json:"newsletter_subscribed"`
	PreferredPaymentMethod pgtype.Text            `json:"preferred_payment_method"`
	PrivacyShowProfile     pgtype.Bool            `gorm:"default:true" json:"privacy_show_profile"`
	PrivacyShowActivity    pgtype.Bool            `gorm:"default:false" json:"privacy_show_activity"`
	Preferences            map[string]interface{} `gorm:"type:jsonb;serializer:json" json:"preferences"`
	CreatedAt              time.Time              `gorm:"autoCreateTime" json:"created_at"`
	UpdatedAt              time.Time              `gorm:"autoUpdateTime" json:"updated_at"`
}

func (UserPreference) TableName() string {
	return "user_preferences"
}

// 55. UserFavoriteFoodCategory
type UserFavoriteFoodCategory struct {
	ID             uuid.UUID `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	UserID         uuid.UUID `gorm:"type:uuid;not null;index" json:"user_id"`
	FoodCategoryID uuid.UUID `gorm:"type:uuid;not null;index" json:"food_category_id"`
	CreatedAt      time.Time `gorm:"autoCreateTime" json:"created_at"`
}

func (UserFavoriteFoodCategory) TableName() string {
	return "user_favorite_food_categories"
}

type UserFavoriteGame struct {
	ID        uuid.UUID `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	UserID    uuid.UUID `gorm:"type:uuid;not null;index" json:"user_id"`
	GameID    uuid.UUID `gorm:"type:uuid;not null;index" json:"game_id"`
	CreatedAt time.Time `gorm:"autoCreateTime" json:"created_at"`
}

func (UserFavoriteGame) TableName() string {
	return "user_favorite_games"
}

type UserPoint struct {
	ID     uuid.UUID `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	UserID uuid.UUID `gorm:"type:uuid;uniqueIndex;not null" json:"user_id"`

	TotalPoints     int32 `gorm:"not null;default:0" json:"total_points"`
	AvailablePoints int32 `gorm:"not null;default:0" json:"available_points"`
	LifetimeEarned  int32 `gorm:"not null;default:0" json:"lifetime_earned"`
	LifetimeSpent   int32 `gorm:"not null;default:0" json:"lifetime_spent"`

	TotalExpired  int32      `gorm:"not null;default:0" json:"total_expired"`
	LastExpiredAt *time.Time `json:"last_expired_at"`

	CreatedAt time.Time `gorm:"autoCreateTime" json:"created_at"`
	UpdatedAt time.Time `gorm:"autoUpdateTime" json:"updated_at"`
}

func (UserPoint) TableName() string {
	return "user_points"
}

// ===============================
// 2. UserPointLedger (Lô điểm + hạn sử dụng - FIFO)
// ===============================
type UserPointLedger struct {
	ID     uuid.UUID `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	UserID uuid.UUID `gorm:"type:uuid;not null;index" json:"user_id"`

	Points     int32 `gorm:"not null" json:"points"` // điểm gốc
	UsedPoints int32 `gorm:"not null;default:0" json:"used_points"`

	Source      string     `gorm:"type:text;not null" json:"source"` // daily_checkin, bonus, admin, purchase...
	ReferenceID *uuid.UUID `gorm:"type:uuid" json:"reference_id"`

	EarnedAt  time.Time  `gorm:"not null;index" json:"earned_at"`
	ExpiresAt *time.Time `gorm:"index" json:"expires_at"` // null = không hết hạn

	Status string `gorm:"type:text;default:'active';check:status IN ('active','expired','depleted')" json:"status"`

	CreatedAt time.Time `gorm:"autoCreateTime" json:"created_at"`
	UpdatedAt time.Time `gorm:"autoUpdateTime" json:"updated_at"`
}

func (UserPointLedger) TableName() string {
	return "user_point_ledgers"
}

type UserPointTransaction struct {
	ID     uuid.UUID `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	UserID uuid.UUID `gorm:"type:uuid;not null;index" json:"user_id"`

	LedgerID *uuid.UUID `gorm:"type:uuid;index" json:"ledger_id"`

	TransactionType string `gorm:"type:text;not null;check:transaction_type IN ('earn','spend','expire','refund')" json:"transaction_type"`

	Points        int32 `gorm:"not null" json:"points"` // (+) vừa nhận, (-) khi tiêu
	BalanceBefore int32 `gorm:"not null" json:"balance_before"`
	BalanceAfter  int32 `gorm:"not null" json:"balance_after"`

	Source        string     `gorm:"type:text" json:"source"`
	ReferenceID   *uuid.UUID `gorm:"type:uuid" json:"reference_id"`
	ReferenceType string     `gorm:"type:text" json:"reference_type"`
	Description   string     `gorm:"type:text" json:"description"`

	Metadata map[string]interface{} `gorm:"type:jsonb;serializer:json" json:"metadata"`

	CreatedAt time.Time `gorm:"autoCreateTime;index" json:"created_at"`
}

func (UserPointTransaction) TableName() string {
	return "user_point_transactions"
}

type UserDailyCheckin struct {
	ID          uuid.UUID `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	UserID      uuid.UUID `gorm:"type:uuid;not null;uniqueIndex:idx_user_checkin_date" json:"user_id"`
	CheckinDate time.Time `gorm:"type:date;not null;uniqueIndex:idx_user_checkin_date" json:"checkin_date"`

	PointsEarned int32 `gorm:"not null;default:10" json:"points_earned"`
	Streak       int32 `gorm:"not null;default:1" json:"streak"`
	BonusPoints  int32 `gorm:"not null;default:0" json:"bonus_points"`

	CreatedAt time.Time `gorm:"autoCreateTime" json:"created_at"`
}

func (UserDailyCheckin) TableName() string {
	return "user_daily_checkins"
}

type UserReward struct {
	ID       uuid.UUID `gorm:"type:uuid;primaryKey;default:gen_random_uuid()"`
	UserID   uuid.UUID `gorm:"type:uuid;not null;index"`
	RewardID uuid.UUID `gorm:"type:uuid;not null;index"`

	Code   string `gorm:"unique"`    // mã voucher thực tế
	Status string `gorm:"type:text"` // unused | used | expired

	ExpiresAt *time.Time
	CreatedAt time.Time
}

func (UserReward) TableName() string {
	return "user_reward"
}

type Reward struct {
	ID          uuid.UUID `gorm:"type:uuid;primaryKey;default:gen_random_uuid()"`
	Code        string    `gorm:"unique;not null"`
	Name        string    `gorm:"not null"`
	Description string    `gorm:"type:text"`

	CostPoints int32 `gorm:"not null"`
	Stock      int32 `gorm:"not null"`

	RewardType string `gorm:"type:text;not null"`
	Provider   string `gorm:"type:text"`
	StartAt    *time.Time
	EndAt      *time.Time
	IsActive   bool `gorm:"default:true"`

	Metadata map[string]interface{} `gorm:"type:jsonb;serializer:json"`

	CreatedAt time.Time
	UpdatedAt time.Time
}

func (Reward) TableName() string {
	return "reward"
}

type UserMinigamePlay struct {
	ID         uuid.UUID `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	UserID     uuid.UUID `gorm:"type:uuid;not null;index" json:"user_id"`
	MinigameID uuid.UUID `gorm:"type:uuid;not null;index" json:"minigame_id"`

	PointsEarned int32  `gorm:"not null;default:0" json:"points_earned"`
	Score        int32  `gorm:"default:0" json:"score"`
	GameResult   string `gorm:"type:text" json:"game_result"` // win, lose, draw

	PlayedAt time.Time `gorm:"not null;index" json:"played_at"`

	Metadata map[string]interface{} `gorm:"type:jsonb;serializer:json" json:"metadata"`

	CreatedAt time.Time `gorm:"autoCreateTime" json:"created_at"`
}

func (UserMinigamePlay) TableName() string {
	return "user_minigame_plays"
}
