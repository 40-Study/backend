package handler

import (
	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"study.com/v1/internal/dto"
	"study.com/v1/internal/middleware"
	"study.com/v1/internal/service"
	"study.com/v1/internal/utils"
)

type SectionHandler struct {
	service     service.SectionServiceInterface
	permChecker *middleware.PermissionChecker
}

func NewSectionHandler(service service.SectionServiceInterface, permChecker *middleware.PermissionChecker) *SectionHandler {
	return &SectionHandler{service: service, permChecker: permChecker}
}

// sectionForbiddenResponse ánh xạ ErrNotSectionCourseOwner (C-12) sang HTTP 403; trả false
// khi lỗi không phải lỗi quyền để handler tiếp tục xử lý theo nhánh 400 hiện có.
func sectionForbiddenResponse(c *fiber.Ctx, err error) bool {
	if err == service.ErrNotSectionCourseOwner {
		_ = c.Status(fiber.StatusForbidden).JSON(fiber.Map{
			"message": "You are not the instructor of this course",
		})
		return true
	}
	return false
}

func (h *SectionHandler) CreateSection(c *fiber.Ctx) error {
	courseID, err := uuid.Parse(c.Params("course_id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Invalid course ID",
			"error":   err.Error(),
		})
	}

	userID, ok := c.Locals("user_id").(uuid.UUID)
	if !ok {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"message": "Unauthorized",
		})
	}

	var req dto.CreateSectionDTO
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Invalid request body",
			"error":   err.Error(),
		})
	}

	if errors := utils.ValidateStruct(req); len(errors) > 0 {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Validation failed",
			"errors":  errors,
		})
	}

	section, err := h.service.CreateSection(c.Context(), courseID, userID, req)
	if err != nil {
		if sectionForbiddenResponse(c, err) {
			return nil
		}
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Failed to create section",
			"error":   err.Error(),
		})
	}

	return c.Status(fiber.StatusCreated).JSON(fiber.Map{
		"message": "Section created successfully",
		"data":    section,
	})
}

func (h *SectionHandler) GetAllSections(c *fiber.Ctx) error {

	courseID, err := uuid.Parse(c.Params("course_id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Invalid course ID",
			"error":   err.Error(),
		})
	}

	// Phase 1 §2: userID quyet dinh locked/lock_reason/progress cua tung bai. Route nay nam
	// sau middleware.AuthMiddleware (course_router.go) nen userID luon co mat tren duong that.
	userID, ok := c.Locals("user_id").(uuid.UUID)
	if !ok {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"message": "Unauthorized",
		})
	}

	sections, err := h.service.GetAllSections(c.Context(), courseID, userID)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"message": "Failed to retrieve sections",
			"error":   err.Error(),
		})
	}

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"message": "Sections retrieved successfully",
		"data":    sections,
	})
}

func (h *SectionHandler) GetSectionByID(c *fiber.Ctx) error {
	sectionID, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Invalid section ID",
			"error":   err.Error(),
		})
	}

	section, err := h.service.GetSectionByID(c.Context(), sectionID)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"message": "Section not found",
			"error":   err.Error(),
		})
	}

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"message": "Section retrieved successfully",
		"data":    section,
	})
}

func (h *SectionHandler) UpdateSection(c *fiber.Ctx) error {
	courseID, err := uuid.Parse(c.Params("course_id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Invalid course ID",
			"error":   err.Error(),
		})
	}

	sectionID, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Invalid section ID",
			"error":   err.Error(),
		})
	}

	userID, ok := c.Locals("user_id").(uuid.UUID)
	if !ok {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"message": "Unauthorized",
		})
	}

	var req dto.UpdateSectionDTO
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Invalid request body",
			"error":   err.Error(),
		})
	}

	if errors := utils.ValidateStruct(req); len(errors) > 0 {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Validation failed",
			"errors":  errors,
		})
	}

	isAdmin := isAdminActor(c, h.permChecker, userID)
	section, err := h.service.UpdateSection(c.Context(), courseID, sectionID, userID, isAdmin, req)
	if err != nil {
		if sectionForbiddenResponse(c, err) {
			return nil
		}
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Failed to update section",
			"error":   err.Error(),
		})
	}

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"message": "Section updated successfully",
		"data":    section,
	})
}

func (h *SectionHandler) DeleteSection(c *fiber.Ctx) error {
	courseID, err := uuid.Parse(c.Params("course_id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Invalid course ID",
			"error":   err.Error(),
		})
	}

	sectionID, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Invalid section ID",
			"error":   err.Error(),
		})
	}

	userID, ok := c.Locals("user_id").(uuid.UUID)
	if !ok {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"message": "Unauthorized",
		})
	}

	isAdmin := isAdminActor(c, h.permChecker, userID)
	if err := h.service.DeleteSection(c.Context(), courseID, sectionID, userID, isAdmin); err != nil {
		if sectionForbiddenResponse(c, err) {
			return nil
		}
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Failed to delete section",
			"error":   err.Error(),
		})
	}

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"message": "Section deleted successfully",
	})
}

func (h *SectionHandler) ReorderSections(c *fiber.Ctx) error {
	courseID, err := uuid.Parse(c.Params("course_id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Invalid course ID",
			"error":   err.Error(),
		})
	}

	var req dto.ReorderDTO
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Invalid request body",
			"error":   err.Error(),
		})
	}

	if errors := utils.ValidateStruct(req); len(errors) > 0 {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Validation failed",
			"errors":  errors,
		})
	}

	if err := h.service.ReorderSections(c.Context(), courseID, req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Failed to reorder sections",
			"error":   err.Error(),
		})
	}

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"message": "Sections reordered successfully",
	})
}
