package handler

import (
	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"study.com/v1/internal/dto"
	"study.com/v1/internal/service"
)

type RolePermissionHandlerInterface interface {
	// Role endpoints
	CreateRole(c *fiber.Ctx) error
	GetRole(c *fiber.Ctx) error
	GetAllRoles(c *fiber.Ctx) error
	UpdateRole(c *fiber.Ctx) error
	DeleteRole(c *fiber.Ctx) error

	// Permission endpoints
	GetPermission(c *fiber.Ctx) error
	GetAllPermissions(c *fiber.Ctx) error
	UpdatePermission(c *fiber.Ctx) error

	// Role-Permission management endpoints
	AddPermissionsToRole(c *fiber.Ctx) error
	RemovePermissionsFromRole(c *fiber.Ctx) error
	SetRolePermissions(c *fiber.Ctx) error
	GetRolePermissions(c *fiber.Ctx) error
}

type RolePermissionHandler struct {
	service service.RolePermissionServiceInterface
}

func NewRolePermissionHandler(service service.RolePermissionServiceInterface) *RolePermissionHandler {
	return &RolePermissionHandler{service: service}
}

// ============ Role Endpoints ============

func (h *RolePermissionHandler) CreateRole(c *fiber.Ctx) error {
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

func (h *RolePermissionHandler) GetRole(c *fiber.Ctx) error {
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

func (h *RolePermissionHandler) GetAllRoles(c *fiber.Ctx) error {
	page := c.QueryInt("page", 1)
	pageSize := c.QueryInt("page_size", 20)

	roles, err := h.service.GetAllRoles(c.Context(), page, pageSize)
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

func (h *RolePermissionHandler) UpdateRole(c *fiber.Ctx) error {
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

func (h *RolePermissionHandler) DeleteRole(c *fiber.Ctx) error {
	idStr := c.Params("id")
	id, err := uuid.Parse(idStr)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Invalid role ID",
			"error":   err.Error(),
		})
	}

	if err := h.service.DeleteRole(c.Context(), id); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Failed to delete role",
			"error":   err.Error(),
		})
	}

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"message": "Role deleted successfully",
	})
}

// ============ Permission Endpoints ============

func (h *RolePermissionHandler) GetPermission(c *fiber.Ctx) error {
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

func (h *RolePermissionHandler) GetAllPermissions(c *fiber.Ctx) error {
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

func (h *RolePermissionHandler) UpdatePermission(c *fiber.Ctx) error {
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

// ============ Role-Permission Management Endpoints ============

func (h *RolePermissionHandler) AddPermissionsToRole(c *fiber.Ctx) error {
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

func (h *RolePermissionHandler) RemovePermissionsFromRole(c *fiber.Ctx) error {
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

func (h *RolePermissionHandler) SetRolePermissions(c *fiber.Ctx) error {
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

func (h *RolePermissionHandler) GetRolePermissions(c *fiber.Ctx) error {
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
