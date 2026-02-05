package handler

import (
	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"study.com/v1/internal/dto"
	"study.com/v1/internal/service"
	"study.com/v1/internal/utils"
	"strings"
)

type UserOrganizationRoleHandlerInterface interface {
	// User APIs
	GetMyOrgRoles(c *fiber.Ctx) error

	// Admin APIs
	GetUserOrgRoles(c *fiber.Ctx) error
	AssignOrgRolesToUser(c *fiber.Ctx) error
	RevokeOrgRoleFromUser(c *fiber.Ctx) error

	// Role Management
	GetUsersWithOrgRoleSimple(c *fiber.Ctx) error

	// Organization Members
	GetOrganizationMembers(c *fiber.Ctx) error
	GetUsersWithOrgRole(c *fiber.Ctx) error
}

type UserOrganizationRoleHandler struct {
	service service.UserOrganizationRoleServiceInterface
}

func NewUserOrganizationRoleHandler(service service.UserOrganizationRoleServiceInterface) *UserOrganizationRoleHandler {
	return &UserOrganizationRoleHandler{service: service}
}

// ============ User APIs ============

// GetMyOrgRoles - GET /me/org-roles
func (h *UserOrganizationRoleHandler) GetMyOrgRoles(c *fiber.Ctx) error {
	userID, ok := c.Locals("user_id").(uuid.UUID)
	if !ok || userID == uuid.Nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"message": "Unauthorized",
		})
	}

	var orgID *uuid.UUID
	orgIDStr := c.Query("org_id")
	if orgIDStr != "" {
		parsed, err := uuid.Parse(orgIDStr)
		if err != nil {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
				"message": "Invalid organization ID format",
			})
		}
		orgID = &parsed
	}

	roles, err := h.service.GetMyOrgRoles(c.Context(), userID, orgID)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"message": "Failed to get organization roles",
			"error":   err.Error(),
		})
	}

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"message": "Success",
		"data":    roles,
	})
}

// ============ Admin APIs ============

// GetUserOrgRoles - GET /users/:user_id/org-roles
func (h *UserOrganizationRoleHandler) GetUserOrgRoles(c *fiber.Ctx) error {
	userIDStr := c.Params("user_id")
	userID, err := uuid.Parse(userIDStr)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Invalid user ID",
			"error":   err.Error(),
		})
	}

	status := c.Query("status")

	result, err := h.service.GetUserOrgRoles(c.Context(), userID, status)
	if err != nil {
		httpStatus := fiber.StatusInternalServerError
		if err.Error() == "user not found" {
			httpStatus = fiber.StatusNotFound
		}
		return c.Status(httpStatus).JSON(fiber.Map{
			"message": "Failed to retrieve user organization roles",
			"error":   err.Error(),
		})
	}

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"message": "Success",
		"data":    result,
	})
}

// AssignOrgRolesToUser - POST /users/:user_id/org-roles
func (h *UserOrganizationRoleHandler) AssignOrgRolesToUser(c *fiber.Ctx) error {
	userIDStr := c.Params("user_id")
	userID, err := uuid.Parse(userIDStr)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Invalid user ID",
			"error":   err.Error(),
		})
	}

	var req dto.AssignOrgRolesToUserDTO
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

	result, err := h.service.AssignOrgRolesToUser(c.Context(), userID, req, granterID)
	if err != nil {
		status := fiber.StatusInternalServerError
		errMsg := err.Error()
		if errMsg == "user not found" || errMsg == "organization not found" ||
			strings.Contains(errMsg, "role not found") || strings.Contains(errMsg, "role is not active") ||
			strings.Contains(errMsg, "does not belong to this organization") {
			status = fiber.StatusNotFound
		}
		if strings.Contains(errMsg, "roles already assigned") {
			status = fiber.StatusConflict
		}
		return c.Status(status).JSON(fiber.Map{
			"message": "Failed to assign organization roles to user",
			"error":   errMsg,
		})
	}

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"message": "Organization roles assigned successfully",
		"data":    result,
	})
}

// RevokeOrgRoleFromUser - DELETE /users/:user_id/org-roles/:org_role_id
func (h *UserOrganizationRoleHandler) RevokeOrgRoleFromUser(c *fiber.Ctx) error {
	userIDStr := c.Params("user_id")
	userID, err := uuid.Parse(userIDStr)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Invalid user ID format",
		})
	}

	orgRoleIDStr := c.Params("org_role_id")
	orgRoleID, err := uuid.Parse(orgRoleIDStr)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Invalid organization role ID format",
		})
	}

	revokerID, ok := c.Locals("user_id").(uuid.UUID)
	if !ok || revokerID == uuid.Nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"message": "Unauthorized",
		})
	}

	err = h.service.RevokeOrgRoleFromUser(c.Context(), userID, orgRoleID, revokerID)
	if err != nil {
		status := fiber.StatusInternalServerError
		errMsg := err.Error()
		if errMsg == "user not found" ||
			errMsg == "organization role assignment not found" ||
			errMsg == "role assignment does not belong to this user" {
			status = fiber.StatusNotFound
		}
		if errMsg == "organization role already inactive for this user" {
			status = fiber.StatusBadRequest
		}
		return c.Status(status).JSON(fiber.Map{
			"message": "Failed to revoke organization role",
			"error":   errMsg,
		})
	}

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"message": "Organization role revoked successfully",
	})
}

// ============ Role Management ============

// GetUsersWithOrgRoleSimple - GET /org-roles/:role_id/users
func (h *UserOrganizationRoleHandler) GetUsersWithOrgRoleSimple(c *fiber.Ctx) error {
	roleIDStr := c.Params("role_id")
	roleID, err := uuid.Parse(roleIDStr)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Invalid role ID",
			"error":   err.Error(),
		})
	}

	page := c.QueryInt("page", 1)
	pageSize := c.QueryInt("page_size", 20)
	status := c.Query("status")

	result, err := h.service.GetUsersWithOrgRoleByRoleID(c.Context(), roleID, page, pageSize, status)
	if err != nil {
		httpStatus := fiber.StatusInternalServerError
		if err.Error() == "organization role not found" {
			httpStatus = fiber.StatusNotFound
		}
		return c.Status(httpStatus).JSON(fiber.Map{
			"message": "Failed to retrieve users with role",
			"error":   err.Error(),
		})
	}

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"message": "Success",
		"data":    result,
	})
}

// ============ Organization Members ============

// GetOrganizationMembers - GET /organizations/:organization_id/members
func (h *UserOrganizationRoleHandler) GetOrganizationMembers(c *fiber.Ctx) error {
	organizationIDStr := c.Params("organization_id")
	organizationID, err := uuid.Parse(organizationIDStr)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Invalid organization ID",
			"error":   err.Error(),
		})
	}

	page := c.QueryInt("page", 1)
	pageSize := c.QueryInt("page_size", 20)
	status := c.Query("status")

	result, err := h.service.GetOrganizationMembers(c.Context(), organizationID, page, pageSize, status)
	if err != nil {
		httpStatus := fiber.StatusInternalServerError
		if err.Error() == "organization not found" {
			httpStatus = fiber.StatusNotFound
		}
		return c.Status(httpStatus).JSON(fiber.Map{
			"message": "Failed to retrieve organization members",
			"error":   err.Error(),
		})
	}

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"message": "Success",
		"data":    result,
	})
}

// GetUsersWithOrgRole - GET /organizations/:organization_id/roles/:role_id/users
func (h *UserOrganizationRoleHandler) GetUsersWithOrgRole(c *fiber.Ctx) error {
	organizationIDStr := c.Params("organization_id")
	organizationID, err := uuid.Parse(organizationIDStr)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Invalid organization ID",
			"error":   err.Error(),
		})
	}

	roleIDStr := c.Params("role_id")
	roleID, err := uuid.Parse(roleIDStr)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Invalid role ID",
			"error":   err.Error(),
		})
	}

	page := c.QueryInt("page", 1)
	pageSize := c.QueryInt("page_size", 20)
	status := c.Query("status")

	result, err := h.service.GetUsersWithOrgRole(c.Context(), roleID, organizationID, page, pageSize, status)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"message": "Failed to retrieve users with role",
			"error":   err.Error(),
		})
	}

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"message": "Success",
		"data":    result,
	})
}
