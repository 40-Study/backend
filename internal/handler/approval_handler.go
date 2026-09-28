package handler

import (
	"errors"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"study.com/v1/internal/dto"
	"study.com/v1/internal/repository"
	"study.com/v1/internal/service"
)

// Phase 3 duyệt khoá học + duyệt giáo viên (2026-09-28) — contract:
// plans/260927-2055-role-based-ux-qa/admin-features/phase-03-course-teacher-approval.md
// Một handler cho cả 2 luồng để router dùng chung chỉ thêm MỘT tham số (giảm xung đột merge với
// lane Phase 4 đang cùng thêm tham số vào SetupAllRoutes).
type ApprovalHandler struct {
	courseReview service.CourseReviewServiceInterface
	teacherApp   service.TeacherApplicationServiceInterface
}

func NewApprovalHandler(
	courseReview service.CourseReviewServiceInterface,
	teacherApp service.TeacherApplicationServiceInterface,
) *ApprovalHandler {
	return &ApprovalHandler{courseReview: courseReview, teacherApp: teacherApp}
}

func actorFromLocals(c *fiber.Ctx) (uuid.UUID, bool) {
	id, ok := c.Locals("user_id").(uuid.UUID)
	return id, ok && id != uuid.Nil
}

func unauthorized(c *fiber.Ctx) error {
	return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"message": "Unauthorized"})
}

func parseRejectReason(c *fiber.Ctx) (string, error) {
	var req dto.ReviewReasonRequestDTO
	if err := c.BodyParser(&req); err != nil {
		return "", err
	}
	return req.Reason, nil
}

// reasonErrorResponse ánh xạ lỗi lý do từ chối (dùng chung 2 luồng) — trả false nếu không phải.
func reasonErrorResponse(c *fiber.Ctx, err error) (bool, error) {
	switch {
	case errors.Is(err, service.ErrReviewReasonRequired):
		return true, c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Reason is required", "errors": []string{"reason is required"},
		})
	case errors.Is(err, service.ErrReviewReasonTooLong):
		return true, c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Reason is too long", "errors": []string{"reason must be at most 1000 characters"},
		})
	}
	return false, nil
}

func courseReviewError(c *fiber.Ctx, err error, invalidStatusMsg string) error {
	if handled, resp := reasonErrorResponse(c, err); handled {
		return resp
	}
	switch {
	case errors.Is(err, repository.ErrCourseReviewNotFound):
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"message": "Course not found"})
	case errors.Is(err, repository.ErrCourseReviewNotOwner):
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"message": "You are not the instructor of this course"})
	case errors.Is(err, repository.ErrCourseInvalidReviewStatus):
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"message": invalidStatusMsg, "code": "INVALID_COURSE_STATUS"})
	}
	return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"message": "Failed to process course review", "error": err.Error()})
}

// SubmitCourseForReview — POST /api/courses/:id/submit-review (giáo viên chủ khoá).
func (h *ApprovalHandler) SubmitCourseForReview(c *fiber.Ctx) error {
	courseID, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"message": "Invalid course ID"})
	}
	actorID, ok := actorFromLocals(c)
	if !ok {
		return unauthorized(c)
	}
	result, err := h.courseReview.SubmitForReview(c.Context(), courseID, actorID)
	if err != nil {
		return courseReviewError(c, err, "Course must be in draft or rejected status to submit")
	}
	return c.JSON(fiber.Map{"message": "Course submitted for review", "data": result})
}

// ListCoursesForReview — GET /api/admin/courses?status=&keyword=&page=&page_size=
func (h *ApprovalHandler) ListCoursesForReview(c *fiber.Ctx) error {
	result, err := h.courseReview.ListForReview(c.Context(), c.Query("status"), c.Query("keyword"),
		c.QueryInt("page", 1), c.QueryInt("page_size", 20))
	if err != nil {
		if errors.Is(err, service.ErrInvalidReviewFilter) {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
				"message": "Invalid query params", "errors": []string{"status is not a valid course status"},
			})
		}
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"message": "Failed to list courses", "error": err.Error()})
	}
	return c.JSON(fiber.Map{"message": "Success", "data": result})
}

// ApproveCourse — POST /api/admin/courses/:id/approve
func (h *ApprovalHandler) ApproveCourse(c *fiber.Ctx) error {
	courseID, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"message": "Invalid course ID"})
	}
	adminID, ok := actorFromLocals(c)
	if !ok {
		return unauthorized(c)
	}
	result, err := h.courseReview.Approve(c.Context(), courseID, adminID)
	if err != nil {
		return courseReviewError(c, err, "Course is not pending review")
	}
	return c.JSON(fiber.Map{"message": "Course approved", "data": result})
}

// RejectCourse — POST /api/admin/courses/:id/reject  body {"reason": "..."}
func (h *ApprovalHandler) RejectCourse(c *fiber.Ctx) error {
	courseID, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"message": "Invalid course ID"})
	}
	adminID, ok := actorFromLocals(c)
	if !ok {
		return unauthorized(c)
	}
	reason, err := parseRejectReason(c)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"message": "Invalid request body", "error": err.Error()})
	}
	result, err := h.courseReview.Reject(c.Context(), courseID, adminID, reason)
	if err != nil {
		return courseReviewError(c, err, "Course is not pending review")
	}
	return c.JSON(fiber.Map{"message": "Course rejected", "data": result})
}
