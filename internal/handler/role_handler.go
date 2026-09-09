package handler

import (
	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"study.com/v1/internal/dto"
	"study.com/v1/internal/middleware"
	"study.com/v1/internal/service"
	"study.com/v1/internal/utils"
)

type RoleHandlerInterface interface {
	CreateRole(c *fiber.Ctx) error
	GetRole(c *fiber.Ctx) error
	GetAllRoles(c *fiber.Ctx) error
	UpdateRole(c *fiber.Ctx) error
	DeleteRole(c *fiber.Ctx) error
	RestoreRole(c *fiber.Ctx) error

	// Role-Permission management
	AddPermissionsToRole(c *fiber.Ctx) error
	RemovePermissionsFromRole(c *fiber.Ctx) error
	SetRolePermissions(c *fiber.Ctx) error
	GetRolePermissions(c *fiber.Ctx) error
}

type RoleHandler struct {
	service     service.RoleServiceInterface
	permChecker *middleware.PermissionChecker
}

func NewRoleHandler(service service.RoleServiceInterface, permChecker *middleware.PermissionChecker) *RoleHandler {
	return &RoleHandler{service: service, permChecker: permChecker}
}

// roleErrorStatus ánh xạ lỗi phân quyền (C-03 residual) sang HTTP 403; trả 0 khi không nhận
// diện được để caller giữ nguyên xử lý 400 hiện có.
func roleErrorStatus(err error) int {
	switch err {
	case service.ErrNotRoleOrgMember:
		return fiber.StatusForbidden
	default:
		return 0
	}
}

func (h *RoleHandler) CreateRole(c *fiber.Ctx) error {
	var req dto.CreateRoleDTO
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Invalid request body",
			"error":   err.Error(),
		})
	}

	// M-01 (audit 260909 vòng 2): file này trước đây không gọi ValidateStruct lần nào.
	if errs := utils.ValidateStruct(req); len(errs) > 0 {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Validation failed",
			"errors":  errs,
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
	orgIDStr := c.Query("organization_id")

	var orgID *uuid.UUID
	if orgIDStr != "" {
		parsed, err := uuid.Parse(orgIDStr)
		if err != nil {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
				"message": "Invalid organization_id",
				"error":   err.Error(),
			})
		}
		orgID = &parsed
	}

	roles, err := h.service.GetAllRoles(c.Context(), page, pageSize, keyword, status, orgID)
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

	// M-01 (audit 260909 vòng 2): xem ghi chú ở CreateRole.
	if errs := utils.ValidateStruct(req); len(errs) > 0 {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Validation failed",
			"errors":  errs,
		})
	}

	// C-03 residual: :id là Role.ID, không phải Organization.ID nên middleware router chưa
	// đối chiếu được — kiểm tra role.OrganizationID khớp active_org_id ở tầng service.
	userID, ok := c.Locals("user_id").(uuid.UUID)
	if !ok {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"message": "Unauthorized",
		})
	}
	var activeOrgID *uuid.UUID
	if orgID, ok := c.Locals("active_org_id").(uuid.UUID); ok {
		activeOrgID = &orgID
	}
	isAdmin := isAdminActor(c, h.permChecker, userID)

	role, err := h.service.UpdateRole(c.Context(), id, activeOrgID, isAdmin, req)
	if err != nil {
		if status := roleErrorStatus(err); status != 0 {
			return c.Status(status).JSON(fiber.Map{"message": err.Error()})
		}
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

	// C-03 residual: xem ghi chú ở UpdateRole.
	userID, ok := c.Locals("user_id").(uuid.UUID)
	if !ok {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"message": "Unauthorized",
		})
	}
	var activeOrgID *uuid.UUID
	if orgID, ok := c.Locals("active_org_id").(uuid.UUID); ok {
		activeOrgID = &orgID
	}
	isAdmin := isAdminActor(c, h.permChecker, userID)

	if err := h.service.DeleteRole(c.Context(), id, activeOrgID, isAdmin, hardDelete); err != nil {
		if status := roleErrorStatus(err); status != 0 {
			return c.Status(status).JSON(fiber.Map{"message": err.Error()})
		}
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Failed to delete role",
			"error":   err.Error(),
		})
	}

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"message": "Role deleted successfully",
	})
}

func (h *RoleHandler) RestoreRole(c *fiber.Ctx) error {
	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Invalid role ID",
			"error":   err.Error(),
		})
	}

	if err := h.service.RestoreRole(c.Context(), id); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Failed to restore role",
			"error":   err.Error(),
		})
	}

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"message": "Role restored successfully",
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

	// M-01 (audit 260909 vòng 2): xem ghi chú ở CreateRole.
	if errs := utils.ValidateStruct(req); len(errs) > 0 {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Validation failed",
			"errors":  errs,
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

	// M-01 (audit 260909 vòng 2): xem ghi chú ở CreateRole.
	if errs := utils.ValidateStruct(req); len(errs) > 0 {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Validation failed",
			"errors":  errs,
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

	// M-01 (audit 260909 vòng 2): xem ghi chú ở CreateRole.
	if errs := utils.ValidateStruct(req); len(errs) > 0 {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Validation failed",
			"errors":  errs,
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
