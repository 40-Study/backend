package handler

import (
	"errors"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"study.com/v1/internal/dto"
	"study.com/v1/internal/middleware"
	"study.com/v1/internal/service"
	"study.com/v1/internal/utils"
)

type ScheduleHandler struct {
	service     service.ScheduleServiceInterface
	permChecker *middleware.PermissionChecker
}

func NewScheduleHandler(service service.ScheduleServiceInterface, permChecker *middleware.PermissionChecker) *ScheduleHandler {
	return &ScheduleHandler{service: service, permChecker: permChecker}
}

// scheduleActor (S5): người gọi và cờ admin. Trước đây các route lịch học và buổi học của lớp không kiểm gì
// nên tài khoản đăng nhập bất kỳ, kể cả học viên, tạo, sửa, xoá, huỷ buổi và đọc lịch của lớp bất kỳ.
func (h *ScheduleHandler) scheduleActor(c *fiber.Ctx) (uuid.UUID, bool, bool) {
	actor, ok := c.Locals("user_id").(uuid.UUID)
	if !ok {
		return uuid.Nil, false, false
	}
	return actor, isAdminActor(c, h.permChecker, actor), true
}

func scheduleUnauthorized(c *fiber.Ctx) error {
	return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"message": "Unauthorized"})
}

// scheduleFail ánh xạ lỗi uỷ quyền (404 lớp/lịch/buổi không xem được hoặc không tồn tại, 403 xem được nhưng
// không quản lý); lỗi khác giữ mã và thông điệp cũ của từng route.
func scheduleFail(c *fiber.Ctx, err error, message string, fallback int) error {
	status := classErrorStatus(err)
	if status == 0 && (errors.Is(err, service.ErrScheduleNotFound) || errors.Is(err, service.ErrClassSessionNotFound)) {
		status = fiber.StatusNotFound
	}
	// Điểm danh ngoài ngày / buổi đã đóng: 409 (đúng người, sai thời điểm), không phải 400 chung chung.
	if status == 0 && (errors.Is(err, service.ErrCheckInOutsideSessionDay) || errors.Is(err, service.ErrSessionClosedForCheckIn)) {
		status = fiber.StatusConflict
	}
	// B-10: trùng giờ là 409, giờ kết thúc không sau giờ bắt đầu là 400 (chặn cả khi fallback của route khác 400).
	if status == 0 && errors.Is(err, service.ErrSessionOverlap) {
		status = fiber.StatusConflict
	}
	if status == 0 && errors.Is(err, service.ErrSessionTimeOrder) {
		status = fiber.StatusBadRequest
	}
	if status != 0 {
		return c.Status(status).JSON(fiber.Map{"message": err.Error()})
	}
	return c.Status(fallback).JSON(fiber.Map{"message": message, "error": err.Error()})
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

	actor, isAdmin, ok := h.scheduleActor(c)
	if !ok {
		return scheduleUnauthorized(c)
	}

	schedule, err := h.service.CreateSchedule(c.Context(), classID, actor, isAdmin, req)
	if err != nil {
		return scheduleFail(c, err, "Failed to create schedule", fiber.StatusBadRequest)
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

	actor, isAdmin, ok := h.scheduleActor(c)
	if !ok {
		return scheduleUnauthorized(c)
	}

	schedules, err := h.service.GetSchedulesByClass(c.Context(), classID, actor, isAdmin)
	if err != nil {
		return scheduleFail(c, err, "Failed to retrieve schedules", fiber.StatusInternalServerError)
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

	actor, isAdmin, ok := h.scheduleActor(c)
	if !ok {
		return scheduleUnauthorized(c)
	}

	schedule, err := h.service.GetScheduleByID(c.Context(), id, actor, isAdmin)
	if err != nil {
		return scheduleFail(c, err, "Schedule not found", fiber.StatusNotFound)
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

	actor, isAdmin, ok := h.scheduleActor(c)
	if !ok {
		return scheduleUnauthorized(c)
	}

	schedule, err := h.service.UpdateSchedule(c.Context(), id, actor, isAdmin, req)
	if err != nil {
		return scheduleFail(c, err, "Failed to update schedule", fiber.StatusBadRequest)
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

	actor, isAdmin, ok := h.scheduleActor(c)
	if !ok {
		return scheduleUnauthorized(c)
	}

	if err := h.service.DeleteSchedule(c.Context(), id, actor, isAdmin); err != nil {
		return scheduleFail(c, err, "Failed to delete schedule", fiber.StatusBadRequest)
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

	actor, isAdmin, ok := h.scheduleActor(c)
	if !ok {
		return scheduleUnauthorized(c)
	}

	timetable, err := h.service.GetClassTimetable(c.Context(), classID, actor, isAdmin)
	if err != nil {
		return scheduleFail(c, err, "Failed to retrieve timetable", fiber.StatusInternalServerError)
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

	actor, isAdmin, ok := h.scheduleActor(c)
	if !ok {
		return scheduleUnauthorized(c)
	}

	session, err := h.service.CreateSession(c.Context(), classID, actor, isAdmin, req)
	if err != nil {
		return scheduleFail(c, err, "Failed to create session", fiber.StatusBadRequest)
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

	actor, isAdmin, ok := h.scheduleActor(c)
	if !ok {
		return scheduleUnauthorized(c)
	}

	sessions, err := h.service.GetSessionsByClass(c.Context(), classID, actor, isAdmin, page, pageSize)
	if err != nil {
		return scheduleFail(c, err, "Failed to retrieve sessions", fiber.StatusInternalServerError)
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

	actor, isAdmin, ok := h.scheduleActor(c)
	if !ok {
		return scheduleUnauthorized(c)
	}

	session, err := h.service.GetSessionByID(c.Context(), id, actor, isAdmin)
	if err != nil {
		return scheduleFail(c, err, "Session not found", fiber.StatusNotFound)
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

	actor, isAdmin, ok := h.scheduleActor(c)
	if !ok {
		return scheduleUnauthorized(c)
	}

	session, err := h.service.UpdateSession(c.Context(), id, actor, isAdmin, req)
	if err != nil {
		return scheduleFail(c, err, "Failed to update session", fiber.StatusBadRequest)
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

	actor, isAdmin, ok := h.scheduleActor(c)
	if !ok {
		return scheduleUnauthorized(c)
	}

	if err := h.service.CancelSession(c.Context(), id, actor, isAdmin, body.CancelReason); err != nil {
		return scheduleFail(c, err, "Failed to cancel session", fiber.StatusBadRequest)
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

	actor, isAdmin, ok := h.scheduleActor(c)
	if !ok {
		return scheduleUnauthorized(c)
	}

	sessions, err := h.service.GenerateSessions(c.Context(), classID, actor, isAdmin, req)
	if err != nil {
		return scheduleFail(c, err, "Failed to generate sessions", fiber.StatusBadRequest)
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

	attendances, err := h.service.GetSessionAttendances(c.Context(), sessionID, userID, isAdminActor(c, h.permChecker, userID))
	if err != nil {
		return scheduleFail(c, err, "Failed to retrieve attendances", fiber.StatusInternalServerError)
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

	attendance, err := h.service.MarkAttendance(c.Context(), sessionID, req, userID, isAdminActor(c, h.permChecker, userID))
	if err != nil {
		return scheduleFail(c, err, "Failed to mark attendance", fiber.StatusBadRequest)
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

	attendances, err := h.service.BulkMarkAttendance(c.Context(), sessionID, req, userID, isAdminActor(c, h.permChecker, userID))
	if err != nil {
		return scheduleFail(c, err, "Failed to bulk mark attendance", fiber.StatusBadRequest)
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

	attendance, err := h.service.UpdateAttendance(c.Context(), sessionID, id, req, userID, isAdminActor(c, h.permChecker, userID))
	if err != nil {
		return scheduleFail(c, err, "Failed to update attendance", fiber.StatusBadRequest)
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
		return scheduleFail(c, err, "Failed to check in", fiber.StatusBadRequest)
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
		return scheduleFail(c, err, "Failed to check out", fiber.StatusBadRequest)
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

	// B-08: ?sessions_from=YYYY-MM-DD&sessions_to=YYYY-MM-DD kèm thêm buổi học cụ thể của lớp trong khoảng
	// đó. Không gửi thì giữ nguyên hành vi cũ (chỉ lịch lặp tuần) nên trang lịch học viên không đổi.
	var timetable *dto.TimetableResponseDTO
	var err error
	if rawFrom, rawTo := c.Query("sessions_from"), c.Query("sessions_to"); rawFrom != "" || rawTo != "" {
		from, errFrom := time.Parse("2006-01-02", rawFrom)
		to, errTo := time.Parse("2006-01-02", rawTo)
		if errFrom != nil || errTo != nil {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
				"message": "sessions_from and sessions_to must both be YYYY-MM-DD",
			})
		}
		timetable, err = h.service.GetMyTimetableWithSessions(c.Context(), userID, activeRole, from, to)
		if errors.Is(err, service.ErrTimetableRangeInvalid) {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"message": err.Error()})
		}
	} else {
		timetable, err = h.service.GetMyTimetable(c.Context(), userID, activeRole)
	}
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
