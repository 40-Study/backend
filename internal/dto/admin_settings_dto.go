package dto

import (
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

// Contract C4 (plans/261008-qa-followup-features/contract.md): GET /api/admin/settings.
// Đường ghi giữ nguyên: PUT /api/admin/settings/platform-fee (UpdatePlatformFeeRequest, orderDTO.go).

// AdminSettingsUpdaterDTO — admin sửa cấu hình gần nhất.
type AdminSettingsUpdaterDTO struct {
	ID   uuid.UUID `json:"id"`
	Name string    `json:"name"`
}

// AdminSettingsDTO — platform_fee_percent mã hoá GIỐNG PlatformFeeSettingResponse (decimal.Decimal);
// updated_at/updated_by là null (không omitempty) khi cấu hình chưa từng bị sửa.
type AdminSettingsDTO struct {
	PlatformFeePercent decimal.Decimal          `json:"platform_fee_percent"`
	UpdatedAt          *time.Time               `json:"updated_at"`
	UpdatedBy          *AdminSettingsUpdaterDTO `json:"updated_by"`
}
