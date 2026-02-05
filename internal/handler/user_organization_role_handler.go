package handler

import (
	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"study.com/v1/internal/dto"
	"study.com/v1/internal/service"
	"study.com/v1/internal/utils"
)

type UserOrganizationRoleHandlerInterface interface {
	// Assign operations
	AssignOrgRolesToUser(c *fiber.Ctx) error

	// Query operations
	GetUserOrgRoleByID(c *fiber.Ctx) error
	GetUserOrgRoles(c *fiber.Ctx) error
	GetUserOrgRolesInOrganization(c *fiber.Ctx) error
	GetUsersWithOrgRole(c *fiber.Ctx) error
	GetOrganizationMembers(c *fiber.Ctx) error
	CheckUserHasOrgRole(c *fiber.Ctx) error

	// Status operations
	UpdateUserOrgRoleStatus(c *fiber.Ctx) error

	// Delete operations
	RemoveOrgRoleFromUser(c *fiber.Ctx) error
	RemoveAllOrgRolesFromUser(c *fiber.Ctx) error
	RemoveUserFromOrganization(c *fiber.Ctx) error
}

type UserOrganizationRoleHandler struct {
	service service.UserOrganizationRoleServiceInterface
}

func NewUserOrganizationRoleHandler(service service.UserOrganizationRoleServiceInterface) *UserOrganizationRoleHandler {
	return &UserOrganizationRoleHandler{service: service}
}

// ============ Assign Operations ============

// AssignOrgRolesToUser assigns organization roles to a user (single or multiple)
// POST /users/:user_id/organization-roles
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
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Failed to assign organization roles to user",
			"error":   err.Error(),
		})
	}

	return c.Status(fiber.StatusCreated).JSON(fiber.Map{
		"message": "Organization roles assigned to user successfully",
		"data":    result,
	})
}

// ============ Query Operations ============

// GetUserOrgRoleByID gets a user organization role by its ID
// GET /user-organization-roles/:id
func (h *UserOrganizationRoleHandler) GetUserOrgRoleByID(c *fiber.Ctx) error {
	idStr := c.Params("id")
	id, err := uuid.Parse(idStr)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Invalid user organization role ID",
			"error":   err.Error(),
		})
	}

	result, err := h.service.GetUserOrgRoleByID(c.Context(), id)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"message": "User organization role not found",
			"error":   err.Error(),
		})
	}

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"message": "User organization role retrieved successfully",
		"data":    result,
	})
}

// GetUserOrgRoles gets all organization roles of a user
// GET /users/:user_id/organization-roles
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

	result, err := h.service.GetUserOrgRoles(c.Context(), userID, status) // thêm limit offset 
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"message": "Failed to retrieve user organization roles",
			"error":   err.Error(),
		})
	}

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"message": "User organization roles retrieved successfully",
		"data":    result,
	})
}

// GetUserOrgRolesInOrganization gets all roles of a user in a specific organization
// GET /users/:user_id/organizations/:organization_id/roles
func (h *UserOrganizationRoleHandler) GetUserOrgRolesInOrganization(c *fiber.Ctx) error {
	userIDStr := c.Params("user_id")
	userID, err := uuid.Parse(userIDStr)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Invalid user ID",
			"error":   err.Error(),
		})
	}

	organizationIDStr := c.Params("organization_id")
	organizationID, err := uuid.Parse(organizationIDStr)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Invalid organization ID",
			"error":   err.Error(),
		})
	}

	status := c.Query("status")

	result, err := h.service.GetUserOrgRolesInOrganization(c.Context(), userID, organizationID, status)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"message": "Failed to retrieve user roles in organization",
			"error":   err.Error(),
		})
	}

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"message": "User roles in organization retrieved successfully",
		"data":    result,
	})
}

// GetUsersWithOrgRole gets all users with a specific role in an organization
// GET /organizations/:organization_id/roles/:role_id/users
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
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Failed to retrieve users with role",
			"error":   err.Error(),
		})
	}

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"message": "Users with role retrieved successfully",
		"data":    result,
	})
}

// GetOrganizationMembers gets all members of an organization
// GET /organizations/:organization_id/members
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
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"message": "Failed to retrieve organization members",
			"error":   err.Error(),
		})
	}

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"message": "Organization members retrieved successfully",
		"data":    result,
	})
}

// CheckUserHasOrgRole checks if a user has a specific role in an organization
// GET /users/:user_id/organizations/:organization_id/roles/:role_id/check
func (h *UserOrganizationRoleHandler) CheckUserHasOrgRole(c *fiber.Ctx) error {
	userIDStr := c.Params("user_id")
	userID, err := uuid.Parse(userIDStr)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Invalid user ID",
			"error":   err.Error(),
		})
	}

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

	hasRole, err := h.service.CheckUserHasOrgRole(c.Context(), userID, roleID, organizationID)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"message": "Failed to check user organization role",
			"error":   err.Error(),
		})
	}

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"message":  "Check completed",
		"has_role": hasRole,
	})
}

// ============ Status Operations ============

// UpdateUserOrgRoleStatus updates the status of a user organization role
// PATCH /user-organization-roles/:id/status
func (h *UserOrganizationRoleHandler) UpdateUserOrgRoleStatus(c *fiber.Ctx) error {
	idStr := c.Params("id")
	id, err := uuid.Parse(idStr)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Invalid user organization role ID",
			"error":   err.Error(),
		})
	}

	var req dto.UpdateUserOrgRoleStatusDTO
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

	result, err := h.service.UpdateUserOrgRoleStatus(c.Context(), id, req, updatedBy)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Failed to update user organization role status",
			"error":   err.Error(),
		})
	}

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"message": "User organization role status updated successfully",
		"data":    result,
	})
}

// ============ Delete Operations ============

// RemoveOrgRoleFromUser removes an organization role from a user
// DELETE /user-organization-roles/:id
func (h *UserOrganizationRoleHandler) RemoveOrgRoleFromUser(c *fiber.Ctx) error {
	idStr := c.Params("id")
	id, err := uuid.Parse(idStr)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Invalid user organization role ID",
			"error":   err.Error(),
		})
	}

	if err := h.service.RemoveOrgRoleFromUser(c.Context(), id); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Failed to remove organization role from user",
			"error":   err.Error(),
		})
	}

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"message": "Organization role removed from user successfully",
	})
}

// RemoveAllOrgRolesFromUser removes all organization roles from a user
// DELETE /users/:user_id/organization-roles
func (h *UserOrganizationRoleHandler) RemoveAllOrgRolesFromUser(c *fiber.Ctx) error {
	userIDStr := c.Params("user_id")
	userID, err := uuid.Parse(userIDStr)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Invalid user ID",
			"error":   err.Error(),
		})
	}

	if err := h.service.RemoveAllOrgRolesFromUser(c.Context(), userID); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Failed to remove all organization roles from user",
			"error":   err.Error(),
		})
	}

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"message": "All organization roles removed from user successfully",
	})
}

// RemoveUserFromOrganization removes a user from an organization
// DELETE /users/:user_id/organizations/:organization_id
func (h *UserOrganizationRoleHandler) RemoveUserFromOrganization(c *fiber.Ctx) error {
	userIDStr := c.Params("user_id")
	userID, err := uuid.Parse(userIDStr)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Invalid user ID",
			"error":   err.Error(),
		})
	}

	organizationIDStr := c.Params("organization_id")
	organizationID, err := uuid.Parse(organizationIDStr)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Invalid organization ID",
			"error":   err.Error(),
		})
	}

	if err := h.service.RemoveUserFromOrganization(c.Context(), userID, organizationID); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Failed to remove user from organization",
			"error":   err.Error(),
		})
	}

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"message": "User removed from organization successfully",
	})
}
