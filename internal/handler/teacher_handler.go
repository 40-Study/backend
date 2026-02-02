package handler

import (
	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"study.com/v1/internal/service"
)

type TeacherHandlerInterface interface {
	GetAllTeachers(c *fiber.Ctx) error
	GetTeacher(c *fiber.Ctx) error
	DeleteTeacher(c *fiber.Ctx) error
}

type TeacherHandler struct {
	service service.TeacherServiceInterface
}

func NewTeacherHandler(service service.TeacherServiceInterface) *TeacherHandler {
	return &TeacherHandler{service: service}
}

func (h *TeacherHandler) GetTeacher(c *fiber.Ctx) error {
	idStr := c.Params("id")
	id, err := uuid.Parse(idStr)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Invalid teacher ID",
			"error":   err.Error(),
		})
	}

	teacher, err := h.service.GetTeacherByID(c.Context(), id)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"message": "Teacher not found",
			"error":   err.Error(),
		})
	}

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"message": "Teacher retrieved successfully",
		"data":    teacher,
	})
}

func (h *TeacherHandler) GetAllTeachers(c *fiber.Ctx) error {
	page := c.QueryInt("page", 1)
	pageSize := c.QueryInt("page_size", 20)
	keyword := c.Query("keyword")
	status := c.Query("status")

	teachers, err := h.service.GetAllTeachers(c.Context(), page, pageSize, keyword, status)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"message": "Failed to retrieve teachers",
			"error":   err.Error(),
		})
	}

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"message": "Teachers retrieved successfully",
		"data":    teachers,
	})
}

func (h *TeacherHandler) DeleteTeacher(c *fiber.Ctx) error {
	idStr := c.Params("id")
	id, err := uuid.Parse(idStr)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Invalid teacher ID",
			"error":   err.Error(),
		})
	}

	hardDelete := c.QueryBool("hard_delete", false)

	if err := h.service.DeleteTeacher(c.Context(), id, hardDelete); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Failed to delete teacher",
			"error":   err.Error(),
		})
	}

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"message": "Teacher deleted successfully",
	})
}
