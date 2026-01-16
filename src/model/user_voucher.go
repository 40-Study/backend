package model

import (
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"gorm.io/gorm"
)

// UserVoucher - Danh sách voucher mà user đã lưu/bookmark
// Chỉ dùng để hiển thị voucher đã save, không track usage
type UserVoucher struct {
	ID        uuid.UUID `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	UserID    uuid.UUID `gorm:"type:uuid;not null;uniqueIndex:uidx_user_vouchers_user_voucher,priority:1" json:"user_id"`
	VoucherID uuid.UUID `gorm:"type:uuid;not null;uniqueIndex:uidx_user_vouchers_user_voucher,priority:2" json:"voucher_id"`

	// Thời gian user lưu voucher này
	SavedAt time.Time `gorm:"not null;index" json:"saved_at"`

	// Source: user tự lưu, admin cấp, event reward, etc.
	Source pgtype.Text `json:"source"`

	CreatedAt time.Time      `gorm:"autoCreateTime" json:"created_at"`
	DeletedAt gorm.DeletedAt `gorm:"index" json:"deleted_at"` // Soft delete khi user "bỏ lưu"

	// Relations
	User    *User    `gorm:"foreignKey:UserID" json:"user,omitempty"`
	Voucher *Voucher `gorm:"foreignKey:VoucherID" json:"voucher,omitempty"`
}

func (UserVoucher) TableName() string {
	return "user_vouchers"
}

const (
	UserVoucherSourceManualClaim  = "manual_claim"  // User tự nhập code
	UserVoucherSourceAdminGrant   = "admin_grant"   // Admin cấp
	UserVoucherSourceEventReward  = "event_reward"  // Thưởng từ event
	UserVoucherSourceReferral     = "referral"      // Giới thiệu bạn bè
	UserVoucherSourcePurchase     = "purchase"      // Mua voucher
	UserVoucherSourceLoyaltyPoint = "loyalty_point" // Đổi điểm
	UserVoucherSourcePromotion    = "promotion"     // Campaign tặng
)
