package handler

import (
	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"study.com/v1/internal/dto"
	"study.com/v1/internal/middleware"
	"study.com/v1/internal/service"
	"study.com/v1/internal/utils"
)

type ExerciseHandler struct {
	service     service.ExerciseServiceInterface
	permChecker *middleware.PermissionChecker
}

func NewExerciseHandler(service service.ExerciseServiceInterface, permChecker *middleware.PermissionChecker) *ExerciseHandler {
	return &ExerciseHandler{service: service, permChecker: permChecker}
}

// canManageExercise (S2): người gọi là admin, người tạo hoặc giảng viên chủ khoá đang gắn bài tập.
// Quyết định ai được đọc test case ẩn / bài nộp của người khác / sửa bài tập. Lỗi tra cứu quyền
// coi như KHÔNG có quyền (fail-closed).
func (h *ExerciseHandler) canManageExercise(c *fiber.Ctx, exerciseID uuid.UUID) bool {
	userID, ok := c.Locals("user_id").(uuid.UUID)
	if !ok {
		return false
	}
	allowed, err := h.service.CanManage(c.Context(), exerciseID, userID, isAdminActor(c, h.permChecker, userID))
	return err == nil && allowed
}

// requireExerciseManager ghi 403 và trả ok=false khi người gọi không quản lý bài tập; caller chỉ
// cần `return err` (nil sau khi đã ghi response).
func (h *ExerciseHandler) requireExerciseManager(c *fiber.Ctx, exerciseID uuid.UUID) (bool, error) {
	if h.canManageExercise(c, exerciseID) {
		return true, nil
	}
	return false, c.Status(fiber.StatusForbidden).JSON(fiber.Map{
		"message": "Forbidden",
		"error":   "only the exercise owner can perform this action",
	})
}

// canAuthorExercise: tạo bài tập mới chỉ dành cho admin hoặc người có quyền sửa khoá học của mình
// (giảng viên). Học viên không được tạo bài tập/test case.
func (h *ExerciseHandler) canAuthorExercise(c *fiber.Ctx, userID uuid.UUID) bool {
	if isAdminActor(c, h.permChecker, userID) {
		return true
	}
	if h.permChecker == nil {
		return false
	}
	var activeOrgID *uuid.UUID
	if orgID, ok := c.Locals("active_org_id").(uuid.UUID); ok {
		activeOrgID = &orgID
	}
	ok, err := h.permChecker.HasPermission(c.Context(), userID, activeOrgID, "COURSES_UPDATE_OWN")
	return err == nil && ok
}

// ============================================================================
// EXERCISE CRUD
// ============================================================================

func (h *ExerciseHandler) CreateExercise(c *fiber.Ctx) error {
	userID, ok := c.Locals("user_id").(uuid.UUID)
	if !ok {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"message": "Unauthorized"})
	}
	if !h.canAuthorExercise(c, userID) {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{
			"message": "Forbidden",
			"error":   "only teachers and admins can create exercises",
		})
	}

	var req dto.CreateExerciseDTO
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

	exercise, err := h.service.CreateExercise(c.Context(), userID, req)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Failed to create exercise",
			"error":   err.Error(),
		})
	}

	return c.Status(fiber.StatusCreated).JSON(fiber.Map{
		"message": "Exercise created successfully",
		"data":    exercise,
	})
}

func (h *ExerciseHandler) GetExerciseByID(c *fiber.Ctx) error {
	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Invalid exercise ID",
			"error":   err.Error(),
		})
	}

	// S2: test case ẩn chỉ trả cho người quản lý bài tập; học viên chỉ nhận test mẫu.
	exercise, err := h.service.GetExerciseByID(c.Context(), id, h.canManageExercise(c, id))
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"message": "Exercise not found",
			"error":   err.Error(),
		})
	}

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"message": "Exercise retrieved successfully",
		"data":    exercise,
	})
}

func (h *ExerciseHandler) ListExercises(c *fiber.Ctx) error {
	page := c.QueryInt("page", 1)
	pageSize := c.QueryInt("page_size", 20)

	exercises, err := h.service.ListExercises(c.Context(), page, pageSize)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"message": "Failed to retrieve exercises",
			"error":   err.Error(),
		})
	}

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"message": "Exercises retrieved successfully",
		"data":    exercises,
	})
}

func (h *ExerciseHandler) UpdateExercise(c *fiber.Ctx) error {
	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Invalid exercise ID",
			"error":   err.Error(),
		})
	}

	if ok, err := h.requireExerciseManager(c, id); !ok {
		return err
	}

	var req dto.UpdateExerciseDTO
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

	exercise, err := h.service.UpdateExercise(c.Context(), id, req)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Failed to update exercise",
			"error":   err.Error(),
		})
	}

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"message": "Exercise updated successfully",
		"data":    exercise,
	})
}

func (h *ExerciseHandler) DeleteExercise(c *fiber.Ctx) error {
	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Invalid exercise ID",
			"error":   err.Error(),
		})
	}

	if ok, err := h.requireExerciseManager(c, id); !ok {
		return err
	}

	if err := h.service.DeleteExercise(c.Context(), id); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Failed to delete exercise",
			"error":   err.Error(),
		})
	}

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"message": "Exercise deleted successfully",
	})
}

// ============================================================================
// TEST CASES
// ============================================================================

func (h *ExerciseHandler) CreateTestCase(c *fiber.Ctx) error {
	exerciseID, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Invalid exercise ID",
			"error":   err.Error(),
		})
	}

	if ok, err := h.requireExerciseManager(c, exerciseID); !ok {
		return err
	}

	var req dto.CreateExerciseTestCaseDTO
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

	testCase, err := h.service.CreateTestCase(c.Context(), exerciseID, req)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Failed to create test case",
			"error":   err.Error(),
		})
	}

	return c.Status(fiber.StatusCreated).JSON(fiber.Map{
		"message": "Test case created successfully",
		"data":    testCase,
	})
}

func (h *ExerciseHandler) GetTestCases(c *fiber.Ctx) error {
	exerciseID, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Invalid exercise ID",
			"error":   err.Error(),
		})
	}

	testCases, err := h.service.GetTestCases(c.Context(), exerciseID, h.canManageExercise(c, exerciseID))
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"message": "Failed to retrieve test cases",
			"error":   err.Error(),
		})
	}

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"message": "Test cases retrieved successfully",
		"data":    testCases,
	})
}

func (h *ExerciseHandler) ImportTestCases(c *fiber.Ctx) error {
	exerciseID, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Invalid exercise ID",
			"error":   err.Error(),
		})
	}

	if ok, err := h.requireExerciseManager(c, exerciseID); !ok {
		return err
	}

	var req dto.ImportExerciseTestCasesDTO
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

	testCases, err := h.service.ImportTestCases(c.Context(), exerciseID, req)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Failed to import test cases",
			"error":   err.Error(),
		})
	}

	return c.Status(fiber.StatusCreated).JSON(fiber.Map{
		"message": "Test cases imported successfully",
		"data":    testCases,
	})
}

func (h *ExerciseHandler) DeleteTestCase(c *fiber.Ctx) error {
	exerciseID, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Invalid exercise ID",
			"error":   err.Error(),
		})
	}

	if ok, err := h.requireExerciseManager(c, exerciseID); !ok {
		return err
	}

	testCaseID, err := uuid.Parse(c.Params("testCaseId"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Invalid test case ID",
			"error":   err.Error(),
		})
	}

	if err := h.service.DeleteTestCase(c.Context(), exerciseID, testCaseID); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Failed to delete test case",
			"error":   err.Error(),
		})
	}

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"message": "Test case deleted successfully",
	})
}

// ============================================================================
// SUBMISSIONS
// ============================================================================

func (h *ExerciseHandler) SubmitExercise(c *fiber.Ctx) error {
	exerciseID, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Invalid exercise ID",
			"error":   err.Error(),
		})
	}

	userID, ok := c.Locals("user_id").(uuid.UUID)
	if !ok {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"message": "Unauthorized",
		})
	}

	var req dto.SubmitExerciseDTO
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

	submission, err := h.service.SubmitExercise(c.Context(), exerciseID, userID, req)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Failed to submit exercise",
			"error":   err.Error(),
		})
	}

	return c.Status(fiber.StatusCreated).JSON(fiber.Map{
		"message": "Exercise submitted successfully",
		"data":    submission,
	})
}

func (h *ExerciseHandler) GetSubmissions(c *fiber.Ctx) error {
	exerciseID, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Invalid exercise ID",
			"error":   err.Error(),
		})
	}

	// S2: danh sách bài nộp của TẤT CẢ học viên (kèm mã nguồn) chỉ dành cho người quản lý bài tập.
	if ok, err := h.requireExerciseManager(c, exerciseID); !ok {
		return err
	}

	page := c.QueryInt("page", 1)
	pageSize := c.QueryInt("page_size", 20)

	submissions, err := h.service.GetSubmissions(c.Context(), exerciseID, page, pageSize)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"message": "Failed to retrieve submissions",
			"error":   err.Error(),
		})
	}

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"message": "Submissions retrieved successfully",
		"data":    submissions,
	})
}

func (h *ExerciseHandler) GetMySubmissions(c *fiber.Ctx) error {
	exerciseID, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Invalid exercise ID",
			"error":   err.Error(),
		})
	}

	userID, ok := c.Locals("user_id").(uuid.UUID)
	if !ok {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"message": "Unauthorized",
		})
	}

	page := c.QueryInt("page", 1)
	pageSize := c.QueryInt("page_size", 20)

	submissions, err := h.service.GetMySubmissions(c.Context(), exerciseID, userID, page, pageSize)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"message": "Failed to retrieve submissions",
			"error":   err.Error(),
		})
	}

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"message": "Submissions retrieved successfully",
		"data":    submissions,
	})
}

func (h *ExerciseHandler) GetSubmissionByID(c *fiber.Ctx) error {
	submissionID, err := uuid.Parse(c.Params("submissionId"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Invalid submission ID",
			"error":   err.Error(),
		})
	}

	requesterID, ok := c.Locals("user_id").(uuid.UUID)
	if !ok {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"message": "Unauthorized"})
	}

	// S2: chỉ chủ bài nộp hoặc người quản lý bài tập; còn lại nhận 404 như bài nộp không tồn tại.
	submission, err := h.service.GetSubmissionByID(c.Context(), submissionID, requesterID, isAdminActor(c, h.permChecker, requesterID))
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"message": "Submission not found",
			"error":   err.Error(),
		})
	}

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"message": "Submission retrieved successfully",
		"data":    submission,
	})
}

// ============================================================================
// CONTENT PROGRESS
// ============================================================================

func (h *ExerciseHandler) UpdateContentProgress(c *fiber.Ctx) error {
	userID, ok := c.Locals("user_id").(uuid.UUID)
	if !ok {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"message": "Unauthorized",
		})
	}

	var body struct {
		LessonContentID string                    `json:"lesson_content_id" validate:"required,uuid"`
		dto.UpdateContentProgressDTO
	}
	if err := c.BodyParser(&body); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Invalid request body",
			"error":   err.Error(),
		})
	}

	lessonContentID, err := uuid.Parse(body.LessonContentID)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Invalid lesson content ID",
			"error":   err.Error(),
		})
	}

	progress, err := h.service.UpdateContentProgress(c.Context(), userID, lessonContentID, body.UpdateContentProgressDTO)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Failed to update content progress",
			"error":   err.Error(),
		})
	}

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"message": "Content progress updated successfully",
		"data":    progress,
	})
}

func (h *ExerciseHandler) GetContentProgress(c *fiber.Ctx) error {
	lessonContentID, err := uuid.Parse(c.Params("lessonContentId"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Invalid lesson content ID",
			"error":   err.Error(),
		})
	}

	userID, ok := c.Locals("user_id").(uuid.UUID)
	if !ok {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"message": "Unauthorized",
		})
	}

	progress, err := h.service.GetContentProgress(c.Context(), userID, lessonContentID)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"message": "Content progress not found",
			"error":   err.Error(),
		})
	}

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"message": "Content progress retrieved successfully",
		"data":    progress,
	})
}
