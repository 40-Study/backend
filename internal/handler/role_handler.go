package handler

import (
	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"study.com/v1/internal/dto"
	"study.com/v1/internal/service"
)

type RoleHandlerInterface interface {
	CreateRole(c *fiber.Ctx) error
	GetRole(c *fiber.Ctx) error
	GetAllRoles(c *fiber.Ctx) error
	UpdateRole(c *fiber.Ctx) error
	DeleteRole(c *fiber.Ctx) error

	// Role-Permission management
	AddPermissionsToRole(c *fiber.Ctx) error
	RemovePermissionsFromRole(c *fiber.Ctx) error
	SetRolePermissions(c *fiber.Ctx) error
	GetRolePermissions(c *fiber.Ctx) error
}

type RoleHandler struct {
	service service.RoleServiceInterface
}

func NewRoleHandler(service service.RoleServiceInterface) *RoleHandler {
	return &RoleHandler{service: service}
}

func (h *RoleHandler) CreateRole(c *fiber.Ctx) error {
	var req dto.CreateRoleDTO
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Invalid request body",
			"error":   err.Error(),
		})
	}

	role, err := h.service.CreateRole(c.Context(), req)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Failed to create role",
			"error":   err.Error(),
		})
	}

	return c.Status(fiber.StatusCreated).JSON(fiber.Map{
		"message": "Role created successfully",
		"data":    role,
	})
}

func (h *RoleHandler) GetRole(c *fiber.Ctx) error {
	idStr := c.Params("id")
	id, err := uuid.Parse(idStr)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Invalid role ID",
			"error":   err.Error(),
		})
	}

	role, err := h.service.GetRoleByID(c.Context(), id)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"message": "Role not found",
			"error":   err.Error(),
		})
	}

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"message": "Role retrieved successfully",
		"data":    role,
	})
}

func (h *RoleHandler) GetAllRoles(c *fiber.Ctx) error {
	page := c.QueryInt("page", 1)
	pageSize := c.QueryInt("page_size", 20)
	keyword := c.Query("keyword")
	status := c.Query("status")

	roles, err := h.service.GetAllRoles(c.Context(), page, pageSize, keyword, status)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"message": "Failed to retrieve roles",
			"error":   err.Error(),
		})
	}

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"message": "Roles retrieved successfully",
		"data":    roles,
	})
}

func (h *RoleHandler) UpdateRole(c *fiber.Ctx) error {
	idStr := c.Params("id")
	id, err := uuid.Parse(idStr)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Invalid role ID",
			"error":   err.Error(),
		})
	}

	var req dto.UpdateRoleDTO
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Invalid request body",
			"error":   err.Error(),
		})
	}

	role, err := h.service.UpdateRole(c.Context(), id, req)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Failed to update role",
			"error":   err.Error(),
		})
	}

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"message": "Role updated successfully",
		"data":    role,
	})
}

func (h *RoleHandler) DeleteRole(c *fiber.Ctx) error {
	idStr := c.Params("id")
	id, err := uuid.Parse(idStr)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Invalid role ID",
			"error":   err.Error(),
		})
	}

	hardDelete := c.QueryBool("hard_delete", false)

	if err := h.service.DeleteRole(c.Context(), id, hardDelete); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Failed to delete role",
			"error":   err.Error(),
		})
	}

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"message": "Role deleted successfully",
	})
}

// ============ Role-Permission Management ============

func (h *RoleHandler) AddPermissionsToRole(c *fiber.Ctx) error {
	idStr := c.Params("id")
	roleID, err := uuid.Parse(idStr)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Invalid role ID",
			"error":   err.Error(),
		})
	}

	var req dto.AddPermissionsToRoleDTO
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Invalid request body",
			"error":   err.Error(),
		})
	}

	if err := h.service.AddPermissionsToRole(c.Context(), roleID, req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Failed to add permissions to role",
			"error":   err.Error(),
		})
	}

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"message": "Permissions added to role successfully",
	})
}

func (h *RoleHandler) RemovePermissionsFromRole(c *fiber.Ctx) error {
	idStr := c.Params("id")
	roleID, err := uuid.Parse(idStr)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Invalid role ID",
			"error":   err.Error(),
		})
	}

	var req dto.RemovePermissionsFromRoleDTO
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Invalid request body",
			"error":   err.Error(),
		})
	}

	if err := h.service.RemovePermissionsFromRole(c.Context(), roleID, req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Failed to remove permissions from role",
			"error":   err.Error(),
		})
	}

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"message": "Permissions removed from role successfully",
	})
}

func (h *RoleHandler) SetRolePermissions(c *fiber.Ctx) error {
	idStr := c.Params("id")
	roleID, err := uuid.Parse(idStr)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Invalid role ID",
			"error":   err.Error(),
		})
	}

	var req dto.AddPermissionsToRoleDTO
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Invalid request body",
			"error":   err.Error(),
		})
	}

	if err := h.service.SetRolePermissions(c.Context(), roleID, req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Failed to set role permissions",
			"error":   err.Error(),
		})
	}

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"message": "Role permissions set successfully",
	})
}

func (h *RoleHandler) GetRolePermissions(c *fiber.Ctx) error {
	idStr := c.Params("id")
	roleID, err := uuid.Parse(idStr)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Invalid role ID",
			"error":   err.Error(),
		})
	}

	permissions, err := h.service.GetRolePermissions(c.Context(), roleID)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Failed to get role permissions",
			"error":   err.Error(),
		})
	}

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"message": "Role permissions retrieved successfully",
		"data":    permissions,
	})
}
