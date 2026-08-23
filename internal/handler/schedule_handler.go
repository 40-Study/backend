package handler

import (
	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"study.com/v1/internal/dto"
	"study.com/v1/internal/service"
	"study.com/v1/internal/utils"
)

type ScheduleHandler struct {
	service service.ScheduleServiceInterface
}

func NewScheduleHandler(service service.ScheduleServiceInterface) *ScheduleHandler {
	return &ScheduleHandler{service: service}
}

// ============================================================================
// CLASS SCHEDULE
// ============================================================================

func (h *ScheduleHandler) CreateSchedule(c *fiber.Ctx) error {
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

	if errs := utils.ValidateStruct(req); errs != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Validation failed",
			"errors":  errs,
		})
	}

	schedule, err := h.service.CreateSchedule(c.Context(), classID, req)
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

func (h *ScheduleHandler) GetSchedulesByClass(c *fiber.Ctx) error {
	classID, err := uuid.Parse(c.Params("classId"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Invalid class ID",
			"error":   err.Error(),
		})
	}

	schedules, err := h.service.GetSchedulesByClass(c.Context(), classID)
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

func (h *ScheduleHandler) GetScheduleByID(c *fiber.Ctx) error {
	_, err := uuid.Parse(c.Params("classId"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Invalid class ID",
			"error":   err.Error(),
		})
	}

	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Invalid schedule ID",
			"error":   err.Error(),
		})
	}

	schedule, err := h.service.GetScheduleByID(c.Context(), id)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"message": "Schedule not found",
			"error":   err.Error(),
		})
	}

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"message": "Schedule retrieved successfully",
		"data":    schedule,
	})
}

func (h *ScheduleHandler) UpdateSchedule(c *fiber.Ctx) error {
	_, err := uuid.Parse(c.Params("classId"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Invalid class ID",
			"error":   err.Error(),
		})
	}

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

	if errs := utils.ValidateStruct(req); errs != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Validation failed",
			"errors":  errs,
		})
	}

	schedule, err := h.service.UpdateSchedule(c.Context(), id, req)
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

func (h *ScheduleHandler) DeleteSchedule(c *fiber.Ctx) error {
	_, err := uuid.Parse(c.Params("classId"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Invalid class ID",
			"error":   err.Error(),
		})
	}

	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Invalid schedule ID",
			"error":   err.Error(),
		})
	}

	if err := h.service.DeleteSchedule(c.Context(), id); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Failed to delete schedule",
			"error":   err.Error(),
		})
	}

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"message": "Schedule deleted successfully",
	})
}

func (h *ScheduleHandler) GetClassTimetable(c *fiber.Ctx) error {
	classID, err := uuid.Parse(c.Params("classId"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Invalid class ID",
			"error":   err.Error(),
		})
	}

	timetable, err := h.service.GetClassTimetable(c.Context(), classID)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"message": "Failed to retrieve timetable",
			"error":   err.Error(),
		})
	}

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"message": "Timetable retrieved successfully",
		"data":    timetable,
	})
}

// ============================================================================
// CLASS SESSION
// ============================================================================

func (h *ScheduleHandler) CreateSession(c *fiber.Ctx) error {
	classID, err := uuid.Parse(c.Params("classId"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Invalid class ID",
			"error":   err.Error(),
		})
	}

	var req dto.CreateClassSessionDTO
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

	session, err := h.service.CreateSession(c.Context(), classID, req)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Failed to create session",
			"error":   err.Error(),
		})
	}

	return c.Status(fiber.StatusCreated).JSON(fiber.Map{
		"message": "Session created successfully",
		"data":    session,
	})
}

func (h *ScheduleHandler) GetSessionsByClass(c *fiber.Ctx) error {
	classID, err := uuid.Parse(c.Params("classId"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Invalid class ID",
			"error":   err.Error(),
		})
	}

	page := c.QueryInt("page", 1)
	pageSize := c.QueryInt("page_size", 20)

	sessions, err := h.service.GetSessionsByClass(c.Context(), classID, page, pageSize)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"message": "Failed to retrieve sessions",
			"error":   err.Error(),
		})
	}

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"message": "Sessions retrieved successfully",
		"data":    sessions,
	})
}

func (h *ScheduleHandler) GetSessionByID(c *fiber.Ctx) error {
	_, err := uuid.Parse(c.Params("classId"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Invalid class ID",
			"error":   err.Error(),
		})
	}

	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Invalid session ID",
			"error":   err.Error(),
		})
	}

	session, err := h.service.GetSessionByID(c.Context(), id)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"message": "Session not found",
			"error":   err.Error(),
		})
	}

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"message": "Session retrieved successfully",
		"data":    session,
	})
}

func (h *ScheduleHandler) UpdateSession(c *fiber.Ctx) error {
	_, err := uuid.Parse(c.Params("classId"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Invalid class ID",
			"error":   err.Error(),
		})
	}

	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Invalid session ID",
			"error":   err.Error(),
		})
	}

	var req dto.UpdateClassSessionDTO
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

	session, err := h.service.UpdateSession(c.Context(), id, req)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Failed to update session",
			"error":   err.Error(),
		})
	}

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"message": "Session updated successfully",
		"data":    session,
	})
}

func (h *ScheduleHandler) CancelSession(c *fiber.Ctx) error {
	_, err := uuid.Parse(c.Params("classId"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Invalid class ID",
			"error":   err.Error(),
		})
	}

	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Invalid session ID",
			"error":   err.Error(),
		})
	}

	var body struct {
		CancelReason string `json:"cancel_reason"`
	}
	if err := c.BodyParser(&body); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Invalid request body",
			"error":   err.Error(),
		})
	}

	if err := h.service.CancelSession(c.Context(), id, body.CancelReason); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Failed to cancel session",
			"error":   err.Error(),
		})
	}

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"message": "Session cancelled successfully",
	})
}

func (h *ScheduleHandler) GenerateSessions(c *fiber.Ctx) error {
	classID, err := uuid.Parse(c.Params("classId"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Invalid class ID",
			"error":   err.Error(),
		})
	}

	var req dto.GenerateSessionsDTO
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

	sessions, err := h.service.GenerateSessions(c.Context(), classID, req)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Failed to generate sessions",
			"error":   err.Error(),
		})
	}

	return c.Status(fiber.StatusCreated).JSON(fiber.Map{
		"message": "Sessions generated successfully",
		"data":    sessions,
	})
}

// ============================================================================
// SESSION ATTENDANCE
// ============================================================================

func (h *ScheduleHandler) GetSessionAttendances(c *fiber.Ctx) error {
	sessionID, err := uuid.Parse(c.Params("sessionId"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Invalid session ID",
			"error":   err.Error(),
		})
	}

	userID, ok := c.Locals("user_id").(uuid.UUID)
	if !ok {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"message": "Unauthorized"})
	}

	attendances, err := h.service.GetSessionAttendances(c.Context(), sessionID, userID)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"message": "Failed to retrieve attendances",
			"error":   err.Error(),
		})
	}

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"message": "Attendances retrieved successfully",
		"data":    attendances,
	})
}

func (h *ScheduleHandler) MarkAttendance(c *fiber.Ctx) error {
	sessionID, err := uuid.Parse(c.Params("sessionId"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Invalid session ID",
			"error":   err.Error(),
		})
	}

	userID, ok := c.Locals("user_id").(uuid.UUID)
	if !ok {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"message": "Unauthorized",
		})
	}

	var req dto.MarkAttendanceDTO
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

	attendance, err := h.service.MarkAttendance(c.Context(), sessionID, req, userID)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Failed to mark attendance",
			"error":   err.Error(),
		})
	}

	return c.Status(fiber.StatusCreated).JSON(fiber.Map{
		"message": "Attendance marked successfully",
		"data":    attendance,
	})
}

func (h *ScheduleHandler) BulkMarkAttendance(c *fiber.Ctx) error {
	sessionID, err := uuid.Parse(c.Params("sessionId"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Invalid session ID",
			"error":   err.Error(),
		})
	}

	userID, ok := c.Locals("user_id").(uuid.UUID)
	if !ok {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"message": "Unauthorized",
		})
	}

	var req dto.BulkMarkAttendanceDTO
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

	attendances, err := h.service.BulkMarkAttendance(c.Context(), sessionID, req, userID)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Failed to bulk mark attendance",
			"error":   err.Error(),
		})
	}

	return c.Status(fiber.StatusCreated).JSON(fiber.Map{
		"message": "Attendance marked successfully",
		"data":    attendances,
	})
}

func (h *ScheduleHandler) UpdateAttendance(c *fiber.Ctx) error {
	sessionID, err := uuid.Parse(c.Params("sessionId"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Invalid session ID",
			"error":   err.Error(),
		})
	}

	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Invalid attendance ID",
			"error":   err.Error(),
		})
	}

	userID, ok := c.Locals("user_id").(uuid.UUID)
	if !ok {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"message": "Unauthorized"})
	}

	var req dto.UpdateSessionAttendanceDTO
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

	attendance, err := h.service.UpdateAttendance(c.Context(), sessionID, id, req, userID)
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

func (h *ScheduleHandler) StudentCheckIn(c *fiber.Ctx) error {
	sessionID, err := uuid.Parse(c.Params("sessionId"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Invalid session ID",
			"error":   err.Error(),
		})
	}

	userID, ok := c.Locals("user_id").(uuid.UUID)
	if !ok {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"message": "Unauthorized",
		})
	}

	attendance, err := h.service.StudentCheckIn(c.Context(), sessionID, userID)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Failed to check in",
			"error":   err.Error(),
		})
	}

	return c.Status(fiber.StatusCreated).JSON(fiber.Map{
		"message": "Checked in successfully",
		"data":    attendance,
	})
}

func (h *ScheduleHandler) StudentCheckOut(c *fiber.Ctx) error {
	sessionID, err := uuid.Parse(c.Params("sessionId"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Invalid session ID",
			"error":   err.Error(),
		})
	}

	userID, ok := c.Locals("user_id").(uuid.UUID)
	if !ok {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"message": "Unauthorized",
		})
	}

	attendance, err := h.service.StudentCheckOut(c.Context(), sessionID, userID)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Failed to check out",
			"error":   err.Error(),
		})
	}

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"message": "Checked out successfully",
		"data":    attendance,
	})
}

// ============================================================================
// MY TIMETABLE / ATTENDANCES / REMINDERS
// ============================================================================

func (h *ScheduleHandler) GetMyTimetable(c *fiber.Ctx) error {
	userID, ok := c.Locals("user_id").(uuid.UUID)
	if !ok {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"message": "Unauthorized",
		})
	}

	activeRole, _ := c.Locals("active_role").(string)

	timetable, err := h.service.GetMyTimetable(c.Context(), userID, activeRole)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"message": "Failed to retrieve timetable",
			"error":   err.Error(),
		})
	}

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"message": "Timetable retrieved successfully",
		"data":    timetable,
	})
}

func (h *ScheduleHandler) GetMyAttendances(c *fiber.Ctx) error {
	userID, ok := c.Locals("user_id").(uuid.UUID)
	if !ok {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"message": "Unauthorized",
		})
	}

	page := c.QueryInt("page", 1)
	pageSize := c.QueryInt("page_size", 20)

	attendances, total, err := h.service.GetMyAttendances(c.Context(), userID, page, pageSize)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"message": "Failed to retrieve attendances",
			"error":   err.Error(),
		})
	}

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"message": "Attendances retrieved successfully",
		"data": fiber.Map{
			"attendances": attendances,
			"total":       total,
			"page":        page,
			"page_size":   pageSize,
		},
	})
}

func (h *ScheduleHandler) GetReminderSettings(c *fiber.Ctx) error {
	userID, ok := c.Locals("user_id").(uuid.UUID)
	if !ok {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"message": "Unauthorized",
		})
	}

	settings, err := h.service.GetReminderSettings(c.Context(), userID)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"message": "Failed to retrieve reminder settings",
			"error":   err.Error(),
		})
	}

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"message": "Reminder settings retrieved successfully",
		"data":    settings,
	})
}

func (h *ScheduleHandler) UpdateReminderSetting(c *fiber.Ctx) error {
	userID, ok := c.Locals("user_id").(uuid.UUID)
	if !ok {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"message": "Unauthorized",
		})
	}

	var req dto.UpdateReminderSettingDTO
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

	setting, err := h.service.UpdateReminderSetting(c.Context(), userID, req)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Failed to update reminder setting",
			"error":   err.Error(),
		})
	}

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"message": "Reminder setting updated successfully",
		"data":    setting,
	})
}
