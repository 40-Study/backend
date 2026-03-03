package handler

import (
	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"study.com/v1/internal/dto"
	"study.com/v1/internal/service"
)

type ClassScheduleHandlerInterface interface {
	CreateClassSchedule(c *fiber.Ctx) error
	GetAllClassSchedules(c *fiber.Ctx) error
	UpdateClassSchedule(c *fiber.Ctx) error
	DeleteClassSchedule(c *fiber.Ctx) error
}

type ClassScheduleHandler struct {
	service service.ClassScheduleServiceInterface
}

func NewClassScheduleHandler(service service.ClassScheduleServiceInterface) *ClassScheduleHandler {
	return &ClassScheduleHandler{service: service}
}

func (h *ClassScheduleHandler) CreateClassSchedule(c *fiber.Ctx) error {
	classID, err := uuid.Parse(c.Params("classId"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Invalid class ID",
			"error":   err.Error(),
		})
	}

	var req dto.CreateClassScheduleDTO
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Invalid request body",
			"error":   err.Error(),
		})
	}

	schedule, err := h.service.CreateClassSchedule(c.Context(), classID, req)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Failed to create schedule",
			"error":   err.Error(),
		})
	}

	return c.Status(fiber.StatusCreated).JSON(fiber.Map{
		"message": "Schedule created successfully",
		"data":    schedule,
	})
}

func (h *ClassScheduleHandler) GetAllClassSchedules(c *fiber.Ctx) error {
	classID, err := uuid.Parse(c.Params("classId"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Invalid class ID",
			"error":   err.Error(),
		})
	}

	schedules, err := h.service.GetAllClassSchedulesByClassID(c.Context(), classID)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"message": "Failed to retrieve schedules",
			"error":   err.Error(),
		})
	}

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"message": "Schedules retrieved successfully",
		"data":    schedules,
	})
}

func (h *ClassScheduleHandler) UpdateClassSchedule(c *fiber.Ctx) error {
	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Invalid schedule ID",
			"error":   err.Error(),
		})
	}

	var req dto.UpdateClassScheduleDTO
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Invalid request body",
			"error":   err.Error(),
		})
	}

	schedule, err := h.service.UpdateClassSchedule(c.Context(), id, req)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Failed to update schedule",
			"error":   err.Error(),
		})
	}

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"message": "Schedule updated successfully",
		"data":    schedule,
	})
}

func (h *ClassScheduleHandler) DeleteClassSchedule(c *fiber.Ctx) error {
	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Invalid schedule ID",
			"error":   err.Error(),
		})
	}

	if err := h.service.DeleteClassSchedule(c.Context(), id); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Failed to delete schedule",
			"error":   err.Error(),
		})
	}

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"message": "Schedule deleted successfully",
	})
}
