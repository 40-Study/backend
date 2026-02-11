package handler

import (
	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"study.com/v1/internal/dto"
	"study.com/v1/internal/service"
)

type AttendanceHandlerInterface interface {
	MarkAttendance(c *fiber.Ctx) error
	GetAllAttendances(c *fiber.Ctx) error
	GetAttendanceByID(c *fiber.Ctx) error
	UpdateAttendance(c *fiber.Ctx) error
	DeleteAttendance(c *fiber.Ctx) error
}

type AttendanceHandler struct {
	service service.AttendanceServiceInterface
}

func NewAttendanceHandler(service service.AttendanceServiceInterface) *AttendanceHandler {
	return &AttendanceHandler{service: service}
}

func (h *AttendanceHandler) MarkAttendance(c *fiber.Ctx) error {
	classID, err := uuid.Parse(c.Params("classId"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Invalid class ID",
			"error":   err.Error(),
		})
	}

	var req dto.BulkCreateAttendanceDTO
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Invalid request body",
			"error":   err.Error(),
		})
	}

	attendances, err := h.service.MarkAttendance(c.Context(), classID, req)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Failed to mark attendances",
			"error":   err.Error(),
		})
	}

	return c.Status(fiber.StatusCreated).JSON(fiber.Map{
		"message": "Attendances marked successfully",
		"data":    attendances,
	})
}

func (h *AttendanceHandler) GetAllAttendances(c *fiber.Ctx) error {
	classID, err := uuid.Parse(c.Params("classId"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Invalid class ID",
			"error":   err.Error(),
		})
	}

	page := c.QueryInt("page", 1)
	pageSize := c.QueryInt("page_size", 20)
	date := c.Query("date")

	attendances, err := h.service.GetAllAttendances(c.Context(), classID, date, page, pageSize)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Failed to retrieve attendances",
			"error":   err.Error(),
		})
	}

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"message": "Attendances retrieved successfully",
		"data":    attendances,
	})
}

func (h *AttendanceHandler) GetAttendanceByID(c *fiber.Ctx) error {
	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Invalid attendance ID",
			"error":   err.Error(),
		})
	}

	attendance, err := h.service.GetAttendanceByID(c.Context(), id)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"message": "Attendance not found",
			"error":   err.Error(),
		})
	}

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"message": "Attendance retrieved successfully",
		"data":    attendance,
	})
}

func (h *AttendanceHandler) UpdateAttendance(c *fiber.Ctx) error {
	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Invalid attendance ID",
			"error":   err.Error(),
		})
	}

	var req dto.UpdateAttendanceDTO
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Invalid request body",
			"error":   err.Error(),
		})
	}

	attendance, err := h.service.UpdateAttendance(c.Context(), id, req)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Failed to update attendance",
			"error":   err.Error(),
		})
	}

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"message": "Attendance updated successfully",
		"data":    attendance,
	})
}

func (h *AttendanceHandler) DeleteAttendance(c *fiber.Ctx) error {
	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Invalid attendance ID",
			"error":   err.Error(),
		})
	}

	if err := h.service.DeleteAttendance(c.Context(), id); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Failed to delete attendance",
			"error":   err.Error(),
		})
	}

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"message": "Attendance deleted successfully",
	})
}
