package handler

import (
	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"study.com/v1/internal/service"
)

type ParentDashboardHandler struct {
	svc service.ParentDashboardServiceInterface
}

func NewParentDashboardHandler(svc service.ParentDashboardServiceInterface) *ParentDashboardHandler {
	return &ParentDashboardHandler{svc: svc}
}

// GetChildOverview GET /parent/children/:id/overview
func (h *ParentDashboardHandler) GetChildOverview(c *fiber.Ctx) error {
	parentID, ok := c.Locals("user_id").(uuid.UUID)
	if !ok || parentID == uuid.Nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "unauthorized"})
	}

	childID, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid child id"})
	}

	overview, err := h.svc.GetChildOverview(c.Context(), parentID, childID)
	if err != nil {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(fiber.Map{
		"message": "success",
		"data":    overview,
	})
}

// GetChildCourses GET /parent/children/:id/courses
func (h *ParentDashboardHandler) GetChildCourses(c *fiber.Ctx) error {
	parentID, ok := c.Locals("user_id").(uuid.UUID)
	if !ok || parentID == uuid.Nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "unauthorized"})
	}

	childID, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid child id"})
	}

	page := c.QueryInt("page", 1)
	pageSize := c.QueryInt("page_size", 20)

	courses, err := h.svc.GetChildCourses(c.Context(), parentID, childID, page, pageSize)
	if err != nil {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(fiber.Map{
		"message": "success",
		"data":    courses,
	})
}

// GetChildGrades GET /parent/children/:id/grades
func (h *ParentDashboardHandler) GetChildGrades(c *fiber.Ctx) error {
	parentID, ok := c.Locals("user_id").(uuid.UUID)
	if !ok || parentID == uuid.Nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "unauthorized"})
	}

	childID, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid child id"})
	}

	grades, err := h.svc.GetChildGrades(c.Context(), parentID, childID)
	if err != nil {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(fiber.Map{
		"message": "success",
		"data":    grades,
	})
}

// GetChildSchedule GET /parent/children/:id/schedule
func (h *ParentDashboardHandler) GetChildSchedule(c *fiber.Ctx) error {
	parentID, ok := c.Locals("user_id").(uuid.UUID)
	if !ok || parentID == uuid.Nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "unauthorized"})
	}

	childID, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid child id"})
	}

	schedule, err := h.svc.GetChildSchedule(c.Context(), parentID, childID)
	if err != nil {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(fiber.Map{
		"message": "success",
		"data":    schedule,
	})
}

// GetChildTimetable GET /parent/children/:id/timetable
func (h *ParentDashboardHandler) GetChildTimetable(c *fiber.Ctx) error {
	parentID, ok := c.Locals("user_id").(uuid.UUID)
	if !ok || parentID == uuid.Nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "unauthorized"})
	}

	childID, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid child id"})
	}

	timetable, err := h.svc.GetChildTimetable(c.Context(), parentID, childID)
	if err != nil {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(fiber.Map{
		"message": "success",
		"data":    timetable,
	})
}

// GetChildAttendance GET /parent/children/:id/attendance
func (h *ParentDashboardHandler) GetChildAttendance(c *fiber.Ctx) error {
	parentID, ok := c.Locals("user_id").(uuid.UUID)
	if !ok || parentID == uuid.Nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "unauthorized"})
	}

	childID, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid child id"})
	}

	page := c.QueryInt("page", 1)
	pageSize := c.QueryInt("page_size", 20)

	attendance, err := h.svc.GetChildAttendance(c.Context(), parentID, childID, page, pageSize)
	if err != nil {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(fiber.Map{
		"message": "success",
		"data":    attendance,
	})
}

// GetChildAssignments GET /parent/children/:id/assignments
func (h *ParentDashboardHandler) GetChildAssignments(c *fiber.Ctx) error {
	parentID, ok := c.Locals("user_id").(uuid.UUID)
	if !ok || parentID == uuid.Nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "unauthorized"})
	}

	childID, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid child id"})
	}

	page := c.QueryInt("page", 1)
	pageSize := c.QueryInt("page_size", 20)

	assignments, err := h.svc.GetChildAssignments(c.Context(), parentID, childID, page, pageSize)
	if err != nil {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(fiber.Map{
		"message": "success",
		"data":    assignments,
	})
}
