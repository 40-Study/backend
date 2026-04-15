package handler

import (
	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"study.com/v1/internal/dto"
	"study.com/v1/internal/service"
	"study.com/v1/internal/utils"
)

type QuizHandler struct {
	service service.QuizServiceInterface
}

func NewQuizHandler(service service.QuizServiceInterface) *QuizHandler {
	return &QuizHandler{service: service}
}

// ============================================================================
// QUIZ CRUD
// ============================================================================

func (h *QuizHandler) CreateQuiz(c *fiber.Ctx) error {
	var req dto.CreateQuizDTO
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

	quiz, err := h.service.CreateQuiz(c.Context(), req)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Failed to create quiz",
			"error":   err.Error(),
		})
	}

	return c.Status(fiber.StatusCreated).JSON(fiber.Map{
		"message": "Quiz created successfully",
		"data":    quiz,
	})
}

func (h *QuizHandler) GetAllQuizzes(c *fiber.Ctx) error {
	var lessonID, courseID, sessionID *uuid.UUID

	if lid := c.Query("lesson_id"); lid != "" {
		parsed, err := uuid.Parse(lid)
		if err == nil {
			lessonID = &parsed
		}
	}
	if cid := c.Query("course_id"); cid != "" {
		parsed, err := uuid.Parse(cid)
		if err == nil {
			courseID = &parsed
		}
	}
	if sid := c.Query("session_id"); sid != "" {
		parsed, err := uuid.Parse(sid)
		if err == nil {
			sessionID = &parsed
		}
	}

	page := c.QueryInt("page", 1)
	pageSize := c.QueryInt("page_size", 20)

	quizzes, err := h.service.GetAllQuizzes(c.Context(), lessonID, courseID, sessionID, page, pageSize)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"message": "Failed to retrieve quizzes",
			"error":   err.Error(),
		})
	}

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"message": "Quizzes retrieved successfully",
		"data":    quizzes,
	})
}

func (h *QuizHandler) GetQuizByID(c *fiber.Ctx) error {
	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Invalid quiz ID",
			"error":   err.Error(),
		})
	}

	quiz, err := h.service.GetQuizByID(c.Context(), id)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"message": "Quiz not found",
			"error":   err.Error(),
		})
	}

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"message": "Quiz retrieved successfully",
		"data":    quiz,
	})
}

func (h *QuizHandler) UpdateQuiz(c *fiber.Ctx) error {
	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Invalid quiz ID",
			"error":   err.Error(),
		})
	}

	var req dto.UpdateQuizDTO
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

	quiz, err := h.service.UpdateQuiz(c.Context(), id, req)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Failed to update quiz",
			"error":   err.Error(),
		})
	}

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"message": "Quiz updated successfully",
		"data":    quiz,
	})
}

func (h *QuizHandler) DeleteQuiz(c *fiber.Ctx) error {
	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Invalid quiz ID",
			"error":   err.Error(),
		})
	}

	if err := h.service.DeleteQuiz(c.Context(), id); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Failed to delete quiz",
			"error":   err.Error(),
		})
	}

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"message": "Quiz deleted successfully",
	})
}

func (h *QuizHandler) DuplicateQuiz(c *fiber.Ctx) error {
	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Invalid quiz ID",
			"error":   err.Error(),
		})
	}

	quiz, err := h.service.DuplicateQuiz(c.Context(), id)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Failed to duplicate quiz",
			"error":   err.Error(),
		})
	}

	return c.Status(fiber.StatusCreated).JSON(fiber.Map{
		"message": "Quiz duplicated successfully",
		"data":    quiz,
	})
}

// ============================================================================
// QUESTIONS
// ============================================================================

func (h *QuizHandler) CreateQuestion(c *fiber.Ctx) error {
	quizID, err := uuid.Parse(c.Params("quizId"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Invalid quiz ID",
			"error":   err.Error(),
		})
	}

	var req dto.CreateQuestionDTO
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

	question, err := h.service.CreateQuestion(c.Context(), quizID, req)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Failed to create question",
			"error":   err.Error(),
		})
	}

	return c.Status(fiber.StatusCreated).JSON(fiber.Map{
		"message": "Question created successfully",
		"data":    question,
	})
}

func (h *QuizHandler) GetQuestionsByQuiz(c *fiber.Ctx) error {
	quizID, err := uuid.Parse(c.Params("quizId"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Invalid quiz ID",
			"error":   err.Error(),
		})
	}

	questions, err := h.service.GetQuestionsByQuiz(c.Context(), quizID)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"message": "Failed to retrieve questions",
			"error":   err.Error(),
		})
	}

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"message": "Questions retrieved successfully",
		"data":    questions,
	})
}

func (h *QuizHandler) UpdateQuestion(c *fiber.Ctx) error {
	quizID, err := uuid.Parse(c.Params("quizId"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Invalid quiz ID",
			"error":   err.Error(),
		})
	}

	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Invalid question ID",
			"error":   err.Error(),
		})
	}

	var req dto.UpdateQuestionDTO
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

	question, err := h.service.UpdateQuestion(c.Context(), quizID, id, req)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Failed to update question",
			"error":   err.Error(),
		})
	}

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"message": "Question updated successfully",
		"data":    question,
	})
}

func (h *QuizHandler) DeleteQuestion(c *fiber.Ctx) error {
	quizID, err := uuid.Parse(c.Params("quizId"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Invalid quiz ID",
			"error":   err.Error(),
		})
	}

	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Invalid question ID",
			"error":   err.Error(),
		})
	}

	if err := h.service.DeleteQuestion(c.Context(), quizID, id); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Failed to delete question",
			"error":   err.Error(),
		})
	}

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"message": "Question deleted successfully",
	})
}

func (h *QuizHandler) ReorderQuestions(c *fiber.Ctx) error {
	quizID, err := uuid.Parse(c.Params("quizId"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Invalid quiz ID",
			"error":   err.Error(),
		})
	}

	var req dto.ReorderQuestionsDTO
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

	if err := h.service.ReorderQuestions(c.Context(), quizID, req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Failed to reorder questions",
			"error":   err.Error(),
		})
	}

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"message": "Questions reordered successfully",
	})
}

func (h *QuizHandler) BulkCreateQuestions(c *fiber.Ctx) error {
	quizID, err := uuid.Parse(c.Params("quizId"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Invalid quiz ID",
			"error":   err.Error(),
		})
	}

	var req dto.BulkCreateQuestionsDTO
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

	questions, err := h.service.BulkCreateQuestions(c.Context(), quizID, req)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Failed to create questions",
			"error":   err.Error(),
		})
	}

	return c.Status(fiber.StatusCreated).JSON(fiber.Map{
		"message": "Questions created successfully",
		"data":    questions,
	})
}

// ============================================================================
// QUIZ ATTEMPTS
// ============================================================================

func (h *QuizHandler) StartQuiz(c *fiber.Ctx) error {
	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Invalid quiz ID",
			"error":   err.Error(),
		})
	}

	userID, ok := c.Locals("user_id").(uuid.UUID)
	if !ok {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"message": "Unauthorized",
		})
	}

	result, err := h.service.StartQuiz(c.Context(), id, userID)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Failed to start quiz",
			"error":   err.Error(),
		})
	}

	return c.Status(fiber.StatusCreated).JSON(fiber.Map{
		"message": "Quiz started successfully",
		"data":    result,
	})
}

func (h *QuizHandler) SubmitQuiz(c *fiber.Ctx) error {
	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Invalid quiz ID",
			"error":   err.Error(),
		})
	}

	userID, ok := c.Locals("user_id").(uuid.UUID)
	if !ok {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"message": "Unauthorized",
		})
	}

	var req dto.SubmitQuizDTO
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

	result, err := h.service.SubmitQuiz(c.Context(), id, userID, req)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Failed to submit quiz",
			"error":   err.Error(),
		})
	}

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"message": "Quiz submitted successfully",
		"data":    result,
	})
}

func (h *QuizHandler) GetMyAttempts(c *fiber.Ctx) error {
	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Invalid quiz ID",
			"error":   err.Error(),
		})
	}

	userID, ok := c.Locals("user_id").(uuid.UUID)
	if !ok {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"message": "Unauthorized",
		})
	}

	attempts, err := h.service.GetMyAttempts(c.Context(), id, userID)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"message": "Failed to retrieve attempts",
			"error":   err.Error(),
		})
	}

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"message": "Attempts retrieved successfully",
		"data":    attempts,
	})
}

func (h *QuizHandler) GetAttemptByID(c *fiber.Ctx) error {
	_, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Invalid quiz ID",
			"error":   err.Error(),
		})
	}

	attemptID, err := uuid.Parse(c.Params("attemptId"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Invalid attempt ID",
			"error":   err.Error(),
		})
	}

	userID, ok := c.Locals("user_id").(uuid.UUID)
	if !ok {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"message": "Unauthorized",
		})
	}

	attempt, err := h.service.GetAttemptByID(c.Context(), attemptID, userID)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"message": "Attempt not found",
			"error":   err.Error(),
		})
	}

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"message": "Attempt retrieved successfully",
		"data":    attempt,
	})
}

func (h *QuizHandler) GetQuizResults(c *fiber.Ctx) error {
	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Invalid quiz ID",
			"error":   err.Error(),
		})
	}

	results, err := h.service.GetQuizResults(c.Context(), id)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"message": "Failed to retrieve quiz results",
			"error":   err.Error(),
		})
	}

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"message": "Quiz results retrieved successfully",
		"data":    results,
	})
}

func (h *QuizHandler) GetQuizStatistics(c *fiber.Ctx) error {
	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Invalid quiz ID",
			"error":   err.Error(),
		})
	}

	stats, err := h.service.GetQuizStatistics(c.Context(), id)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"message": "Failed to retrieve quiz statistics",
			"error":   err.Error(),
		})
	}

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"message": "Quiz statistics retrieved successfully",
		"data":    stats,
	})
}

func (h *QuizHandler) SaveAnswer(c *fiber.Ctx) error {
	attemptID, err := uuid.Parse(c.Params("attemptId"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Invalid attempt ID",
			"error":   err.Error(),
		})
	}

	userID, ok := c.Locals("user_id").(uuid.UUID)
	if !ok {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"message": "Unauthorized",
		})
	}

	var req dto.SaveAnswerDTO
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

	if err := h.service.SaveAnswer(c.Context(), attemptID, userID, req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Failed to save answer",
			"error":   err.Error(),
		})
	}

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"message": "Answer saved successfully",
	})
}

func (h *QuizHandler) GetAttemptProgress(c *fiber.Ctx) error {
	attemptID, err := uuid.Parse(c.Params("attemptId"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Invalid attempt ID",
			"error":   err.Error(),
		})
	}

	userID, ok := c.Locals("user_id").(uuid.UUID)
	if !ok {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"message": "Unauthorized",
		})
	}

	progress, err := h.service.GetAttemptProgress(c.Context(), attemptID, userID)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"message": "Attempt not found",
			"error":   err.Error(),
		})
	}

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"message": "Attempt progress retrieved successfully",
		"data":    progress,
	})
}

// ============================================================================
// MY QUIZZES
// ============================================================================

func (h *QuizHandler) GetMyCreatedQuizzes(c *fiber.Ctx) error {
	userID, ok := c.Locals("user_id").(uuid.UUID)
	if !ok {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"message": "Unauthorized",
		})
	}

	page := c.QueryInt("page", 1)
	pageSize := c.QueryInt("page_size", 20)

	quizzes, err := h.service.GetMyCreatedQuizzes(c.Context(), userID, page, pageSize)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"message": "Failed to retrieve quizzes",
			"error":   err.Error(),
		})
	}

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"message": "Quizzes retrieved successfully",
		"data":    quizzes,
	})
}

func (h *QuizHandler) GetMyQuizHistory(c *fiber.Ctx) error {
	userID, ok := c.Locals("user_id").(uuid.UUID)
	if !ok {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"message": "Unauthorized",
		})
	}

	page := c.QueryInt("page", 1)
	pageSize := c.QueryInt("page_size", 20)

	attempts, err := h.service.GetMyQuizHistory(c.Context(), userID, page, pageSize)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"message": "Failed to retrieve quiz history",
			"error":   err.Error(),
		})
	}

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"message": "Quiz history retrieved successfully",
		"data":    attempts,
	})
}
