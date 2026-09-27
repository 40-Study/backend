package model

import (
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

// PlatformSetting là bảng ĐƠN DÒNG (singleton, id cố định = SingletonSettingID) lưu tham số cấu
// hình nền tảng. Hiện chỉ có PlatformFeePercent (quyết định chủ dự án 27/09/2026 #2: "phí nền
// tảng CÓ, một tỉ lệ % chung cấu hình được, mặc định 0% cho tới khi chủ dự án đặt").
//
// Dùng 1 dòng cố định thay vì bảng key-value tổng quát: hiện đúng 1 tham số cần cấu hình, thêm
// tổng quát hoá key/value cho một nhu cầu CHƯA CÓ là vi phạm YAGNI (coding-guidelines.md §2).
// Khi có tham số thứ 2 thật sự cần cấu hình, mở rộng bảng này (thêm cột) — vẫn đơn giản hơn
// key/value vì mỗi tham số giữ đúng kiểu dữ liệu của nó (decimal, không phải string phải parse).
type PlatformSetting struct {
	ID                 int             `gorm:"primaryKey;autoIncrement:false" json:"id"`
	PlatformFeePercent decimal.Decimal `gorm:"type:decimal(5,2);not null;default:0" json:"platform_fee_percent"`
	UpdatedAt          time.Time       `gorm:"autoUpdateTime" json:"updated_at"`
	UpdatedBy          *uuid.UUID      `gorm:"type:uuid" json:"updated_by,omitempty"`
}

func (PlatformSetting) TableName() string {
	return "platform_settings"
}

// SingletonSettingID là id CỐ ĐỊNH của dòng duy nhất trong bảng platform_settings.
const SingletonSettingID = 1
