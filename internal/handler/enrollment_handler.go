package handler

import (
	"encoding/json"
	"fmt"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"study.com/v1/internal/dto"
	"study.com/v1/internal/middleware"
	"study.com/v1/internal/service"
	"study.com/v1/internal/utils"
)

type EnrollmentHandler struct {
	service service.EnrollmentServiceInterface
	// permChecker (F3, audit 260917-live): can de tinh isAdmin qua isAdminActor, truyen xuong
	// EnrollmentService.UpdateLessonProgress lam bypass luat khoa tuan tu — co the nil (test
	// khong truyen), isAdminActor fail-closed.
	permChecker *middleware.PermissionChecker
}

func NewEnrollmentHandler(service service.EnrollmentServiceInterface, permChecker *middleware.PermissionChecker) *EnrollmentHandler {
	return &EnrollmentHandler{service: service, permChecker: permChecker}
}

func (h *EnrollmentHandler) Enroll(c *fiber.Ctx) error {
	courseID, err := uuid.Parse(c.Params("courseId"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Invalid course ID", "error": err.Error(),
		})
	}

	userID, err := extractUserID(c)
	if err != nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"message": "Unauthorized", "error": err.Error(),
		})
	}

	enrollment, err := h.service.Enroll(c.Context(), userID, courseID)
	if err != nil {
		if err == service.ErrPaymentRequired {
			return c.Status(fiber.StatusPaymentRequired).JSON(fiber.Map{
				"message": "Khóa học trả phí, vui lòng thanh toán qua đơn hàng",
			})
		}
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Failed to enroll", "error": err.Error(),
		})
	}

	return c.Status(fiber.StatusCreated).JSON(fiber.Map{
		"message": "Enrolled successfully", "data": enrollment,
	})
}

func (h *EnrollmentHandler) Unenroll(c *fiber.Ctx) error {
	courseID, err := uuid.Parse(c.Params("courseId"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Invalid course ID", "error": err.Error(),
		})
	}

	userID, err := extractUserID(c)
	if err != nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"message": "Unauthorized", "error": err.Error(),
		})
	}

	if err := h.service.Unenroll(c.Context(), userID, courseID); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Failed to unenroll", "error": err.Error(),
		})
	}

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"message": "Unenrolled successfully",
	})
}

func (h *EnrollmentHandler) GetMyEnrollments(c *fiber.Ctx) error {
	userID, err := extractUserID(c)
	if err != nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"message": "Unauthorized", "error": err.Error(),
		})
	}

	page := c.QueryInt("page", 1)
	pageSize := c.QueryInt("page_size", 20)

	enrollments, err := h.service.GetMyEnrollments(c.Context(), userID, page, pageSize)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"message": "Failed to retrieve enrollments", "error": err.Error(),
		})
	}

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"message": "Enrollments retrieved successfully", "data": enrollments,
	})
}

func (h *EnrollmentHandler) GetEnrollmentDetail(c *fiber.Ctx) error {
	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Invalid enrollment ID", "error": err.Error(),
		})
	}

	userID, err := extractUserID(c)
	if err != nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"message": "Unauthorized", "error": err.Error(),
		})
	}

	detail, err := h.service.GetEnrollmentDetail(c.Context(), id, userID)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"message": "Enrollment not found", "error": err.Error(),
		})
	}

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"message": "Enrollment retrieved successfully", "data": detail,
	})
}

func (h *EnrollmentHandler) UpdateLessonProgress(c *fiber.Ctx) error {
	lessonID, err := uuid.Parse(c.Params("lessonId"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Invalid lesson ID", "error": err.Error(),
		})
	}

	userID, err := extractUserID(c)
	if err != nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"message": "Unauthorized", "error": err.Error(),
		})
	}

	var req dto.UpdateLessonProgressDTO
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Invalid request body", "error": err.Error(),
		})
	}

	if errors := utils.ValidateStruct(req); len(errors) > 0 {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Validation failed",
			"errors":  errors,
		})
	}

	isAdmin := isAdminActor(c, h.permChecker, userID)
	progress, err := h.service.UpdateLessonProgress(c.Context(), userID, lessonID, req, isAdmin)
	if err != nil {
		// F3 (audit 260917-live): bai dang bi khoa boi luat hoc tuan tu -> 403, giong het cach
		// LessonContentHandler.GetContent map ErrLessonLocked (xem lesson_content_handler.go).
		if err == service.ErrLessonLocked {
			return c.Status(fiber.StatusForbidden).JSON(fiber.Map{
				"message": "LESSON_LOCKED",
			})
		}
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Failed to update progress", "error": err.Error(),
		})
	}

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"message": "Progress updated successfully", "data": progress,
	})
}

// TrackProgressBeacon nhan tien do do trinh phat gui bang
// `navigator.sendBeacon("/api/progress", ...)` khi nguoi hoc dong tab hoac roi trang
// (web: app/courses/[slug]/learn/player-client.tsx, handler beforeunload).
//
// Truoc ban va nay backend khong co route `/progress` nao ca (progress_router.go bi
// comment toan bo vi ProgressHandler chua duoc viet), nen moi beacon nhan 404 va
// tien do cua giay phut cuoi truoc khi dong tab bi mat am tham.
//
// Khong viet lai ProgressHandler cho viec nay: luong ghi da co san va da duoc lam
// chi-tang trong service.UpdateLessonProgress, nen o day chi can mot adapter mong
// dich body camelCase cua beacon sang DTO snake_case roi goi dung service do.
// Mot ProgressHandler rieng se la ban sao thu hai cua cung mot logic ghi.
func (h *EnrollmentHandler) TrackProgressBeacon(c *fiber.Ctx) error {
	userID, err := extractUserID(c)
	if err != nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"message": "Unauthorized", "error": err.Error(),
		})
	}

	// KHONG dung c.BodyParser o day: sendBeacon gui Content-Type
	// "text/plain;charset=UTF-8" va KHONG cho doi header, con BodyParser cua Fiber
	// tu choi content-type do voi 422 Unprocessable Entity. Unmarshal body truc tiep.
	var req dto.BeaconProgressDTO
	if err := json.Unmarshal(c.Body(), &req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Invalid request body", "error": err.Error(),
		})
	}

	if errors := utils.ValidateStruct(req); len(errors) > 0 {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Validation failed", "errors": errors,
		})
	}

	// Kiem tra CO MAT cua lesson id o tang handler chu khong dung `validate:"required,uuid"`:
	// beacon Phase 1 gui `lesson_id` (snake_case) con ban truoc Phase 1 gui `lessonId`
	// (camelCase) — hai truong khac ten nen khong the danh dau required cho tung truong, va
	// danh dau required cho CA HAI se tu choi chinh nhung request hop le cua ban con lai.
	//
	// Thieu ca hai KHONG duoc coi la "khong doi gi": do la mot client hong, phai tra 400 de
	// khong am tham danh roi tien do cua nguoi hoc.
	rawLessonID := req.ResolvedLessonID()
	if rawLessonID == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Validation failed",
			"errors":  []string{"lesson_id là bắt buộc"},
		})
	}
	lessonID, err := uuid.Parse(rawLessonID)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Invalid lesson ID", "error": err.Error(),
		})
	}

	isAdmin := isAdminActor(c, h.permChecker, userID)
	progress, err := h.service.UpdateLessonProgress(c.Context(), userID, lessonID, req.ToUpdateDTO(), isAdmin)
	if err != nil {
		// F3 (audit 260917-live): beacon di qua CUNG mot service, phai map loi khoa giong het
		// nhanh PUT /lessons/:lessonId/progress o tren.
		if err == service.ErrLessonLocked {
			return c.Status(fiber.StatusForbidden).JSON(fiber.Map{
				"message": "LESSON_LOCKED",
			})
		}
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Failed to update progress", "error": err.Error(),
		})
	}

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"message": "Progress updated successfully", "data": progress,
	})
}

// GetCourseEnrollments returns enrollments for a course (instructor view)
func (h *EnrollmentHandler) GetCourseEnrollments(c *fiber.Ctx) error {
	courseID, err := uuid.Parse(c.Params("courseId"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Invalid course ID", "error": err.Error(),
		})
	}

	page := c.QueryInt("page", 1)
	pageSize := c.QueryInt("page_size", 20)

	enrollments, err := h.service.GetCourseEnrollments(c.Context(), courseID, page, pageSize)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"message": "Failed to retrieve enrollments", "error": err.Error(),
		})
	}

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"message": "Enrollments retrieved successfully", "data": enrollments,
	})
}

// DebugCourseEnrollments returns all enrollments including soft-deleted (for debugging)
func (h *EnrollmentHandler) DebugCourseEnrollments(c *fiber.Ctx) error {
	courseID, err := uuid.Parse(c.Params("courseId"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Invalid course ID", "error": err.Error(),
		})
	}

	enrollments, err := h.service.DebugGetCourseEnrollments(c.Context(), courseID)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"message": "Failed to retrieve enrollments", "error": err.Error(),
		})
	}

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"message": "Debug enrollments retrieved", "data": enrollments,
	})
}

func extractUserID(c *fiber.Ctx) (uuid.UUID, error) {
	if userID, ok := c.Locals("user_id").(uuid.UUID); ok {
		return userID, nil
	}
	if userIDStr, ok := c.Locals("user_id").(string); ok {
		return uuid.Parse(userIDStr)
	}
	return uuid.Nil, fmt.Errorf("user ID not found in context")
}
