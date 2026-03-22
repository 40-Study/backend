package handler

import (
	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"study.com/v1/internal/dto"
	"study.com/v1/internal/service"
)

type AssignmentHandlerInterface interface {
	Create(c *fiber.Ctx) error
	GetByID(c *fiber.Ctx) error
	GetBySession(c *fiber.Ctx) error
	Update(c *fiber.Ctx) error
	Delete(c *fiber.Ctx) error
	Publish(c *fiber.Ctx) error
	Unpublish(c *fiber.Ctx) error
	AddTestCase(c *fiber.Ctx) error
	DeleteTestCase(c *fiber.Ctx) error
	ImportTestCases(c *fiber.Ctx) error
	GetTestCases(c *fiber.Ctx) error
	GetSandbox(c *fiber.Ctx) error
}

type AssignmentHandler struct {
	svc        service.AssignmentServiceInterface
	livekitSvc service.LivekitServiceInterface
}

func NewAssignmentHandler(svc service.AssignmentServiceInterface, livekitSvc service.LivekitServiceInterface) *AssignmentHandler {
	return &AssignmentHandler{svc: svc, livekitSvc: livekitSvc}
}

func (h *AssignmentHandler) Create(c *fiber.Ctx) error {
	var req dto.CreateAssignmentDTO
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}

	assignment, err := h.svc.Create(c.Context(), req)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}

	return c.Status(fiber.StatusCreated).JSON(fiber.Map{
		"message": "Assignment created",
		"data":    assignment,
	})
}

func (h *AssignmentHandler) GetByID(c *fiber.Ctx) error {
	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid id"})
	}

	includeHidden := c.QueryBool("include_hidden", false)

	assignment, err := h.svc.GetByID(c.Context(), id, includeHidden)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
	if assignment == nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "assignment not found"})
	}

	return c.JSON(fiber.Map{"data": assignment})
}

func (h *AssignmentHandler) GetBySession(c *fiber.Ctx) error {
	sessionIDStr := c.Query("session_id")
	if sessionIDStr == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "session_id is required"})
	}
	sessionID, err := uuid.Parse(sessionIDStr)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid session_id"})
	}

	page := c.QueryInt("page", 1)
	pageSize := c.QueryInt("page_size", 20)

	result, err := h.svc.GetBySession(c.Context(), sessionID, page, pageSize)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(result)
}

func (h *AssignmentHandler) Update(c *fiber.Ctx) error {
	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid id"})
	}

	var req dto.UpdateAssignmentDTO
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}

	assignment, err := h.svc.Update(c.Context(), id, req)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(fiber.Map{"message": "Assignment updated", "data": assignment})
}

func (h *AssignmentHandler) Delete(c *fiber.Ctx) error {
	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid id"})
	}

	if err := h.svc.Delete(c.Context(), id); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(fiber.Map{"message": "Assignment deleted"})
}

func (h *AssignmentHandler) Publish(c *fiber.Ctx) error {
	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid id"})
	}

	assignment, err := h.svc.Publish(c.Context(), id, h.livekitSvc)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(fiber.Map{"message": "Assignment published", "data": assignment})
}

func (h *AssignmentHandler) Unpublish(c *fiber.Ctx) error {
	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid id"})
	}

	assignment, err := h.svc.Unpublish(c.Context(), id)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(fiber.Map{"message": "Assignment unpublished", "data": assignment})
}

func (h *AssignmentHandler) AddTestCase(c *fiber.Ctx) error {
	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid id"})
	}

	var req dto.CreateTestCaseDTO
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}

	testCase, err := h.svc.AddTestCase(c.Context(), id, req)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}

	return c.Status(fiber.StatusCreated).JSON(fiber.Map{
		"message": "Test case added",
		"data":    testCase,
	})
}

func (h *AssignmentHandler) DeleteTestCase(c *fiber.Ctx) error {
	tcID, err := uuid.Parse(c.Params("tcId"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid test case id"})
	}

	if err := h.svc.DeleteTestCase(c.Context(), tcID); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(fiber.Map{"message": "Test case deleted"})
}

func (h *AssignmentHandler) ImportTestCases(c *fiber.Ctx) error {
	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid id"})
	}

	var req dto.ImportTestCasesDTO
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}

	testCases, err := h.svc.ImportTestCases(c.Context(), id, req)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}

	return c.Status(fiber.StatusCreated).JSON(fiber.Map{
		"message": "Test cases imported",
		"count":   len(testCases),
		"data":    testCases,
	})
}

func (h *AssignmentHandler) GetTestCases(c *fiber.Ctx) error {
	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid id"})
	}

	includeHidden := c.QueryBool("include_hidden", false)

	testCases, err := h.svc.GetTestCases(c.Context(), id, includeHidden)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(fiber.Map{"data": testCases})
}

func (h *AssignmentHandler) GetSandbox(c *fiber.Ctx) error {
	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid id"})
	}

	userIDVal := c.Locals("user_id")
	if userIDVal == nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "unauthorized"})
	}
	userID := userIDVal.(uuid.UUID)

	result, err := h.svc.GetSandbox(c.Context(), id, userID)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(result)
}
