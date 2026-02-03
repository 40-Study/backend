package handler

import (
	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"study.com/v1/internal/dto"
	"study.com/v1/internal/service"
	"study.com/v1/internal/utils"
)

type UserSystemRoleHandlerInterface interface {
	// Assign operations
	AssignSystemRolesToUser(c *fiber.Ctx) error

	// Query operations
	GetUserSystemRoleByID(c *fiber.Ctx) error
	GetUserSystemRoles(c *fiber.Ctx) error
	GetUsersWithSystemRole(c *fiber.Ctx) error
	CheckUserHasSystemRole(c *fiber.Ctx) error

	// Status operations
	UpdateUserSystemRoleStatus(c *fiber.Ctx) error

	// Delete operations
	RemoveSystemRoleFromUser(c *fiber.Ctx) error
	RemoveAllSystemRolesFromUser(c *fiber.Ctx) error
}

type UserSystemRoleHandler struct {
	service service.UserSystemRoleServiceInterface
}

func NewUserSystemRoleHandler(service service.UserSystemRoleServiceInterface) *UserSystemRoleHandler {
	return &UserSystemRoleHandler{service: service}
}

// ============ Assign Operations ============

// AssignSystemRolesToUser assigns system roles to a user (single or multiple)
// POST /users/:user_id/system-roles
func (h *UserSystemRoleHandler) AssignSystemRolesToUser(c *fiber.Ctx) error {
	userIDStr := c.Params("user_id")
	userID, err := uuid.Parse(userIDStr)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Invalid user ID",
			"error":   err.Error(),
		})
	}

	var req dto.AssignSystemRolesToUserDTO
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Invalid request body",
			"error":   err.Error(),
		})
	}

	if validationErrors := utils.ValidateStruct(req); len(validationErrors) > 0 {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Validation failed",
			"errors":  validationErrors,
		})
	}

	granterID, ok := c.Locals("user_id").(uuid.UUID)
	if !ok || granterID == uuid.Nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"message": "User not authenticated",
		})
	}

	result, err := h.service.AssignSystemRolesToUser(c.Context(), userID, req, granterID)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Failed to assign system roles to user",
			"error":   err.Error(),
		})
	}

	return c.Status(fiber.StatusCreated).JSON(fiber.Map{
		"message": "System roles assigned to user successfully",
		"data":    result,
	})
}

// ============ Query Operations ============

// GetUserSystemRoleByID gets a user system role by its ID
// GET /user-system-roles/:id
func (h *UserSystemRoleHandler) GetUserSystemRoleByID(c *fiber.Ctx) error {
	idStr := c.Params("id")
	id, err := uuid.Parse(idStr)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Invalid user system role ID",
			"error":   err.Error(),
		})
	}

	result, err := h.service.GetUserSystemRoleByID(c.Context(), id)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"message": "User system role not found",
			"error":   err.Error(),
		})
	}

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"message": "User system role retrieved successfully",
		"data":    result,
	})
}

// GetUserSystemRoles gets all system roles of a user
// GET /users/:user_id/system-roles
func (h *UserSystemRoleHandler) GetUserSystemRoles(c *fiber.Ctx) error {
	userIDStr := c.Params("user_id")
	userID, err := uuid.Parse(userIDStr)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Invalid user ID",
			"error":   err.Error(),
		})
	}

	status := c.Query("status")

	result, err := h.service.GetUserSystemRoles(c.Context(), userID, status)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"message": "Failed to retrieve user system roles",
			"error":   err.Error(),
		})
	}

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"message": "User system roles retrieved successfully",
		"data":    result,
	})
}

// GetUsersWithSystemRole gets all users with a specific system role
// GET /system-roles/:system_role_id/users
func (h *UserSystemRoleHandler) GetUsersWithSystemRole(c *fiber.Ctx) error {
	systemRoleIDStr := c.Params("system_role_id")
	systemRoleID, err := uuid.Parse(systemRoleIDStr)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Invalid system role ID",
			"error":   err.Error(),
		})
	}

	page := c.QueryInt("page", 1)
	pageSize := c.QueryInt("page_size", 20)
	status := c.Query("status")

	result, err := h.service.GetUsersWithSystemRole(c.Context(), systemRoleID, page, pageSize, status)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Failed to retrieve users with system role",
			"error":   err.Error(),
		})
	}

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"message": "Users with system role retrieved successfully",
		"data":    result,
	})
}

// CheckUserHasSystemRole checks if a user has a specific system role
// GET /users/:user_id/system-roles/:system_role_id/check
func (h *UserSystemRoleHandler) CheckUserHasSystemRole(c *fiber.Ctx) error {
	userIDStr := c.Params("user_id")
	userID, err := uuid.Parse(userIDStr)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Invalid user ID",
			"error":   err.Error(),
		})
	}

	systemRoleIDStr := c.Params("system_role_id")
	systemRoleID, err := uuid.Parse(systemRoleIDStr)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Invalid system role ID",
			"error":   err.Error(),
		})
	}

	hasRole, err := h.service.CheckUserHasSystemRole(c.Context(), userID, systemRoleID)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"message": "Failed to check user system role",
			"error":   err.Error(),
		})
	}

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"message":  "Check completed",
		"has_role": hasRole,
	})
}

// ============ Status Operations ============

// UpdateUserSystemRoleStatus updates the status of a user system role
// PATCH /user-system-roles/:id/status
func (h *UserSystemRoleHandler) UpdateUserSystemRoleStatus(c *fiber.Ctx) error {
	idStr := c.Params("id")
	id, err := uuid.Parse(idStr)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Invalid user system role ID",
			"error":   err.Error(),
		})
	}

	var req dto.UpdateUserSystemRoleStatusDTO
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Invalid request body",
			"error":   err.Error(),
		})
	}

	updatedBy, ok := c.Locals("user_id").(uuid.UUID)
	if !ok || updatedBy == uuid.Nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"message": "User not authenticated",
		})
	}

	result, err := h.service.UpdateUserSystemRoleStatus(c.Context(), id, req, updatedBy)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Failed to update user system role status",
			"error":   err.Error(),
		})
	}

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"message": "User system role status updated successfully",
		"data":    result,
	})
}

// ============ Delete Operations ============

// RemoveSystemRoleFromUser removes a system role from a user
// DELETE /user-system-roles/:id
func (h *UserSystemRoleHandler) RemoveSystemRoleFromUser(c *fiber.Ctx) error {
	idStr := c.Params("id")
	id, err := uuid.Parse(idStr)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Invalid user system role ID",
			"error":   err.Error(),
		})
	}

	if err := h.service.RemoveSystemRoleFromUser(c.Context(), id); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Failed to remove system role from user",
			"error":   err.Error(),
		})
	}

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"message": "System role removed from user successfully",
	})
}

// RemoveAllSystemRolesFromUser removes all system roles from a user
// DELETE /users/:user_id/system-roles
func (h *UserSystemRoleHandler) RemoveAllSystemRolesFromUser(c *fiber.Ctx) error {
	userIDStr := c.Params("user_id")
	userID, err := uuid.Parse(userIDStr)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Invalid user ID",
			"error":   err.Error(),
		})
	}

	if err := h.service.RemoveAllSystemRolesFromUser(c.Context(), userID); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Failed to remove all system roles from user",
			"error":   err.Error(),
		})
	}

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"message": "All system roles removed from user successfully",
	})
}
