package handler

import (
	"strconv"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"study.com/v1/internal/dto"
	"study.com/v1/internal/service"
	"study.com/v1/internal/utils"
)

type ContestHandler struct {
	contestService service.ContestServiceInterface
}

func NewContestHandler(contestService service.ContestServiceInterface) *ContestHandler {
	return &ContestHandler{contestService: contestService}
}

// ─── Public endpoints ───────────────────────────────────────────────────────

// ListContests - GET /api/contests
func (h *ContestHandler) ListContests(c *fiber.Ctx) error {
	page, _ := strconv.Atoi(c.Query("page", "1"))
	limit, _ := strconv.Atoi(c.Query("limit", "20"))
	keyword := c.Query("keyword")
	status := c.Query("status")

	result, err := h.contestService.ListContests(c.Context(), keyword, status, page, limit)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"message": err.Error(),
		})
	}

	return c.JSON(fiber.Map{
		"message": "Contests retrieved successfully",
		"data":    result,
	})
}

// GetContest - GET /api/contests/:slug
func (h *ContestHandler) GetContest(c *fiber.Ctx) error {
	slug := c.Params("slug")

	var userID *uuid.UUID
	if uid := c.Locals("user_id"); uid != nil {
		parsed, ok := uid.(uuid.UUID)
		if ok {
			userID = &parsed
		}
	}

	result, err := h.contestService.GetContestBySlug(c.Context(), slug, userID)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"message": err.Error(),
		})
	}

	return c.JSON(fiber.Map{
		"message": "Contest retrieved successfully",
		"data":    result,
	})
}

// ─── Contest CRUD ───────────────────────────────────────────────────────────

// CreateContest - POST /api/contests
func (h *ContestHandler) CreateContest(c *fiber.Ctx) error {
	userID, err := parseUserID(c)
	if err != nil {
		return err
	}

	var req dto.CreateContestRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Invalid request body",
		})
	}

	if errs := utils.ValidateStruct(req); len(errs) > 0 {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Validation failed",
			"errors":  errs,
		})
	}

	result, err := h.contestService.CreateContest(c.Context(), userID, req)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": err.Error(),
		})
	}

	return c.Status(fiber.StatusCreated).JSON(fiber.Map{
		"message": "Contest created successfully",
		"data":    result,
	})
}

// UpdateContest - PUT /api/contests/:id
func (h *ContestHandler) UpdateContest(c *fiber.Ctx) error {
	userID, err := parseUserID(c)
	if err != nil {
		return err
	}

	contestID, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Invalid contest ID",
		})
	}

	var req dto.UpdateContestRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Invalid request body",
		})
	}

	result, err := h.contestService.UpdateContest(c.Context(), userID, contestID, req)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": err.Error(),
		})
	}

	return c.JSON(fiber.Map{
		"message": "Contest updated successfully",
		"data":    result,
	})
}

// DeleteContest - DELETE /api/contests/:id
func (h *ContestHandler) DeleteContest(c *fiber.Ctx) error {
	userID, err := parseUserID(c)
	if err != nil {
		return err
	}

	contestID, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Invalid contest ID",
		})
	}

	if err := h.contestService.DeleteContest(c.Context(), userID, contestID); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": err.Error(),
		})
	}

	return c.JSON(fiber.Map{
		"message": "Contest deleted successfully",
	})
}

// PublishContest - POST /api/contests/:id/publish
func (h *ContestHandler) PublishContest(c *fiber.Ctx) error {
	userID, err := parseUserID(c)
	if err != nil {
		return err
	}

	contestID, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Invalid contest ID",
		})
	}

	result, err := h.contestService.PublishContest(c.Context(), userID, contestID)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": err.Error(),
		})
	}

	return c.JSON(fiber.Map{
		"message": "Contest published successfully",
		"data":    result,
	})
}

// ─── My contests ────────────────────────────────────────────────────────────

// GetMyContests - GET /api/contests/me
func (h *ContestHandler) GetMyContests(c *fiber.Ctx) error {
	userID, err := parseUserID(c)
	if err != nil {
		return err
	}

	result, err := h.contestService.GetMyContests(c.Context(), userID)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"message": err.Error(),
		})
	}

	return c.JSON(fiber.Map{
		"message": "My contests retrieved successfully",
		"data":    result,
	})
}

// ─── Problems ───────────────────────────────────────────────────────────────

// CreateProblem - POST /api/contests/:id/problems
func (h *ContestHandler) CreateProblem(c *fiber.Ctx) error {
	userID, err := parseUserID(c)
	if err != nil {
		return err
	}

	contestID, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Invalid contest ID",
		})
	}

	var req dto.CreateProblemRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Invalid request body",
		})
	}

	if errs := utils.ValidateStruct(req); len(errs) > 0 {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Validation failed",
			"errors":  errs,
		})
	}

	result, err := h.contestService.CreateProblem(c.Context(), userID, contestID, req)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": err.Error(),
		})
	}

	return c.Status(fiber.StatusCreated).JSON(fiber.Map{
		"message": "Problem created successfully",
		"data":    result,
	})
}

// UpdateProblem - PUT /api/contests/:id/problems/:problemId
func (h *ContestHandler) UpdateProblem(c *fiber.Ctx) error {
	userID, err := parseUserID(c)
	if err != nil {
		return err
	}

	contestID, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Invalid contest ID",
		})
	}

	problemID, err := uuid.Parse(c.Params("problemId"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Invalid problem ID",
		})
	}

	var req dto.UpdateProblemRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Invalid request body",
		})
	}

	result, err := h.contestService.UpdateProblem(c.Context(), userID, contestID, problemID, req)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": err.Error(),
		})
	}

	return c.JSON(fiber.Map{
		"message": "Problem updated successfully",
		"data":    result,
	})
}

// DeleteProblem - DELETE /api/contests/:id/problems/:problemId
func (h *ContestHandler) DeleteProblem(c *fiber.Ctx) error {
	userID, err := parseUserID(c)
	if err != nil {
		return err
	}

	contestID, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Invalid contest ID",
		})
	}

	problemID, err := uuid.Parse(c.Params("problemId"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Invalid problem ID",
		})
	}

	if err := h.contestService.DeleteProblem(c.Context(), userID, contestID, problemID); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": err.Error(),
		})
	}

	return c.JSON(fiber.Map{
		"message": "Problem deleted successfully",
	})
}

// GetProblems - GET /api/contests/:id/problems
func (h *ContestHandler) GetProblems(c *fiber.Ctx) error {
	contestID, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Invalid contest ID",
		})
	}

	result, err := h.contestService.GetProblems(c.Context(), contestID)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"message": err.Error(),
		})
	}

	return c.JSON(fiber.Map{
		"message": "Problems retrieved successfully",
		"data":    result,
	})
}

// ─── Participation ──────────────────────────────────────────────────────────

// JoinContest - POST /api/contests/:id/join
func (h *ContestHandler) JoinContest(c *fiber.Ctx) error {
	userID, err := parseUserID(c)
	if err != nil {
		return err
	}

	contestID, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Invalid contest ID",
		})
	}

	result, err := h.contestService.JoinContest(c.Context(), userID, contestID)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": err.Error(),
		})
	}

	return c.JSON(fiber.Map{
		"message": "Joined contest successfully",
		"data":    result,
	})
}

// GetLeaderboard - GET /api/contests/:id/leaderboard
func (h *ContestHandler) GetLeaderboard(c *fiber.Ctx) error {
	contestID, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Invalid contest ID",
		})
	}

	page, _ := strconv.Atoi(c.Query("page", "1"))
	limit, _ := strconv.Atoi(c.Query("limit", "50"))

	result, err := h.contestService.GetLeaderboard(c.Context(), contestID, page, limit)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"message": err.Error(),
		})
	}

	return c.JSON(fiber.Map{
		"message": "Leaderboard retrieved successfully",
		"data":    result,
	})
}

// ─── Submissions ────────────────────────────────────────────────────────────

// SubmitAnswer - POST /api/contests/:id/problems/:problemId/submit
func (h *ContestHandler) SubmitAnswer(c *fiber.Ctx) error {
	userID, err := parseUserID(c)
	if err != nil {
		return err
	}

	contestID, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Invalid contest ID",
		})
	}

	problemID, err := uuid.Parse(c.Params("problemId"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Invalid problem ID",
		})
	}

	var req dto.SubmitAnswerRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Invalid request body",
		})
	}

	result, err := h.contestService.SubmitAnswer(c.Context(), userID, contestID, problemID, req)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": err.Error(),
		})
	}

	return c.Status(fiber.StatusCreated).JSON(fiber.Map{
		"message": "Answer submitted successfully",
		"data":    result,
	})
}

// GetMySubmissions - GET /api/contests/:id/submissions/me
func (h *ContestHandler) GetMySubmissions(c *fiber.Ctx) error {
	userID, err := parseUserID(c)
	if err != nil {
		return err
	}

	contestID, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Invalid contest ID",
		})
	}

	result, err := h.contestService.GetMySubmissions(c.Context(), userID, contestID)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"message": err.Error(),
		})
	}

	return c.JSON(fiber.Map{
		"message": "Submissions retrieved successfully",
		"data":    result,
	})
}
