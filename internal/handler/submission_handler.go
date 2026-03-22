package handler

import (
	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"study.com/v1/internal/dto"
	"study.com/v1/internal/service"
)

type SubmissionHandlerInterface interface {
	Submit(c *fiber.Ctx) error
	GetByID(c *fiber.Ctx) error
	GetByAssignment(c *fiber.Ctx) error
	GetByUser(c *fiber.Ctx) error
	RunCode(c *fiber.Ctx) error
	RunCustomCode(c *fiber.Ctx) error
	ExecuteCode(c *fiber.Ctx) error
	GetMySubmissions(c *fiber.Ctx) error
}

type SubmissionHandler struct {
	svc service.SubmissionServiceInterface
}

func NewSubmissionHandler(svc service.SubmissionServiceInterface) *SubmissionHandler {
	return &SubmissionHandler{svc: svc}
}

func (h *SubmissionHandler) Submit(c *fiber.Ctx) error {
	var req dto.CreateSubmissionDTO
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}

	submission, err := h.svc.Submit(c.Context(), req)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}

	return c.Status(fiber.StatusCreated).JSON(fiber.Map{
		"message": "Submission created, processing...",
		"data":    submission,
	})
}

func (h *SubmissionHandler) GetByID(c *fiber.Ctx) error {
	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid id"})
	}

	submission, err := h.svc.GetByID(c.Context(), id)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
	if submission == nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "submission not found"})
	}

	return c.JSON(fiber.Map{"data": submission})
}

func (h *SubmissionHandler) GetByAssignment(c *fiber.Ctx) error {
	assignmentID, err := uuid.Parse(c.Params("assignmentId"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid assignment_id"})
	}

	page := c.QueryInt("page", 1)
	pageSize := c.QueryInt("page_size", 20)

	result, err := h.svc.GetByAssignment(c.Context(), assignmentID, page, pageSize)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(result)
}

func (h *SubmissionHandler) GetByUser(c *fiber.Ctx) error {
	userID, err := uuid.Parse(c.Params("userId"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid user_id"})
	}

	page := c.QueryInt("page", 1)
	pageSize := c.QueryInt("page_size", 20)

	result, err := h.svc.GetByUser(c.Context(), userID, page, pageSize)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(result)
}

func (h *SubmissionHandler) RunCode(c *fiber.Ctx) error {
	var req dto.RunCodeDTO
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}

	result, err := h.svc.RunCode(c.Context(), req)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(result)
}

func (h *SubmissionHandler) RunCustomCode(c *fiber.Ctx) error {
	var req dto.RunCustomCodeDTO
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}

	result, err := h.svc.RunCustomCode(c.Context(), req)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(result)
}

// ExecuteCode - Free sandbox execution without assignment
func (h *SubmissionHandler) ExecuteCode(c *fiber.Ctx) error {
	var req dto.ExecuteCodeDTO
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}

	result, err := h.svc.ExecuteCode(c.Context(), req)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(result)
}

func (h *SubmissionHandler) GetMySubmissions(c *fiber.Ctx) error {
	assignmentID, err := uuid.Parse(c.Params("assignmentId"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid assignment_id"})
	}

	userID, err := uuid.Parse(c.Query("user_id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "user_id is required"})
	}

	submissions, err := h.svc.GetUserSubmissionsForAssignment(c.Context(), assignmentID, userID)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(fiber.Map{"data": submissions})
}
