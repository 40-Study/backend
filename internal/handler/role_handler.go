package handler

import (
	"errors"

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

// roleErrorStatus ánh xạ lỗi phân quyền (C-03 residual, S6) sang HTTP 403; trả 0 khi không nhận
// diện được để caller giữ nguyên xử lý mặc định. errors.Is vì ErrPermissionNotOrgScope được bọc
// kèm tên permission.
func roleErrorStatus(err error) int {
	switch {
	case errors.Is(err, service.ErrNotRoleOrgMember), errors.Is(err, service.ErrPermissionNotOrgScope):
		return fiber.StatusForbidden
	case err != nil && err.Error() == "role not found":
		return fiber.StatusNotFound
	default:
		return 0
	}
}

// roleActor gom (user, active_org_id, isAdmin) của người gọi cho mọi route /org-roles/* (S6):
// tổ chức của role phải khớp tổ chức đang active của người gọi, trừ admin (SYSTEM_SETTINGS_MANAGE).
// ok=false nghĩa là chưa đăng nhập (đã trả 401).
func (h *RoleHandler) roleActor(c *fiber.Ctx) (activeOrgID *uuid.UUID, isAdmin bool, ok bool) {
	userID, isUUID := c.Locals("user_id").(uuid.UUID)
	if !isUUID || userID == uuid.Nil {
		_ = c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"message": "Unauthorized"})
		return nil, false, false
	}
	if orgID, has := c.Locals("active_org_id").(uuid.UUID); has {
		activeOrgID = &orgID
	}
	return activeOrgID, isAdminActor(c, h.permChecker, userID), true
}

// respondRoleError trả 403/404 cho lỗi phân quyền/không tìm thấy, còn lại dùng fallback (400/500 cũ).
func respondRoleError(c *fiber.Ctx, err error, fallbackStatus int, message string) error {
	if status := roleErrorStatus(err); status != 0 {
		return c.Status(status).JSON(fiber.Map{"message": err.Error()})
	}
	return c.Status(fallbackStatus).JSON(fiber.Map{"message": message, "error": err.Error()})
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

	activeOrgID, isAdmin, ok := h.roleActor(c)
	if !ok {
		return nil
	}

	role, err := h.service.CreateRole(c.Context(), activeOrgID, isAdmin, req)
	if err != nil {
		return respondRoleError(c, err, fiber.StatusBadRequest, "Failed to create role")
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

	activeOrgID, isAdmin, ok := h.roleActor(c)
	if !ok {
		return nil
	}

	role, err := h.service.GetRoleByID(c.Context(), id, activeOrgID, isAdmin)
	if err != nil {
		return respondRoleError(c, err, fiber.StatusNotFound, "Role not found")
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

	activeOrgID, isAdmin, ok := h.roleActor(c)
	if !ok {
		return nil
	}

	roles, err := h.service.GetAllRoles(c.Context(), page, pageSize, keyword, status, orgID, activeOrgID, isAdmin)
	if err != nil {
		return respondRoleError(c, err, fiber.StatusInternalServerError, "Failed to retrieve roles")
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
	activeOrgID, isAdmin, ok := h.roleActor(c)
	if !ok {
		return nil
	}

	role, err := h.service.UpdateRole(c.Context(), id, activeOrgID, isAdmin, req)
	if err != nil {
		return respondRoleError(c, err, fiber.StatusBadRequest, "Failed to update role")
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
	activeOrgID, isAdmin, ok := h.roleActor(c)
	if !ok {
		return nil
	}

	if err := h.service.DeleteRole(c.Context(), id, activeOrgID, isAdmin, hardDelete); err != nil {
		return respondRoleError(c, err, fiber.StatusBadRequest, "Failed to delete role")
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

	activeOrgID, isAdmin, ok := h.roleActor(c)
	if !ok {
		return nil
	}

	if err := h.service.RestoreRole(c.Context(), id, activeOrgID, isAdmin); err != nil {
		return respondRoleError(c, err, fiber.StatusBadRequest, "Failed to restore role")
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

	activeOrgID, isAdmin, ok := h.roleActor(c)
	if !ok {
		return nil
	}

	if err := h.service.AddPermissionsToRole(c.Context(), roleID, activeOrgID, isAdmin, req); err != nil {
		return respondRoleError(c, err, fiber.StatusBadRequest, "Failed to add permissions to role")
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

	activeOrgID, isAdmin, ok := h.roleActor(c)
	if !ok {
		return nil
	}

	if err := h.service.RemovePermissionsFromRole(c.Context(), roleID, activeOrgID, isAdmin, req); err != nil {
		return respondRoleError(c, err, fiber.StatusBadRequest, "Failed to remove permissions from role")
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

	activeOrgID, isAdmin, ok := h.roleActor(c)
	if !ok {
		return nil
	}

	if err := h.service.SetRolePermissions(c.Context(), roleID, activeOrgID, isAdmin, req); err != nil {
		return respondRoleError(c, err, fiber.StatusBadRequest, "Failed to set role permissions")
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

	activeOrgID, isAdmin, ok := h.roleActor(c)
	if !ok {
		return nil
	}

	permissions, err := h.service.GetRolePermissions(c.Context(), roleID, activeOrgID, isAdmin)
	if err != nil {
		return respondRoleError(c, err, fiber.StatusBadRequest, "Failed to get role permissions")
	}

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"message": "Role permissions retrieved successfully",
		"data":    permissions,
	})
}
