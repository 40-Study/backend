package handler

import (
	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
)	

func (h *AuthHandler) GetAllDevices(c *fiber.Ctx) error {
	// ===== 1. Get user_id from token (set by auth middleware) =====
	user_id := c.Locals("user_id")
	if user_id == nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Empty userId",
		})
	}

	userID, ok := user_id.(uuid.UUID)
	if !ok {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Invalid userId",
		})
	}

	// ===== 2. Get device_id from token =====
	device_id := c.Locals("device_id")
	if device_id == nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Empty deviceId",
		})
	}

	deviceID, ok := device_id.(uuid.UUID)
	if !ok {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Invalid deviceId",
		})
	}

	// ===== 3. Call service to get all devices =====
	devices, err := h.authService.GetAllDevices(c.Context(), userID, deviceID)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"message": "Failed to get devices",
			"error":   err.Error(),
		})
	}

	// ===== 4. Return device list =====
	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"message": "Get devices successfully",
		"data": fiber.Map{
			"devices": devices,
			"total":   len(devices),
		},
	})
}
