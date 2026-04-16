package handler

import (
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"study.com/v1/internal/dto"
	"study.com/v1/internal/service"
	"study.com/v1/internal/utils"
)

type PersonalEventHandler struct {
	svc service.PersonalEventServiceInterface
}

func NewPersonalEventHandler(svc service.PersonalEventServiceInterface) *PersonalEventHandler {
	return &PersonalEventHandler{svc: svc}
}

// POST /api/me/events
func (h *PersonalEventHandler) Create(c *fiber.Ctx) error {
	userIDVal := c.Locals("user_id")
	if userIDVal == nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"message": "Unauthorized"})
	}
	userID := userIDVal.(uuid.UUID)

	var req dto.CreatePersonalEventDTO
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Invalid request body",
			"error":   err.Error(),
		})
	}

	if errs := utils.ValidateStruct(req); errs != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Validation failed",
			"errors":  errs,
		})
	}

	event, err := h.svc.CreateEvent(c.Context(), userID, req)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Failed to create event",
			"error":   err.Error(),
		})
	}

	return c.Status(fiber.StatusCreated).JSON(fiber.Map{
		"message": "Event created successfully",
		"data":    event,
	})
}

// GET /api/me/events?start=ISO&end=ISO
func (h *PersonalEventHandler) List(c *fiber.Ctx) error {
	userIDVal := c.Locals("user_id")
	if userIDVal == nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"message": "Unauthorized"})
	}
	userID := userIDVal.(uuid.UUID)

	startStr := c.Query("start")
	endStr := c.Query("end")
	if startStr == "" || endStr == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "start and end query parameters are required (RFC3339 format)",
		})
	}

	startDate, err := time.Parse(time.RFC3339, startStr)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Invalid start date format, expected RFC3339",
			"error":   err.Error(),
		})
	}

	endDate, err := time.Parse(time.RFC3339, endStr)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Invalid end date format, expected RFC3339",
			"error":   err.Error(),
		})
	}

	events, err := h.svc.GetMyEvents(c.Context(), userID, startDate, endDate)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"message": "Failed to retrieve events",
			"error":   err.Error(),
		})
	}

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"message": "Events retrieved successfully",
		"data":    events,
	})
}

// PUT /api/me/events/:id
func (h *PersonalEventHandler) Update(c *fiber.Ctx) error {
	userIDVal := c.Locals("user_id")
	if userIDVal == nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"message": "Unauthorized"})
	}
	userID := userIDVal.(uuid.UUID)

	eventID, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Invalid event ID",
			"error":   err.Error(),
		})
	}

	var req dto.UpdatePersonalEventDTO
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Invalid request body",
			"error":   err.Error(),
		})
	}

	if errs := utils.ValidateStruct(req); errs != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Validation failed",
			"errors":  errs,
		})
	}

	event, err := h.svc.UpdateEvent(c.Context(), userID, eventID, req)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Failed to update event",
			"error":   err.Error(),
		})
	}

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"message": "Event updated successfully",
		"data":    event,
	})
}

// DELETE /api/me/events/:id
func (h *PersonalEventHandler) Delete(c *fiber.Ctx) error {
	userIDVal := c.Locals("user_id")
	if userIDVal == nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"message": "Unauthorized"})
	}
	userID := userIDVal.(uuid.UUID)

	eventID, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Invalid event ID",
			"error":   err.Error(),
		})
	}

	if err := h.svc.DeleteEvent(c.Context(), userID, eventID); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Failed to delete event",
			"error":   err.Error(),
		})
	}

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"message": "Event deleted successfully",
	})
}
