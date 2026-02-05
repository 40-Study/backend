package handler

import (
	"strings"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"study.com/v1/internal/dto"
	"study.com/v1/internal/service"
	"study.com/v1/internal/utils"
)

type UserSystemRoleHandlerInterface interface {
	GetMySystemRoles(c *fiber.Ctx) error
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

// GetMySystemRoles lay system roles cua chinh minh
// GET /me/system-roles
func (h *UserSystemRoleHandler) GetMySystemRoles(c *fiber.Ctx) error {
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

// AssignSystemRolesToUser gan system roles cho user
// POST /users/:user_id/system-roles
func (h *UserSystemRoleHandler) AssignSystemRolesToUser(c *fiber.Ctx) error {
	// Parse user_id tu path
	userIDStr := c.Params("user_id")
	userID, err := uuid.Parse(userIDStr)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Invalid user ID format",
		})
	}

	// Parse request body
	var req dto.AssignSystemRolesToUserDTO
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Invalid request body",
			"error":   err.Error(),
		})
	}

	// Validate request
	if errs := utils.ValidateStruct(req); len(errs) > 0 {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Validation failed",
			"errors":  errs,
		})
	}

	// Lay granter ID tu context
	granterID := c.Locals("user_id")
	if granterID == nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"message": "Unauthorized",
		})
	}
	granterUUID, _ := granterID.(uuid.UUID)

	// Goi service
	result, err := h.userSystemRoleService.AssignSystemRolesToUser(c.Context(), userID, req, granterUUID)
	if err != nil {
		status := fiber.StatusInternalServerError
		errMsg := err.Error()
		if errMsg == "user not found" ||
			strings.Contains(errMsg, "system role not found") ||
			strings.Contains(errMsg, "system role is not active") {
			status = fiber.StatusNotFound
		}
		if strings.Contains(errMsg, "roles already assigned") {
			status = fiber.StatusConflict
		}
		return c.Status(status).JSON(fiber.Map{
			"message": "Failed to assign system roles",
			"error":   errMsg,
		})
	}

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"message": "System roles assigned successfully",
		"data":    result,
	})
}

// RevokeSystemRoleFromUser thu hoi system role tu user
// DELETE /users/:user_id/system-roles/:system_role_id
func (h *UserSystemRoleHandler) RevokeSystemRoleFromUser(c *fiber.Ctx) error {
	// Parse user_id tu path
	userIDStr := c.Params("user_id")
	userID, err := uuid.Parse(userIDStr)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Invalid user ID format",
		})
	}

	// Parse system_role_id tu path
	systemRoleIDStr := c.Params("system_role_id")
	systemRoleID, err := uuid.Parse(systemRoleIDStr)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Invalid system role ID format",
		})
	}

	// Lay revoker ID tu context
	revokerID := c.Locals("user_id")
	if revokerID == nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"message": "Unauthorized",
		})
	}
	revokerUUID, _ := revokerID.(uuid.UUID)

	// Goi service
	err = h.userSystemRoleService.RevokeSystemRoleFromUser(c.Context(), userID, systemRoleID, revokerUUID)
	if err != nil {
		status := fiber.StatusInternalServerError
		errMsg := err.Error()
		if errMsg == "user not found" ||
			errMsg == "system role not found" ||
			errMsg == "user does not have this system role" {
			status = fiber.StatusNotFound
		}
		if errMsg == "system role already inactive for this user" {
			status = fiber.StatusBadRequest
		}
		return c.Status(status).JSON(fiber.Map{
			"message": "Failed to revoke system role",
			"error":   errMsg,
		})
	}

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"message": "System role revoked successfully",
	})
}

// GetUserSystemRoles lay system roles cua mot user
// GET /users/:user_id/system-roles
func (h *UserSystemRoleHandler) GetUserSystemRoles(c *fiber.Ctx) error {
	// Parse user_id tu path
	userIDStr := c.Params("user_id")
	userID, err := uuid.Parse(userIDStr)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Invalid user ID format",
		})
	}

	// Lay query params
	status := c.Query("status", "")

	// Goi service
	result, err := h.userSystemRoleService.GetUserSystemRoles(c.Context(), userID, status)
	if err != nil {
		httpStatus := fiber.StatusInternalServerError
		if err.Error() == "user not found" {
			httpStatus = fiber.StatusNotFound
		}
		return c.Status(httpStatus).JSON(fiber.Map{
			"message": "Failed to get user system roles",
			"error":   err.Error(),
		})
	}

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"message": "Success",
		"data":    result,
	})
}

// GetUsersBySystemRole lay danh sach users theo system role
// GET /system-roles/:system_role_id/users
func (h *UserSystemRoleHandler) GetUsersBySystemRole(c *fiber.Ctx) error {
	// Parse system_role_id tu path
	systemRoleIDStr := c.Params("system_role_id")
	systemRoleID, err := uuid.Parse(systemRoleIDStr)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Invalid system role ID format",
		})
	}

	// Lay query params
	status := c.Query("status", "")
	page := c.QueryInt("page", 1)
	pageSize := c.QueryInt("page_size", 20)

	// Goi service
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
