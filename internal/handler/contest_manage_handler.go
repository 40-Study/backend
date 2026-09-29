package handler

import (
	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"study.com/v1/internal/dto"
	"study.com/v1/internal/model"
)

// Route của người tạo cuộc thi (quyền CONTESTS_MANAGE_OWN gắn ở router) — contract #3–#6, #8–#11.

// ListManage — #3 GET /contests/manage.
func (h *ContestHandler) ListManage(c *fiber.Ctx) error {
	actor, err := h.requireActor(c)
	if err != nil {
		return respondContestError(c, err)
	}
	q, bad := listQuery(c, model.ContestPhases)
	if bad != "" {
		return contestInvalidQuery(c, bad)
	}
	res, err := h.svc.ListManage(c.Context(), actor, q)
	if err != nil {
		return respondContestError(c, err)
	}
	return contestOK(c, "Success", res)
}

// QuizOptions — #4 GET /contests/manage/quiz-options.
func (h *ContestHandler) QuizOptions(c *fiber.Ctx) error {
	actor, err := h.requireActor(c)
	if err != nil {
		return respondContestError(c, err)
	}
	res, err := h.svc.QuizOptions(c.Context(), actor)
	if err != nil {
		return respondContestError(c, err)
	}
	return contestOK(c, "Success", res)
}

// GetManage — #5 GET /contests/manage/:id.
func (h *ContestHandler) GetManage(c *fiber.Ctx) error {
	return withContestID(c, func(id uuid.UUID) error {
		actor, err := h.requireActor(c)
		if err != nil {
			return respondContestError(c, err)
		}
		res, err := h.svc.GetManage(c.Context(), id, actor)
		if err != nil {
			return respondContestError(c, err)
		}
		return contestOK(c, "Success", res)
	})
}

// ListParticipants — #6 GET /contests/manage/:id/participants.
func (h *ContestHandler) ListParticipants(c *fiber.Ctx) error {
	return withContestID(c, func(id uuid.UUID) error {
		actor, err := h.requireActor(c)
		if err != nil {
			return respondContestError(c, err)
		}
		res, err := h.svc.Participants(c.Context(), id, actor, c.QueryInt("page", 1), c.QueryInt("limit", 12))
		if err != nil {
			return respondContestError(c, err)
		}
		return contestOK(c, "Success", res)
	})
}

// CreateContest — #8 POST /contests.
func (h *ContestHandler) CreateContest(c *fiber.Ctx) error {
	actor, err := h.requireActor(c)
	if err != nil {
		return respondContestError(c, err)
	}
	var req dto.ContestUpsertRequest
	if okBody, resp := parseContestBody(c, &req); !okBody {
		return resp
	}
	res, err := h.svc.Create(c.Context(), actor, &req)
	if err != nil {
		return respondContestError(c, err)
	}
	return c.Status(fiber.StatusCreated).JSON(fiber.Map{"message": "Contest created", "data": res})
}

// UpdateContest — #9 PUT /contests/:id.
func (h *ContestHandler) UpdateContest(c *fiber.Ctx) error {
	return withContestID(c, func(id uuid.UUID) error {
		actor, err := h.requireActor(c)
		if err != nil {
			return respondContestError(c, err)
		}
		var req dto.ContestUpsertRequest
		if okBody, resp := parseContestBody(c, &req); !okBody {
			return resp
		}
		res, err := h.svc.Update(c.Context(), id, actor, &req)
		if err != nil {
			return respondContestError(c, err)
		}
		return contestOK(c, "Contest updated", res)
	})
}

// DeleteContest — #10 DELETE /contests/:id.
func (h *ContestHandler) DeleteContest(c *fiber.Ctx) error {
	return withContestID(c, func(id uuid.UUID) error {
		actor, err := h.requireActor(c)
		if err != nil {
			return respondContestError(c, err)
		}
		if err := h.svc.Delete(c.Context(), id, actor); err != nil {
			return respondContestError(c, err)
		}
		return contestOK(c, "Contest deleted", nil)
	})
}

// SubmitReview — #11 POST /contests/:id/submit-review.
func (h *ContestHandler) SubmitReview(c *fiber.Ctx) error {
	return withContestID(c, func(id uuid.UUID) error {
		actor, err := h.requireActor(c)
		if err != nil {
			return respondContestError(c, err)
		}
		res, err := h.svc.SubmitReview(c.Context(), id, actor)
		if err != nil {
			return respondContestError(c, err)
		}
		return contestOK(c, "Contest submitted for review", res)
	})
}
