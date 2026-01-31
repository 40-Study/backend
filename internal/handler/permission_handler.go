package handler

import (
	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"study.com/v1/internal/dto"
	"study.com/v1/internal/service"
)

type PermissionHandlerInterface interface {
	GetPermission(c *fiber.Ctx) error
	GetAllPermissions(c *fiber.Ctx) error
	UpdatePermission(c *fiber.Ctx) error
}

type PermissionHandler struct {
	service service.PermissionServiceInterface
}

func NewPermissionHandler(service service.PermissionServiceInterface) *PermissionHandler {
	return &PermissionHandler{service: service}
}

func (h *PermissionHandler) GetPermission(c *fiber.Ctx) error {
	idStr := c.Params("id")
	id, err := uuid.Parse(idStr)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Invalid permission ID",
			"error":   err.Error(),
		})
	}

	permission, err := h.service.GetPermissionByID(c.Context(), id)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"message": "Permission not found",
			"error":   err.Error(),
		})
	}

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"message": "Permission retrieved successfully",
		"data":    permission,
	})
}

func (h *PermissionHandler) GetAllPermissions(c *fiber.Ctx) error {
	page := c.QueryInt("page", 1)
	pageSize := c.QueryInt("page_size", 20)

	permissions, err := h.service.GetAllPermissions(c.Context(), page, pageSize)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"message": "Failed to retrieve permissions",
			"error":   err.Error(),
		})
	}

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"message": "Permissions retrieved successfully",
		"data":    permissions,
	})
}

func (h *PermissionHandler) UpdatePermission(c *fiber.Ctx) error {
	idStr := c.Params("id")
	id, err := uuid.Parse(idStr)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Invalid permission ID",
			"error":   err.Error(),
		})
	}

	var req dto.UpdatePermissionDTO
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Invalid request body",
			"error":   err.Error(),
		})
	}

	permission, err := h.service.UpdatePermission(c.Context(), id, req)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Failed to update permission",
			"error":   err.Error(),
		})
	}

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"message": "Permission updated successfully",
		"data":    permission,
	})
}
