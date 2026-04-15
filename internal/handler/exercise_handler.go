package handler

import (
	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"study.com/v1/internal/dto"
	"study.com/v1/internal/service"
	"study.com/v1/internal/utils"
)

type ExerciseHandler struct {
	service service.ExerciseServiceInterface
}

func NewExerciseHandler(service service.ExerciseServiceInterface) *ExerciseHandler {
	return &ExerciseHandler{service: service}
}

// ============================================================================
// EXERCISE CRUD
// ============================================================================

func (h *ExerciseHandler) CreateExercise(c *fiber.Ctx) error {
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

	exercise, err := h.service.CreateExercise(c.Context(), req)
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

	exercise, err := h.service.GetExerciseByID(c.Context(), id)
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

	testCases, err := h.service.GetTestCases(c.Context(), exerciseID)
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

	submission, err := h.service.GetSubmissionByID(c.Context(), submissionID)
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
