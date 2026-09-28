package handler

import (
	"errors"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"study.com/v1/internal/repository"
	"study.com/v1/internal/service"
)

// Phase 3 — nửa "duyệt giáo viên đăng ký" của ApprovalHandler (approval_handler.go).

func teacherApplicationError(c *fiber.Ctx, err error) error {
	if handled, resp := reasonErrorResponse(c, err); handled {
		return resp
	}
	switch {
	case errors.Is(err, repository.ErrTeacherApplicationNotFound), errors.Is(err, service.ErrTeacherProfileNotFound):
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"message": "Teacher application not found"})
	case errors.Is(err, repository.ErrTeacherApplicationNotPending):
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"message": "Application is not pending", "code": "APPLICATION_NOT_PENDING"})
	case errors.Is(err, repository.ErrTeacherApplicationNotRejected):
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"message": "Application is not rejected", "code": "APPLICATION_NOT_REJECTED"})
	case errors.Is(err, repository.ErrTeacherResubmissionLimit):
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Resubmission limit reached — please contact support", "code": "RESUBMISSION_LIMIT_REACHED",
		})
	}
	return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"message": "Failed to process teacher application", "error": err.Error()})
}

// ListTeacherApplications — GET /api/admin/teacher-applications?status=&keyword=&page=&limit=
func (h *ApprovalHandler) ListTeacherApplications(c *fiber.Ctx) error {
	result, err := h.teacherApp.List(c.Context(), c.Query("status"), c.Query("keyword"),
		c.QueryInt("page", 1), c.QueryInt("limit", 20))
	if err != nil {
		if errors.Is(err, service.ErrInvalidReviewFilter) {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
				"message": "Invalid query params", "errors": []string{"status must be 'pending', 'approved' or 'rejected'"},
			})
		}
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"message": "Failed to list teacher applications", "error": err.Error()})
	}
	return c.JSON(fiber.Map{"message": "Success", "data": result})
}

// ApproveTeacherApplication — POST /api/admin/teacher-applications/:userId/approve
func (h *ApprovalHandler) ApproveTeacherApplication(c *fiber.Ctx) error {
	userID, err := uuid.Parse(c.Params("userId"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"message": "Invalid user ID format"})
	}
	adminID, ok := actorFromLocals(c)
	if !ok {
		return unauthorized(c)
	}
	result, err := h.teacherApp.Approve(c.Context(), userID, adminID)
	if err != nil {
		return teacherApplicationError(c, err)
	}
	return c.JSON(fiber.Map{"message": "Teacher application approved", "data": result})
}

// RejectTeacherApplication — POST /api/admin/teacher-applications/:userId/reject  body {"reason"}
func (h *ApprovalHandler) RejectTeacherApplication(c *fiber.Ctx) error {
	userID, err := uuid.Parse(c.Params("userId"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"message": "Invalid user ID format"})
	}
	adminID, ok := actorFromLocals(c)
	if !ok {
		return unauthorized(c)
	}
	reason, err := parseRejectReason(c)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"message": "Invalid request body", "error": err.Error()})
	}
	result, err := h.teacherApp.Reject(c.Context(), userID, adminID, reason)
	if err != nil {
		return teacherApplicationError(c, err)
	}
	return c.JSON(fiber.Map{"message": "Teacher application rejected", "data": result})
}

// GetMyTeacherApplication — GET /api/teacher-profiles/me
func (h *ApprovalHandler) GetMyTeacherApplication(c *fiber.Ctx) error {
	userID, ok := actorFromLocals(c)
	if !ok {
		return unauthorized(c)
	}
	result, err := h.teacherApp.GetMine(c.Context(), userID)
	if err != nil {
		if errors.Is(err, service.ErrTeacherProfileNotFound) {
			return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"message": "Teacher profile not found"})
		}
		return teacherApplicationError(c, err)
	}
	return c.JSON(fiber.Map{"message": "Success", "data": result})
}

// ResubmitMyTeacherApplication — POST /api/teacher-profiles/me/resubmit
func (h *ApprovalHandler) ResubmitMyTeacherApplication(c *fiber.Ctx) error {
	userID, ok := actorFromLocals(c)
	if !ok {
		return unauthorized(c)
	}
	result, err := h.teacherApp.Resubmit(c.Context(), userID)
	if err != nil {
		if errors.Is(err, repository.ErrTeacherApplicationNotFound) {
			return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"message": "Teacher profile not found"})
		}
		return teacherApplicationError(c, err)
	}
	return c.JSON(fiber.Map{"message": "Teacher application resubmitted", "data": result})
}
