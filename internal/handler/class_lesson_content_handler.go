package handler

import (
	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"study.com/v1/internal/dto"
	"study.com/v1/internal/middleware"
	"study.com/v1/internal/service"
)

type ClassLessonContentHandler struct {
	service service.ClassLessonContentServiceInterface
	// permChecker (V3-7, issue #58): xac dinh nguoi goi co phai SYSTEM_ADMIN khong, de admin van
	// xu ly duoc lop co van de ngoai pham vi giao vien lop/instructor khoa. Cung pattern voi
	// ClassHandler/LessonContentHandler. Co the nil (test khong truyen) — isAdminActor fail-closed.
	permChecker *middleware.PermissionChecker
}

func NewClassLessonContentHandler(service service.ClassLessonContentServiceInterface, permChecker *middleware.PermissionChecker) *ClassLessonContentHandler {
	return &ClassLessonContentHandler{service: service, permChecker: permChecker}
}

// clcErrorStatus (V3-7, issue #58): phan loai loi UY QUYEN tu ClassLessonContentService thanh 403
// thay vi 400 mac dinh cua cac handler trong nhom nay — xem service.IsForbiddenErr (mot cho duy
// nhat liet ke sentinel uy quyen cua nhom livestream/class-content).
func clcErrorStatus(err error) int {
	if service.IsForbiddenErr(err) {
		return fiber.StatusForbidden
	}
	return 0
}

// POST /api/lesson-contents/:id/classes
func (h *ClassLessonContentHandler) AssignClassToContent(c *fiber.Ctx) error {
	contentID, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Invalid content ID",
			"error":   err.Error(),
		})
	}

	// V3-7 (issue #58): truoc day `c.Locals("user_id").(uuid.UUID)` — panic (500) neu local la
	// string, va khong co nhanh 401. extractUserID chap nhan ca uuid.UUID lan string.
	userID, err := extractUserID(c)
	if err != nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"message": "Unauthorized",
			"error":   err.Error(),
		})
	}

	var req dto.AssignClassToContentDTO
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Invalid request body",
			"error":   err.Error(),
		})
	}

	isAdmin := isAdminActor(c, h.permChecker, userID)
	result, err := h.service.AssignClassToContent(c.Context(), contentID, userID, isAdmin, req)
	if err != nil {
		if status := clcErrorStatus(err); status != 0 {
			return c.Status(status).JSON(fiber.Map{"message": err.Error()})
		}
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Failed to assign class to content",
			"error":   err.Error(),
		})
	}

	return c.Status(fiber.StatusCreated).JSON(fiber.Map{
		"message": "Class assigned to content successfully",
		"data":    result,
	})
}

// PUT /api/lesson-contents/:id/classes/:class_id
func (h *ClassLessonContentHandler) UpdateClassContentSchedule(c *fiber.Ctx) error {
	contentID, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Invalid content ID",
			"error":   err.Error(),
		})
	}

	classID, err := uuid.Parse(c.Params("class_id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Invalid class ID",
			"error":   err.Error(),
		})
	}

	userID, err := extractUserID(c)
	if err != nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"message": "Unauthorized",
			"error":   err.Error(),
		})
	}

	var req dto.UpdateClassContentScheduleDTO
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Invalid request body",
			"error":   err.Error(),
		})
	}

	isAdmin := isAdminActor(c, h.permChecker, userID)
	result, err := h.service.UpdateClassContentSchedule(c.Context(), contentID, classID, userID, isAdmin, req)
	if err != nil {
		if status := clcErrorStatus(err); status != 0 {
			return c.Status(status).JSON(fiber.Map{"message": err.Error()})
		}
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Failed to update schedule",
			"error":   err.Error(),
		})
	}

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"message": "Schedule updated successfully",
		"data":    result,
	})
}

// DELETE /api/lesson-contents/:id/classes/:class_id
func (h *ClassLessonContentHandler) RemoveClassFromContent(c *fiber.Ctx) error {
	contentID, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Invalid content ID",
			"error":   err.Error(),
		})
	}

	classID, err := uuid.Parse(c.Params("class_id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Invalid class ID",
			"error":   err.Error(),
		})
	}

	userID, err := extractUserID(c)
	if err != nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"message": "Unauthorized",
			"error":   err.Error(),
		})
	}

	isAdmin := isAdminActor(c, h.permChecker, userID)
	if err := h.service.RemoveClassFromContent(c.Context(), contentID, classID, userID, isAdmin); err != nil {
		if status := clcErrorStatus(err); status != 0 {
			return c.Status(status).JSON(fiber.Map{"message": err.Error()})
		}
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Failed to remove class from content",
			"error":   err.Error(),
		})
	}

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"message": "Class removed from content successfully",
	})
}

// GET /api/lesson-contents/:id/classes
func (h *ClassLessonContentHandler) GetClassesForContent(c *fiber.Ctx) error {
	contentID, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Invalid content ID",
			"error":   err.Error(),
		})
	}

	userID, err := extractUserID(c)
	if err != nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"message": "Unauthorized",
			"error":   err.Error(),
		})
	}

	page := c.QueryInt("page", 1)
	pageSize := c.QueryInt("page_size", 20)

	isAdmin := isAdminActor(c, h.permChecker, userID)
	result, err := h.service.GetClassesForContent(c.Context(), contentID, userID, isAdmin, page, pageSize)
	if err != nil {
		if status := clcErrorStatus(err); status != 0 {
			return c.Status(status).JSON(fiber.Map{"message": err.Error()})
		}
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"message": "Failed to get classes for content",
			"error":   err.Error(),
		})
	}

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"message": "Classes retrieved successfully",
		"data":    result,
	})
}

// GET /api/classes/:id/contents
func (h *ClassLessonContentHandler) GetContentScheduleForClass(c *fiber.Ctx) error {
	classID, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Invalid class ID",
			"error":   err.Error(),
		})
	}

	userID, err := extractUserID(c)
	if err != nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"message": "Unauthorized",
			"error":   err.Error(),
		})
	}

	page := c.QueryInt("page", 1)
	pageSize := c.QueryInt("page_size", 20)

	isAdmin := isAdminActor(c, h.permChecker, userID)
	result, err := h.service.GetContentScheduleForClass(c.Context(), classID, userID, isAdmin, page, pageSize)
	if err != nil {
		if status := clcErrorStatus(err); status != 0 {
			return c.Status(status).JSON(fiber.Map{"message": err.Error()})
		}
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"message": "Failed to get content schedule for class",
			"error":   err.Error(),
		})
	}

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"message": "Content schedule retrieved successfully",
		"data":    result,
	})
}

// POST /api/lesson-contents/:id/classes/bulk
func (h *ClassLessonContentHandler) BulkAssignClassesToContent(c *fiber.Ctx) error {
	contentID, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Invalid content ID",
			"error":   err.Error(),
		})
	}

	userID, err := extractUserID(c)
	if err != nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"message": "Unauthorized",
			"error":   err.Error(),
		})
	}

	var req dto.BulkAssignClassesToContentDTO
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Invalid request body",
			"error":   err.Error(),
		})
	}

	isAdmin := isAdminActor(c, h.permChecker, userID)
	result, err := h.service.BulkAssignClassesToContent(c.Context(), contentID, userID, isAdmin, req)
	if err != nil {
		if status := clcErrorStatus(err); status != 0 {
			return c.Status(status).JSON(fiber.Map{"message": err.Error()})
		}
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Failed to bulk assign classes",
			"error":   err.Error(),
		})
	}

	return c.Status(fiber.StatusCreated).JSON(fiber.Map{
		"message": "Classes assigned to content successfully",
		"data":    result,
	})
}
