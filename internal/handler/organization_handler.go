package handler

import (
	"strings"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"study.com/v1/internal/dto"
	"study.com/v1/internal/service"
	"study.com/v1/internal/utils"
)

type OrganizationHandlerInterface interface {
	CreateOrganization(c *fiber.Ctx) error
	GetOrganization(c *fiber.Ctx) error
	GetAllOrganizations(c *fiber.Ctx) error
	UpdateOrganization(c *fiber.Ctx) error
	DeleteOrganization(c *fiber.Ctx) error
}

type OrganizationHandler struct {
	service service.OrganizationServiceInterface
}

func NewOrganizationHandler(service service.OrganizationServiceInterface) *OrganizationHandler {
	return &OrganizationHandler{service: service}
}

func (h *OrganizationHandler) CreateOrganization(c *fiber.Ctx) error {
	var req dto.CreateOrganizationDTO
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Invalid request body",
			"error":   err.Error(),
		})
	}

	// QA vòng 2 (G2, N-01): trước đây handler không gọi ValidateStruct và DTO dùng tag `binding:`
	// (của Gin, validator không đọc) nên tạo được tổ chức tên rỗng/"   ". Trim TRƯỚC khi validate:
	// "   " có 3 ký tự, qua được min=2 nếu không trim.
	req.Name = strings.TrimSpace(req.Name)
	req.Description = strings.TrimSpace(req.Description)
	if errs := utils.ValidateStruct(req); len(errs) > 0 {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Validation failed",
			"errors":  errs,
		})
	}

	creatorUserID, ok := c.Locals("user_id").(uuid.UUID)
	if !ok || creatorUserID == uuid.Nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"message": "Unauthorized",
		})
	}

	org, err := h.service.CreateOrganization(c.Context(), creatorUserID, req)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Failed to create organization",
			"error":   err.Error(),
		})
	}

	return c.Status(fiber.StatusCreated).JSON(fiber.Map{
		"message": "Organization created successfully",
		"data":    org,
	})
}

func (h *OrganizationHandler) GetOrganization(c *fiber.Ctx) error {
	idStr := c.Params("id")
	id, err := uuid.Parse(idStr)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Invalid organization ID",
			"error":   err.Error(),
		})
	}

	org, err := h.service.GetOrganizationByID(c.Context(), id)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"message": "Organization not found",
			"error":   err.Error(),
		})
	}

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"message": "Organization retrieved successfully",
		"data":    org,
	})
}

func (h *OrganizationHandler) GetAllOrganizations(c *fiber.Ctx) error {
	page := c.QueryInt("page", 1)
	pageSize := c.QueryInt("page_size", 20)
	keyword := c.Query("keyword")
	status := c.Query("status")

	orgs, err := h.service.GetAllOrganizations(c.Context(), page, pageSize, keyword, status)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"message": "Failed to retrieve organizations",
			"error":   err.Error(),
		})
	}

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"message": "Organizations retrieved successfully",
		"data":    orgs,
	})
}

func (h *OrganizationHandler) UpdateOrganization(c *fiber.Ctx) error {
	idStr := c.Params("id")
	id, err := uuid.Parse(idStr)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Invalid organization ID",
			"error":   err.Error(),
		})
	}

	var req dto.UpdateOrganizationDTO
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Invalid request body",
			"error":   err.Error(),
		})
	}

	// G2: như CreateOrganization — field con trỏ nên chỉ trim khi client có gửi.
	if req.Name != nil {
		trimmed := strings.TrimSpace(*req.Name)
		req.Name = &trimmed
	}
	if req.Description != nil {
		trimmed := strings.TrimSpace(*req.Description)
		req.Description = &trimmed
	}
	if errs := utils.ValidateStruct(req); len(errs) > 0 {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Validation failed",
			"errors":  errs,
		})
	}

	org, err := h.service.UpdateOrganization(c.Context(), id, req)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Failed to update organization",
			"error":   err.Error(),
		})
	}

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"message": "Organization updated successfully",
		"data":    org,
	})
}

func (h *OrganizationHandler) DeleteOrganization(c *fiber.Ctx) error {
	idStr := c.Params("id")
	id, err := uuid.Parse(idStr)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Invalid organization ID",
			"error":   err.Error(),
		})
	}

	hardDelete := c.QueryBool("hard_delete", false)

	if err := h.service.DeleteOrganization(c.Context(), id, hardDelete); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Failed to delete organization",
			"error":   err.Error(),
		})
	}

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"message": "Organization deleted successfully",
	})
}
