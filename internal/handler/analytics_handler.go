package handler

import (
	"errors"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"study.com/v1/internal/middleware"
	"study.com/v1/internal/service"
)

type AnalyticsHandlerInterface interface {
	GetLivestreamAnalytics(c *fiber.Ctx) error
	GetAssignmentAnalytics(c *fiber.Ctx) error
}

type AnalyticsHandler struct {
	svc         service.AnalyticsServiceInterface
	permChecker *middleware.PermissionChecker
}

func NewAnalyticsHandler(svc service.AnalyticsServiceInterface, permChecker *middleware.PermissionChecker) *AnalyticsHandler {
	return &AnalyticsHandler{svc: svc, permChecker: permChecker}
}

// P1 QA 260927 teacher: 3 handler dưới đây trước không kiểm actor có liên quan gì tới buổi
// live/bài tập không — bất kỳ user đăng nhập nào biết/đoán đúng id là xem được số liệu của
// LỚP/GIÁO VIÊN KHÁC. Nay đòi user_id thật (route đã có AuthMiddleware nên luôn có mặt) và ánh
// xạ lỗi uỷ quyền từ service (ErrNotAnalyticsOwner/ErrNotClassTeacher) sang 403 thay vì 500.
func requireAnalyticsAuthErr(c *fiber.Ctx, err error) error {
	if errors.Is(err, service.ErrAssignmentNotFound) {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"message": "assignment not found"})
	}
	// Buổi không tồn tại là 404 (trước đây rơi về 500 vì service trả errors.New thô); trang thống kê của GV
	// nhập id tay nên đây là trường hợp thường gặp, không phải lỗi hạ tầng.
	if errors.Is(err, service.ErrSessionNotFound) || errors.Is(err, service.ErrClassNotFound) {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"message": "session not found"})
	}
	if errors.Is(err, service.ErrNotAnalyticsOwner) || errors.Is(err, service.ErrNotClassTeacher) {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{
			"message": "You are not authorized to view analytics for this resource",
		})
	}
	return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
}

func (h *AnalyticsHandler) GetLivestreamAnalytics(c *fiber.Ctx) error {
	sessionID, err := uuid.Parse(c.Params("sessionId"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid session_id"})
	}
	userID, ok := c.Locals("user_id").(uuid.UUID)
	if !ok {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"message": "Unauthorized"})
	}
	isAdmin := isAdminActor(c, h.permChecker, userID)

	analytics, err := h.svc.GetLivestreamAnalytics(c.Context(), sessionID, userID, isAdmin)
	if err != nil {
		return requireAnalyticsAuthErr(c, err)
	}

	return c.JSON(fiber.Map{"data": analytics})
}

func (h *AnalyticsHandler) GetAssignmentAnalytics(c *fiber.Ctx) error {
	assignmentID, err := uuid.Parse(c.Params("assignmentId"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid assignment_id"})
	}
	userID, ok := c.Locals("user_id").(uuid.UUID)
	if !ok {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"message": "Unauthorized"})
	}
	isAdmin := isAdminActor(c, h.permChecker, userID)

	analytics, err := h.svc.GetAssignmentAnalytics(c.Context(), assignmentID, userID, isAdmin)
	if err != nil {
		return requireAnalyticsAuthErr(c, err)
	}

	return c.JSON(fiber.Map{"data": analytics})
}

func (h *AnalyticsHandler) GetParticipantAnalytics(c *fiber.Ctx) error {
	sessionID, err := uuid.Parse(c.Params("sessionId"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid session_id"})
	}
	userID, ok := c.Locals("user_id").(uuid.UUID)
	if !ok {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"message": "Unauthorized"})
	}
	isAdmin := isAdminActor(c, h.permChecker, userID)

	analytics, err := h.svc.GetParticipantAnalytics(c.Context(), sessionID, userID, isAdmin)
	if err != nil {
		return requireAnalyticsAuthErr(c, err)
	}

	return c.JSON(fiber.Map{"data": analytics})
}
