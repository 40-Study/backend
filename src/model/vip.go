package model

import (
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/shopspring/decimal"
	"gorm.io/gorm"
)

// 49. VipTier (Bảng cấp VIP)
type VipTier struct {
	ID                 uuid.UUID              `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	Name               string                 `gorm:"not null" json:"name"`
	Level              int32                  `gorm:"unique;not null" json:"level"`
	Description        pgtype.Text            `json:"description"`
	IconUrl            pgtype.Text            `json:"icon_url"`
	BadgeUrl           pgtype.Text            `json:"badge_url"`
	Color              pgtype.Text            `json:"color"`
	MinPoints          int32                  `gorm:"not null" json:"min_points"`
	Benefits           map[string]interface{} `gorm:"type:jsonb;serializer:json" json:"benefits"`
	DiscountPercentage *decimal.Decimal       `gorm:"type:numeric" json:"discount_percentage"`
	CreatedAt          time.Time              `gorm:"autoCreateTime" json:"created_at"`
	UpdatedAt          time.Time              `gorm:"autoUpdateTime" json:"updated_at"`
	DeletedAt          gorm.DeletedAt         `gorm:"index" json:"deleted_at"`
}

func (VipTier) TableName() string {
	return "vip_tiers"
}

// 47. VipPackage (Bảng gói VIP)
type VipPackage struct {
	ID            uuid.UUID        `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	TierID        uuid.UUID        `gorm:"type:uuid;not null;index" json:"tier_id"`
	Name          string           `gorm:"not null" json:"name"`
	Description   pgtype.Text      `json:"description"`
	DurationDays  int32            `gorm:"not null" json:"duration_days"`
	Price         *decimal.Decimal `gorm:"type:numeric" json:"price"`
	DiscountPrice *decimal.Decimal `gorm:"type:numeric" json:"discount_price"`
	BonusPoints   pgtype.Int4      `json:"bonus_points"`
	IsPopular     pgtype.Bool      `gorm:"default:false" json:"is_popular"`
	IsActive      pgtype.Bool      `gorm:"default:true" json:"is_active"`
	DisplayOrder  pgtype.Int4      `json:"display_order"`
	CreatedAt     time.Time        `gorm:"autoCreateTime" json:"created_at"`
	UpdatedAt     time.Time        `gorm:"autoUpdateTime" json:"updated_at"`
	DeletedAt     gorm.DeletedAt   `gorm:"index" json:"deleted_at"`
}

func (VipPackage) TableName() string {
	return "vip_packages"
}

// 44. UserVip (Bảng VIP người dùng)
type UserVip struct {
	ID             uuid.UUID        `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	UserID         uuid.UUID        `gorm:"type:uuid;unique;not null" json:"user_id"` // unique for active, handled by logic or partial index
	TierID         uuid.UUID        `gorm:"type:uuid;not null;index" json:"tier_id"`
	PackageID      pgtype.Int4      `json:"package_id"`
	StartDate      time.Time        `gorm:"not null" json:"start_date"`
	EndDate        time.Time        `gorm:"not null" json:"end_date"`
	IsActive       pgtype.Bool      `gorm:"default:true" json:"is_active"`
	AutoRenew      pgtype.Bool      `gorm:"default:false" json:"auto_renew"`
	TotalSpent     *decimal.Decimal `gorm:"type:numeric" json:"total_spent"`
	LifetimePoints pgtype.Int4      `json:"lifetime_points"`
	CreatedAt      time.Time        `gorm:"autoCreateTime" json:"created_at"`
	UpdatedAt      time.Time        `gorm:"autoUpdateTime" json:"updated_at"`
	DeletedAt      gorm.DeletedAt   `gorm:"index" json:"deleted_at"`
}

func (UserVip) TableName() string {
	return "user_vips"
}

// 58. VipPurchaseHistory (Bảng lịch sử mua VIP)
type VipPurchaseHistory struct {
	ID            uuid.UUID        `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	UserID        uuid.UUID        `gorm:"type:uuid;not null;index" json:"user_id"`
	PackageID     uuid.UUID        `gorm:"type:uuid;not null;index" json:"package_id"`
	PurchaseDate  time.Time        `gorm:"autoCreateTime" json:"purchase_date"`
	AmountPaid    *decimal.Decimal `gorm:"type:numeric;not null" json:"amount_paid"`
	Status        pgtype.Text      `gorm:"default:'completed'" json:"status"`
	TransactionID pgtype.Text      `json:"transaction_id"`
	CreatedAt     time.Time        `gorm:"autoCreateTime" json:"created_at"`
}

func (VipPurchaseHistory) TableName() string {
	return "vip_purchase_histories"
}

// 59. VipReward (Bảng phần thưởng VIP)
type VipReward struct {
	ID          uuid.UUID        `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	TierID      uuid.UUID        `gorm:"type:uuid;not null;index" json:"tier_id"`
	RewardType  string           `gorm:"not null" json:"reward_type"`
	RewardName  string           `gorm:"not null" json:"reward_name"`
	RewardValue *decimal.Decimal `gorm:"type:numeric" json:"reward_value"`
	Description pgtype.Text      `json:"description"`
	Frequency   pgtype.Text      `json:"frequency"`
	IsActive    pgtype.Bool      `gorm:"default:true" json:"is_active"`
	CreatedAt   time.Time        `gorm:"autoCreateTime" json:"created_at"`
	UpdatedAt   time.Time        `gorm:"autoUpdateTime" json:"updated_at"`
}

func (VipReward) TableName() string {
	return "vip_rewards"
}

// 45. UserVipClaim (Bảng nhận thưởng VIP)
type UserVipClaim struct {
	ID             uuid.UUID              `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	UserID         uuid.UUID              `gorm:"type:uuid;not null;index" json:"user_id"`
	RewardID       uuid.UUID              `gorm:"type:uuid;not null;index" json:"reward_id"`
	ClaimedAt      time.Time              `gorm:"autoCreateTime" json:"claimed_at"`
	RewardReceived map[string]interface{} `gorm:"type:jsonb;serializer:json" json:"reward_received"`
}

func (UserVipClaim) TableName() string {
	return "user_vip_claims"
}
