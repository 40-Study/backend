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

func (h *TeacherProfileHandler) CreateTeacherProfile(c *fiber.Ctx) error {
	var req dto.CreateTeacherProfileDTO
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Invalid request body",
			"error":   err.Error(),
		})
	}

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

	var req dto.UpdateTeacherProfileDTO
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Invalid request body",
			"error":   err.Error(),
		})
	}

	profile, err := h.service.UpdateTeacherProfile(c.Context(), id, req)
	if err != nil {
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

	hardDelete := c.QueryBool("hard_delete", false)

	if err := h.service.DeleteTeacherProfile(c.Context(), id, hardDelete); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Failed to delete teacher profile",
			"error":   err.Error(),
		})
	}

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"message": "Teacher profile deleted successfully",
	})
}
