package handler

import (
	"strconv"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"study.com/v1/internal/service"
)

type ProfileHandlerInterface interface {
	GetChildren(c *fiber.Ctx) error
	GetOrganizations(c *fiber.Ctx) error
	GetMyOrgRoles(c *fiber.Ctx) error
}

type ProfileHandler struct {
	profileService     service.ProfileServiceInterface
	userOrgRoleService service.UserOrganizationRoleServiceInterface
}

func NewProfileHandler(profileService service.ProfileServiceInterface, userOrgRoleService service.UserOrganizationRoleServiceInterface) *ProfileHandler {
	return &ProfileHandler{
		profileService:     profileService,
		userOrgRoleService: userOrgRoleService,
	}
}

// GetChildren lấy danh sách con của phụ huynh
func (h *ProfileHandler) GetChildren(c *fiber.Ctx) error {
	userID, ok := c.Locals("user_id").(uuid.UUID)
	if !ok || userID == uuid.Nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"message": "Unauthorized",
		})
	}

	page, _ := strconv.Atoi(c.Query("page", "1"))
	pageSize, _ := strconv.Atoi(c.Query("pageSize", "20"))

	result, err := h.profileService.GetChildren(c.Context(), userID, page, pageSize)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"message": "Failed to get children",
			"error":   err.Error(),
		})
	}

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"message": "Success",
		"data":    result,
	})
}

// GetOrganizations lấy danh sách tổ chức của user
func (h *ProfileHandler) GetOrganizations(c *fiber.Ctx) error {
	userID, ok := c.Locals("user_id").(uuid.UUID)
	if !ok || userID == uuid.Nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"message": "Unauthorized",
		})
	}

	page, _ := strconv.Atoi(c.Query("page", "1"))
	pageSize, _ := strconv.Atoi(c.Query("pageSize", "20"))

	result, err := h.profileService.GetOrganizations(c.Context(), userID, page, pageSize)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"message": "Failed to get organizations",
			"error":   err.Error(),
		})
	}

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"message": "Success",
		"data":    result,
	})
}

func (h *ProfileHandler) GetMyOrgRoles(c *fiber.Ctx) error {
	userID, ok := c.Locals("user_id").(uuid.UUID)
	if !ok || userID == uuid.Nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"message": "Unauthorized",
		})
	}

	var orgID *uuid.UUID
	if orgIDStr := c.Query("org_id"); orgIDStr != "" {
		parsed, err := uuid.Parse(orgIDStr)
		if err != nil {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
				"message": "Invalid organization ID format",
			})
		}
		orgID = &parsed
	}

	roles, err := h.userOrgRoleService.GetMyOrgRoles(c.Context(), userID, orgID)
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
