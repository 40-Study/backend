package handler

import (
	"errors"
	"log"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"study.com/v1/internal/dto"
	"study.com/v1/internal/middleware"
	"study.com/v1/internal/model"
	"study.com/v1/internal/repository"
	"study.com/v1/internal/service"
)

// Phase 1 quản lý người dùng (2026-09-28) — contract:
// plans/260927-2055-role-based-ux-qa/admin-features/phase-01-user-management.md

type UserAdminHandlerInterface interface {
	GetUsers(c *fiber.Ctx) error
	GetUser(c *fiber.Ctx) error
	UpdateUserStatus(c *fiber.Ctx) error
}

type UserAdminHandler struct {
	userAdminService service.UserAdminServiceInterface
}

func NewUserAdminHandler(userAdminService service.UserAdminServiceInterface) *UserAdminHandler {
	return &UserAdminHandler{userAdminService: userAdminService}
}

// GetUsers — GET /api/users?keyword=&role=&status=&page=&limit=
func (h *UserAdminHandler) GetUsers(c *fiber.Ctx) error {
	status := c.Query("status", "")
	if status != "" && status != "active" && status != "locked" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Invalid query params",
			"errors":  []string{"status must be 'active' or 'locked'"},
		})
	}

	filter := repository.AdminUserListFilter{
		Keyword: c.Query("keyword", ""),
		Role:    c.Query("role", ""),
		Status:  status,
		Page:    c.QueryInt("page", 1),
		Limit:   c.QueryInt("limit", 20),
	}

	result, err := h.userAdminService.ListUsers(c.Context(), filter)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"message": "Failed to list users",
			"error":   err.Error(),
		})
	}

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"message": "Success",
		"data":    result,
	})
}

// GetUser — GET /api/users/:id
func (h *UserAdminHandler) GetUser(c *fiber.Ctx) error {
	userID, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Invalid user ID format",
		})
	}

	result, err := h.userAdminService.GetUserDetail(c.Context(), userID)
	if err != nil {
		if errors.Is(err, repository.ErrAdminUserNotFound) {
			return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
				"message": "User not found",
			})
		}
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"message": "Failed to get user",
			"error":   err.Error(),
		})
	}

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"message": "Success",
		"data":    result,
	})
}

// UpdateUserStatus — PUT /api/users/:id/status
func (h *UserAdminHandler) UpdateUserStatus(c *fiber.Ctx) error {
	userID, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Invalid user ID format",
		})
	}

	var req dto.UpdateUserStatusRequestDTO
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Invalid request body",
			"error":   err.Error(),
		})
	}

	actorIDRaw := c.Locals("user_id")
	actorID, ok := actorIDRaw.(uuid.UUID)
	if !ok || actorID == uuid.Nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"message": "Unauthorized",
		})
	}

	// ADM-N3: mở khoá xoá locked_reason/locked_at khỏi users, nên đọc lý do cũ TRƯỚC để ghi vào nhật ký. Chỉ để ghi
	// nhật ký: đọc lỗi thì mở khoá vẫn tiếp tục và dòng nhật ký không có previous_reason (không bịa), có log cảnh báo.
	var previous *dto.AdminUserDetailDTO
	if req.IsActive {
		if previous, err = h.userAdminService.GetUserDetail(c.Context(), userID); err != nil {
			log.Printf("[Audit] không đọc được lý do khoá cũ của %s trước khi mở khoá: %v", userID, err)
			previous = nil
		}
	}

	result, err := h.userAdminService.UpdateUserStatus(c.Context(), userID, actorID, req.IsActive, req.Reason)
	if err != nil {
		switch {
		case errors.Is(err, repository.ErrAdminUserNotFound):
			return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
				"message": "User not found",
			})
		case errors.Is(err, service.ErrCannotLockSelf):
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
				"message": "Cannot lock your own account",
			})
		case errors.Is(err, service.ErrLockReasonRequired):
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
				"message": "Reason is required when locking an account",
				"errors":  []string{"reason is required"},
			})
		case errors.Is(err, repository.ErrLastSystemAdmin):
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
				"message": "Cannot lock the last active system admin",
			})
		default:
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
				"message": "Failed to update user status",
				"error":   err.Error(),
			})
		}
	}

	message := "Account locked"
	if req.IsActive {
		message = "Account unlocked"
		// Cùng một route phục vụ khoá và mở khoá: middleware Audit mặc định ghi user.lock.
		middleware.SetAuditAction(c, model.AuditActionUserUnlock)
		if previous != nil && previous.LockedReason != nil {
			middleware.SetAuditMeta(c, map[string]any{"previous_reason": *previous.LockedReason})
			if previous.LockedAt != nil {
				middleware.SetAuditMeta(c, map[string]any{"previous_locked_at": previous.LockedAt.Format(time.RFC3339)})
			}
		}
	} else if req.Reason != nil {
		// Lý do khoá là bắt buộc (service đã chặn rỗng); nhật ký là nơi duy nhất còn giữ nó sau khi mở khoá.
		middleware.SetAuditMeta(c, map[string]any{"reason": strings.TrimSpace(*req.Reason)})
	}

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"message": message,
		"data":    result,
	})
}
