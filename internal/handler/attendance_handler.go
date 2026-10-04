package handler

import (
	"errors"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"study.com/v1/internal/dto"
	"study.com/v1/internal/middleware"
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
	service     service.AttendanceServiceInterface
	permChecker *middleware.PermissionChecker
}

func NewAttendanceHandler(service service.AttendanceServiceInterface, permChecker *middleware.PermissionChecker) *AttendanceHandler {
	return &AttendanceHandler{service: service, permChecker: permChecker}
}

// attendanceActor (S4): người gọi và cờ admin. Trước đây các route điểm danh của lớp không kiểm gì, nên
// bất kỳ tài khoản đăng nhập nào cũng tạo, sửa, xoá và đọc điểm danh của lớp bất kỳ.
func (h *AttendanceHandler) attendanceActor(c *fiber.Ctx) (uuid.UUID, bool, bool) {
	actor, ok := c.Locals("user_id").(uuid.UUID)
	if !ok {
		return uuid.Nil, false, false
	}
	return actor, isAdminActor(c, h.permChecker, actor), true
}

// attendanceFail ánh xạ lỗi uỷ quyền (404 lớp/bản ghi không xem được hoặc không thuộc lớp, 403 không quản
// lý lớp); lỗi khác giữ nguyên mã 400 và thông điệp của từng route.
func attendanceFail(c *fiber.Ctx, err error, message string, fallback int) error {
	if status := classErrorStatus(err); status != 0 {
		return c.Status(status).JSON(classErrorBody(err))
	}
	if errors.Is(err, service.ErrAttendanceNotFound) {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"message": err.Error()})
	}
	return c.Status(fallback).JSON(fiber.Map{"message": message, "error": err.Error()})
}

func attendanceUnauthorized(c *fiber.Ctx) error {
	return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"message": "Unauthorized"})
}

func (h *AttendanceHandler) MarkAttendance(c *fiber.Ctx) error {
	classID, err := uuid.Parse(c.Params("classId"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Invalid class ID",
			"error":   err.Error(),
		})
	}
	actor, isAdmin, ok := h.attendanceActor(c)
	if !ok {
		return attendanceUnauthorized(c)
	}

	var req dto.BulkCreateAttendanceDTO
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Invalid request body",
			"error":   err.Error(),
		})
	}

	attendances, err := h.service.MarkAttendance(c.Context(), classID, actor, isAdmin, req)
	if err != nil {
		return attendanceFail(c, err, "Failed to mark attendances", fiber.StatusBadRequest)
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
	actor, isAdmin, ok := h.attendanceActor(c)
	if !ok {
		return attendanceUnauthorized(c)
	}

	page := c.QueryInt("page", 1)
	pageSize := c.QueryInt("page_size", 20)
	date := c.Query("date")

	attendances, err := h.service.GetAllAttendances(c.Context(), classID, actor, isAdmin, date, page, pageSize)
	if err != nil {
		return attendanceFail(c, err, "Failed to retrieve attendances", fiber.StatusBadRequest)
	}

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"message": "Attendances retrieved successfully",
		"data":    attendances,
	})
}

func (h *AttendanceHandler) GetAttendanceByID(c *fiber.Ctx) error {
	classID, err := uuid.Parse(c.Params("classId"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Invalid class ID",
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
	actor, isAdmin, ok := h.attendanceActor(c)
	if !ok {
		return attendanceUnauthorized(c)
	}

	attendance, err := h.service.GetAttendanceByID(c.Context(), classID, id, actor, isAdmin)
	if err != nil {
		return attendanceFail(c, err, "Attendance not found", fiber.StatusNotFound)
	}

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"message": "Attendance retrieved successfully",
		"data":    attendance,
	})
}

func (h *AttendanceHandler) UpdateAttendance(c *fiber.Ctx) error {
	classID, err := uuid.Parse(c.Params("classId"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Invalid class ID",
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
	actor, isAdmin, ok := h.attendanceActor(c)
	if !ok {
		return attendanceUnauthorized(c)
	}

	var req dto.UpdateAttendanceDTO
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Invalid request body",
			"error":   err.Error(),
		})
	}

	attendance, err := h.service.UpdateAttendance(c.Context(), classID, id, actor, isAdmin, req)
	if err != nil {
		return attendanceFail(c, err, "Failed to update attendance", fiber.StatusBadRequest)
	}

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"message": "Attendance updated successfully",
		"data":    attendance,
	})
}

func (h *AttendanceHandler) DeleteAttendance(c *fiber.Ctx) error {
	classID, err := uuid.Parse(c.Params("classId"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Invalid class ID",
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
	actor, isAdmin, ok := h.attendanceActor(c)
	if !ok {
		return attendanceUnauthorized(c)
	}

	if err := h.service.DeleteAttendance(c.Context(), classID, id, actor, isAdmin); err != nil {
		return attendanceFail(c, err, "Failed to delete attendance", fiber.StatusBadRequest)
	}

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"message": "Attendance deleted successfully",
	})
}
