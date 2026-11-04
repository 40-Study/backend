package handler

import (
	"errors"
	"strings"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"study.com/v1/internal/dto"
	"study.com/v1/internal/middleware"
	"study.com/v1/internal/service"
	"study.com/v1/internal/utils"
)

// systemRoleInvalid trả 400 với thông báo tiếng Việt (hiện thẳng cho quản trị viên) + chi tiết theo trường.
func systemRoleInvalid(c *fiber.Ctx, errs []utils.ValidationError) error {
	message := "Dữ liệu vai trò không hợp lệ"
	for _, e := range errs {
		switch e.Field {
		case "name":
			message = "Tên vai trò phải có từ 2 đến 100 ký tự và không được để trống"
		case "description":
			message = "Mô tả vai trò tối đa 500 ký tự"
		default:
			continue
		}
		break
	}
	return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
		"message": message,
		"code":    "ERR_VALIDATION",
		"errors":  errs,
	})
}

type SystemRoleHandlerInterface interface {
	CreateSystemRole(c *fiber.Ctx) error
	GetSystemRole(c *fiber.Ctx) error
	GetAllSystemRoles(c *fiber.Ctx) error
	UpdateSystemRole(c *fiber.Ctx) error
	DeleteSystemRole(c *fiber.Ctx) error
	RestoreSystemRole(c *fiber.Ctx) error

	AddPermissionsToSystemRole(c *fiber.Ctx) error
	RemovePermissionsFromSystemRole(c *fiber.Ctx) error
	SetSystemRolePermissions(c *fiber.Ctx) error
	GetSystemRolePermissions(c *fiber.Ctx) error
}

type SystemRoleHandler struct {
	service service.SystemRoleServiceInterface
}

func NewSystemRoleHandler(service service.SystemRoleServiceInterface) *SystemRoleHandler {
	return &SystemRoleHandler{service: service}
}

func (h *SystemRoleHandler) CreateSystemRole(c *fiber.Ctx) error {
	var req dto.CreateSystemRoleDTO
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Invalid request body",
			"error":   err.Error(),
		})
	}

	// QA3-03: handler không từng gọi ValidateStruct nên tạo được vai trò tên rỗng/"   "/1 ký tự (tag min=2 của DTO vô
	// tác dụng). Trim TRƯỚC khi validate: "   " có 3 ký tự, qua được min=2 nếu không trim (cùng cách organization_handler).
	req.Name = strings.TrimSpace(req.Name)
	req.Description = strings.TrimSpace(req.Description)
	if errs := utils.ValidateStruct(req); len(errs) > 0 {
		return systemRoleInvalid(c, errs)
	}

	role, err := h.service.CreateSystemRole(c.Context(), req)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Failed to create system role",
			"error":   err.Error(),
		})
	}

	// Id nằm trong response chứ không ở URL: đặt đích cho middleware Audit.
	middleware.SetAuditTarget(c, role.ID.String())

	return c.Status(fiber.StatusCreated).JSON(fiber.Map{
		"message": "System role created successfully",
		"data":    role,
	})
}

func (h *SystemRoleHandler) GetSystemRole(c *fiber.Ctx) error {
	idStr := c.Params("id")
	id, err := uuid.Parse(idStr)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Invalid system role ID",
			"error":   err.Error(),
		})
	}

	role, err := h.service.GetSystemRoleByID(c.Context(), id)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"message": "System role not found",
			"error":   err.Error(),
		})
	}

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"message": "System role retrieved successfully",
		"data":    role,
	})
}

func (h *SystemRoleHandler) GetAllSystemRoles(c *fiber.Ctx) error {
	page := c.QueryInt("page", 1)
	pageSize := c.QueryInt("page_size", 20)
	keyword := c.Query("keyword")
	status := c.Query("status")

	roles, err := h.service.GetAllSystemRoles(c.Context(), page, pageSize, keyword, status)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"message": "Failed to retrieve system roles",
			"error":   err.Error(),
		})
	}

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"message": "System roles retrieved successfully",
		"data":    roles,
	})
}

func (h *SystemRoleHandler) UpdateSystemRole(c *fiber.Ctx) error {
	idStr := c.Params("id")
	id, err := uuid.Parse(idStr)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Invalid system role ID",
			"error":   err.Error(),
		})
	}

	var req dto.UpdateSystemRoleDTO
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Invalid request body",
			"error":   err.Error(),
		})
	}

	// QA3-03: như CreateSystemRole. Tên gửi lên (kể cả rỗng) vẫn phải qua luật tên của DTO tạo (SSOT): tag `omitempty`
	// của con trỏ bỏ qua chuỗi rỗng, nên "" hoặc "   " sẽ lọt nếu chỉ validate UpdateSystemRoleDTO.
	if req.Name != nil {
		trimmed := strings.TrimSpace(*req.Name)
		req.Name = &trimmed
		if errs := utils.ValidateStruct(dto.CreateSystemRoleDTO{Name: trimmed}); len(errs) > 0 {
			return systemRoleInvalid(c, errs)
		}
	}
	if req.Description != nil {
		trimmed := strings.TrimSpace(*req.Description)
		req.Description = &trimmed
	}
	if errs := utils.ValidateStruct(req); len(errs) > 0 {
		return systemRoleInvalid(c, errs)
	}

	role, err := h.service.UpdateSystemRole(c.Context(), id, req)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Failed to update system role",
			"error":   err.Error(),
		})
	}

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"message": "System role updated successfully",
		"data":    role,
	})
}

func (h *SystemRoleHandler) DeleteSystemRole(c *fiber.Ctx) error {
	idStr := c.Params("id")
	id, err := uuid.Parse(idStr)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Invalid system role ID",
			"error":   err.Error(),
		})
	}

	hardDelete := c.QueryBool("hard_delete", false)

	if err := h.service.DeleteSystemRole(c.Context(), id, hardDelete); err != nil {
		if errors.Is(err, service.ErrSystemRoleProtected) {
			return c.Status(fiber.StatusConflict).JSON(fiber.Map{
				"message": "Không thể xoá vai trò hệ thống dựng sẵn",
				"error":   err.Error(),
				"code":    "SYSTEM_ROLE_PROTECTED",
			})
		}
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Failed to delete system role",
			"error":   err.Error(),
		})
	}

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"message": "System role deleted successfully",
	})
}

func (h *SystemRoleHandler) RestoreSystemRole(c *fiber.Ctx) error {
	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Invalid system role ID",
			"error":   err.Error(),
		})
	}

	if err := h.service.RestoreSystemRole(c.Context(), id); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Failed to restore system role",
			"error":   err.Error(),
		})
	}

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"message": "System role restored successfully",
	})
}

// ============ SystemRole-Permission Management ============

func (h *SystemRoleHandler) AddPermissionsToSystemRole(c *fiber.Ctx) error {
	idStr := c.Params("id")
	roleID, err := uuid.Parse(idStr)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Invalid system role ID",
			"error":   err.Error(),
		})
	}

	var req dto.AddPermissionsToSystemRoleDTO
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Invalid request body",
			"error":   err.Error(),
		})
	}

	if err := h.service.AddPermissionsToSystemRole(c.Context(), roleID, req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Failed to add permissions to system role",
			"error":   err.Error(),
		})
	}

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"message": "Permissions added to system role successfully",
	})
}

func (h *SystemRoleHandler) RemovePermissionsFromSystemRole(c *fiber.Ctx) error {
	idStr := c.Params("id")
	roleID, err := uuid.Parse(idStr)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Invalid system role ID",
			"error":   err.Error(),
		})
	}

	var req dto.RemovePermissionsFromSystemRoleDTO
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Invalid request body",
			"error":   err.Error(),
		})
	}

	if err := h.service.RemovePermissionsFromSystemRole(c.Context(), roleID, req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Failed to remove permissions from system role",
			"error":   err.Error(),
		})
	}

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"message": "Permissions removed from system role successfully",
	})
}

func (h *SystemRoleHandler) SetSystemRolePermissions(c *fiber.Ctx) error {
	idStr := c.Params("id")
	roleID, err := uuid.Parse(idStr)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Invalid system role ID",
			"error":   err.Error(),
		})
	}

	var req dto.AddPermissionsToSystemRoleDTO
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Invalid request body",
			"error":   err.Error(),
		})
	}

	if err := h.service.SetSystemRolePermissions(c.Context(), roleID, req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Failed to set system role permissions",
			"error":   err.Error(),
		})
	}

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"message": "System role permissions set successfully",
	})
}

func (h *SystemRoleHandler) GetSystemRolePermissions(c *fiber.Ctx) error {
	idStr := c.Params("id")
	roleID, err := uuid.Parse(idStr)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Invalid system role ID",
			"error":   err.Error(),
		})
	}

	permissions, err := h.service.GetSystemRolePermissions(c.Context(), roleID)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Failed to get system role permissions",
			"error":   err.Error(),
		})
	}

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"message": "System role permissions retrieved successfully",
		"data":    permissions,
	})
}
