package handler

import (
	"github.com/gofiber/fiber/v2"
	"study.com/v1/internal/service"
)

// AdminSettingsHandler — trang "Cấu hình hệ thống" (contract C4). Đường GHI vẫn là
// PUT /admin/settings/platform-fee ở AdminOrderHandler; handler này chỉ đọc.
type AdminSettingsHandler struct {
	settings service.PlatformSettingServiceInterface
}

func NewAdminSettingsHandler(settings service.PlatformSettingServiceInterface) *AdminSettingsHandler {
	return &AdminSettingsHandler{settings: settings}
}

// GetSettings - GET /api/admin/settings (SYSTEM_SETTINGS_MANAGE)
func (h *AdminSettingsHandler) GetSettings(c *fiber.Ctx) error {
	data, err := h.settings.GetSettings(c.Context())
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"message": "Không tải được cấu hình hệ thống",
			"code":    "SETTINGS_LOAD_FAILED",
		})
	}
	return c.JSON(fiber.Map{"message": "success", "data": data})
}
