package handler

import (
	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"study.com/v1/internal/dto"
	"study.com/v1/internal/service"
	"study.com/v1/internal/utils"
)

type UserSystemRoleHandlerInterface interface {
	// User APIs
	GetMySystemRoles(c *fiber.Ctx) error

	// Admin APIs
	AssignSystemRolesToUser(c *fiber.Ctx) error
	RevokeSystemRoleFromUser(c *fiber.Ctx) error
	GetUserSystemRoles(c *fiber.Ctx) error
	GetUsersBySystemRole(c *fiber.Ctx) error
}

type UserSystemRoleHandler struct {
	userSystemRoleService service.UserSystemRoleServiceInterface
}

func NewUserSystemRoleHandler(userSystemRoleService service.UserSystemRoleServiceInterface) *UserSystemRoleHandler {
	return &UserSystemRoleHandler{
		userSystemRoleService: userSystemRoleService,
	}
}

// ============ User APIs ============

// GetMySystemRoles godoc
// @Summary Get my system roles
// @Description Get system roles of the authenticated user
// @Tags User System Roles
// @Accept json
// @Produce json
// @Security BearerAuth
// @Success 200 {object} map[string]interface{}
// @Failure 401 {object} map[string]interface{}
// @Failure 500 {object} map[string]interface{}
// @Router /me/system-roles [get]
func (h *UserSystemRoleHandler) GetMySystemRoles(c *fiber.Ctx) error {
	// Get user ID from context (set by auth middleware)
	userID := c.Locals("user_id")
	if userID == nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"message": "Unauthorized",
		})
	}

	userUUID, ok := userID.(uuid.UUID)
	if !ok {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"message": "Invalid user ID",
		})
	}

	roles, err := h.userSystemRoleService.GetMySystemRoles(c.Context(), userUUID)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"message": "Failed to get system roles",
			"error":   err.Error(),
		})
	}

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"message": "Success",
		"data":    roles,
	})
}

// ============ Admin APIs ============

// AssignSystemRolesToUser godoc
// @Summary Assign system roles to user
// @Description Assign one or more system roles to a user (Admin only)
// @Tags User System Roles
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param user_id path string true "User ID"
// @Param request body dto.AssignSystemRolesToUserDTO true "System role IDs to assign"
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} map[string]interface{}
// @Failure 401 {object} map[string]interface{}
// @Failure 404 {object} map[string]interface{}
// @Failure 500 {object} map[string]interface{}
// @Router /users/{user_id}/system-roles [post]
func (h *UserSystemRoleHandler) AssignSystemRolesToUser(c *fiber.Ctx) error {
	// 1. Parse user_id from path
	userIDStr := c.Params("user_id")
	userID, err := uuid.Parse(userIDStr)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Invalid user ID format",
		})
	}

	// 2. Parse request body
	var req dto.AssignSystemRolesToUserDTO
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Invalid request body",
			"error":   err.Error(),
		})
	}

	// 3. Validate request
	if errs := utils.ValidateStruct(req); len(errs) > 0 {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Validation failed",
			"errors":  errs,
		})
	}

	// 4. Get granter ID from context
	granterID := c.Locals("user_id")
	if granterID == nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"message": "Unauthorized",
		})
	}
	granterUUID, _ := granterID.(uuid.UUID)

	// 5. Call service
	result, err := h.userSystemRoleService.AssignSystemRolesToUser(c.Context(), userID, req, granterUUID)
	if err != nil {
		status := fiber.StatusInternalServerError
		if err.Error() == "user not found" || 
		   containsString(err.Error(), "system role not found") ||
		   containsString(err.Error(), "system role is not active") {
			status = fiber.StatusNotFound
		}
		if containsString(err.Error(), "roles already assigned") {
			status = fiber.StatusConflict
		}
		return c.Status(status).JSON(fiber.Map{
			"message": "Failed to assign system roles",
			"error":   err.Error(),
		})
	}

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"message": "System roles assigned successfully",
		"data":    result,
	})
}

// RevokeSystemRoleFromUser godoc
// @Summary Revoke system role from user
// @Description Remove/revoke a system role from a user (Admin only)
// @Tags User System Roles
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param user_id path string true "User ID"
// @Param system_role_id path string true "System Role ID"
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} map[string]interface{}
// @Failure 401 {object} map[string]interface{}
// @Failure 404 {object} map[string]interface{}
// @Failure 500 {object} map[string]interface{}
// @Router /users/{user_id}/system-roles/{system_role_id} [delete]
func (h *UserSystemRoleHandler) RevokeSystemRoleFromUser(c *fiber.Ctx) error {
	// 1. Parse user_id from path
	userIDStr := c.Params("user_id")
	userID, err := uuid.Parse(userIDStr)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Invalid user ID format",
		})
	}

	// 2. Parse system_role_id from path
	systemRoleIDStr := c.Params("system_role_id")
	systemRoleID, err := uuid.Parse(systemRoleIDStr)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Invalid system role ID format",
		})
	}

	// 3. Get revoker ID from context
	revokerID := c.Locals("user_id")
	if revokerID == nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"message": "Unauthorized",
		})
	}
	revokerUUID, _ := revokerID.(uuid.UUID)

	// 4. Call service
	err = h.userSystemRoleService.RevokeSystemRoleFromUser(c.Context(), userID, systemRoleID, revokerUUID)
	if err != nil {
		status := fiber.StatusInternalServerError
		if err.Error() == "user not found" || 
		   err.Error() == "system role not found" ||
		   err.Error() == "user does not have this system role" {
			status = fiber.StatusNotFound
		}
		if err.Error() == "system role already inactive for this user" {
			status = fiber.StatusBadRequest
		}
		return c.Status(status).JSON(fiber.Map{
			"message": "Failed to revoke system role",
			"error":   err.Error(),
		})
	}

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"message": "System role revoked successfully",
	})
}

// GetUserSystemRoles godoc
// @Summary Get user's system roles
// @Description Get all system roles assigned to a specific user (Admin only)
// @Tags User System Roles
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param user_id path string true "User ID"
// @Param status query string false "Filter by status (active, suspended, revoked)"
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} map[string]interface{}
// @Failure 401 {object} map[string]interface{}
// @Failure 404 {object} map[string]interface{}
// @Failure 500 {object} map[string]interface{}
// @Router /users/{user_id}/system-roles [get]
func (h *UserSystemRoleHandler) GetUserSystemRoles(c *fiber.Ctx) error {
	// 1. Parse user_id from path
	userIDStr := c.Params("user_id")
	userID, err := uuid.Parse(userIDStr)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Invalid user ID format",
		})
	}

	// 2. Get query params
	status := c.Query("status", "")

	// 3. Call service
	result, err := h.userSystemRoleService.GetUserSystemRoles(c.Context(), userID, status)
	if err != nil {
		status := fiber.StatusInternalServerError
		if err.Error() == "user not found" {
			status = fiber.StatusNotFound
		}
		return c.Status(status).JSON(fiber.Map{
			"message": "Failed to get user system roles",
			"error":   err.Error(),
		})
	}

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"message": "Success",
		"data":    result,
	})
}

// GetUsersBySystemRole godoc
// @Summary Get users by system role
// @Description Get all users assigned to a specific system role (Admin only)
// @Tags User System Roles
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param system_role_id path string true "System Role ID"
// @Param status query string false "Filter by status (active, suspended, revoked)"
// @Param page query int false "Page number" default(1)
// @Param page_size query int false "Page size" default(20)
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} map[string]interface{}
// @Failure 401 {object} map[string]interface{}
// @Failure 404 {object} map[string]interface{}
// @Failure 500 {object} map[string]interface{}
// @Router /system-roles/{system_role_id}/users [get]
func (h *UserSystemRoleHandler) GetUsersBySystemRole(c *fiber.Ctx) error {
	// 1. Parse system_role_id from path
	systemRoleIDStr := c.Params("system_role_id")
	systemRoleID, err := uuid.Parse(systemRoleIDStr)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Invalid system role ID format",
		})
	}

	// 2. Get query params
	status := c.Query("status", "")
	page := c.QueryInt("page", 1)
	pageSize := c.QueryInt("page_size", 20)

	// 3. Call service
	result, err := h.userSystemRoleService.GetUsersBySystemRole(c.Context(), systemRoleID, page, pageSize, status)
	if err != nil {
		httpStatus := fiber.StatusInternalServerError
		if err.Error() == "system role not found" {
			httpStatus = fiber.StatusNotFound
		}
		return c.Status(httpStatus).JSON(fiber.Map{
			"message": "Failed to get users",
			"error":   err.Error(),
		})
	}

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"message": "Success",
		"data":    result,
	})
}

// Helper function
func containsString(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsSubstring(s, substr))
}

func containsSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
