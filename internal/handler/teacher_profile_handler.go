package handler

import (
	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"study.com/v1/internal/dto"
	"study.com/v1/internal/service"
)

type TeacherProfileHandlerInterface interface {
	CreateTeacherProfile(c *fiber.Ctx) error
	GetAllTeacherProfiles(c *fiber.Ctx) error
	GetTeacherProfileByID(c *fiber.Ctx) error
	UpdateTeacherProfile(c *fiber.Ctx) error
	DeleteTeacherProfile(c *fiber.Ctx) error
}

type TeacherProfileHandler struct {
	service service.TeacherProfileServiceInterface
}

func NewTeacherProfileHandler(service service.TeacherProfileServiceInterface) *TeacherProfileHandler {
	return &TeacherProfileHandler{service: service}
}

// teacherProfileForbiddenResponse ánh xạ ErrNotTeacherProfileOwner (C-05) sang HTTP 403.
func teacherProfileForbiddenResponse(c *fiber.Ctx, err error) bool {
	if err == service.ErrNotTeacherProfileOwner {
		_ = c.Status(fiber.StatusForbidden).JSON(fiber.Map{
			"message": "You are not the owner of this teacher profile",
		})
		return true
	}
	return false
}

func (h *TeacherProfileHandler) CreateTeacherProfile(c *fiber.Ctx) error {
	var req dto.CreateTeacherProfileDTO
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Invalid request body",
			"error":   err.Error(),
		})
	}

	// C-05 (audit 260909): không tin user_id từ body — ép theo user đang đăng nhập.
	userID, ok := c.Locals("user_id").(uuid.UUID)
	if !ok {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"message": "Unauthorized",
		})
	}
	req.UserID = userID

	profile, err := h.service.CreateTeacherProfile(c.Context(), req)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Failed to create teacher profile",
			"error":   err.Error(),
		})
	}

	return c.Status(fiber.StatusCreated).JSON(fiber.Map{
		"message": "Teacher profile created successfully",
		"data":    profile,
	})
}

func (h *TeacherProfileHandler) GetAllTeacherProfiles(c *fiber.Ctx) error {
	page := c.QueryInt("page", 1)
	pageSize := c.QueryInt("page_size", 20)
	keyword := c.Query("keyword")
	status := c.Query("status")

	profiles, err := h.service.GetAllTeacherProfiles(c.Context(), page, pageSize, keyword, status)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"message": "Failed to retrieve teacher profiles",
			"error":   err.Error(),
		})
	}

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"message": "Teacher profiles retrieved successfully",
		"data":    profiles,
	})
}

func (h *TeacherProfileHandler) GetTeacherProfileByID(c *fiber.Ctx) error {
	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Invalid ID",
			"error":   err.Error(),
		})
	}

	profile, err := h.service.GetTeacherProfileByID(c.Context(), id)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"message": "Teacher profile not found",
			"error":   err.Error(),
		})
	}

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"message": "Teacher profile retrieved successfully",
		"data":    profile,
	})
}

func (h *TeacherProfileHandler) UpdateTeacherProfile(c *fiber.Ctx) error {
	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Invalid ID",
			"error":   err.Error(),
		})
	}

	userID, ok := c.Locals("user_id").(uuid.UUID)
	if !ok {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"message": "Unauthorized",
		})
	}

	var req dto.UpdateTeacherProfileDTO
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Invalid request body",
			"error":   err.Error(),
		})
	}

	profile, err := h.service.UpdateTeacherProfile(c.Context(), id, userID, req)
	if err != nil {
		if teacherProfileForbiddenResponse(c, err) {
			return nil
		}
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Failed to update teacher profile",
			"error":   err.Error(),
		})
	}

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"message": "Teacher profile updated successfully",
		"data":    profile,
	})
}

func (h *TeacherProfileHandler) DeleteTeacherProfile(c *fiber.Ctx) error {
	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Invalid ID",
			"error":   err.Error(),
		})
	}

	userID, ok := c.Locals("user_id").(uuid.UUID)
	if !ok {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"message": "Unauthorized",
		})
	}

	hardDelete := c.QueryBool("hard_delete", false)

	if err := h.service.DeleteTeacherProfile(c.Context(), id, userID, hardDelete); err != nil {
		if teacherProfileForbiddenResponse(c, err) {
			return nil
		}
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Failed to delete teacher profile",
			"error":   err.Error(),
		})
	}

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"message": "Teacher profile deleted successfully",
	})
}
