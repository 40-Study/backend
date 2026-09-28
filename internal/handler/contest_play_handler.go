package handler

import (
	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"study.com/v1/internal/dto"
)

// Route của thí sinh — contract #2, #12–#17.

// GetMyContests — #2 GET /contests/me.
func (h *ContestHandler) GetMyContests(c *fiber.Ctx) error {
	actor, err := h.requireActor(c)
	if err != nil {
		return respondContestError(c, err)
	}
	res, err := h.svc.ListMine(c.Context(), actor, c.QueryInt("page", 1), c.QueryInt("limit", 12))
	if err != nil {
		return respondContestError(c, err)
	}
	return contestOK(c, "Success", res)
}

// withActorID gộp parse :id + dựng actor bắt buộc cho các route thí sinh.
func (h *ContestHandler) withActorID(c *fiber.Ctx, fn func(id uuid.UUID, actor *serviceActor) error) error {
	return withContestID(c, func(id uuid.UUID) error {
		actor, err := h.requireActor(c)
		if err != nil {
			return respondContestError(c, err)
		}
		return fn(id, actor)
	})
}

// JoinContest — #12 POST /contests/:id/join.
func (h *ContestHandler) JoinContest(c *fiber.Ctx) error {
	return h.withActorID(c, func(id uuid.UUID, actor *serviceActor) error {
		res, err := h.svc.Join(c.Context(), id, actor)
		if err != nil {
			return respondContestError(c, err)
		}
		return c.Status(fiber.StatusCreated).JSON(fiber.Map{"message": "Joined contest", "data": res})
	})
}

// StartContest — #13 POST /contests/:id/start.
func (h *ContestHandler) StartContest(c *fiber.Ctx) error {
	return h.withActorID(c, func(id uuid.UUID, actor *serviceActor) error {
		res, err := h.svc.Start(c.Context(), id, actor)
		if err != nil {
			return respondContestError(c, err)
		}
		return contestOK(c, "Contest started", res)
	})
}

// SubmitContest — #14 POST /contests/:id/submit.
func (h *ContestHandler) SubmitContest(c *fiber.Ctx) error {
	return h.withActorID(c, func(id uuid.UUID, actor *serviceActor) error {
		var req dto.ContestSubmitRequest
		if okBody, resp := parseContestBody(c, &req); !okBody {
			return resp
		}
		res, err := h.svc.Submit(c.Context(), id, actor, &req)
		if err != nil {
			return respondContestError(c, err)
		}
		return contestOK(c, "Contest submitted", res)
	})
}

// MyResult — #15 GET /contests/:id/my-result.
func (h *ContestHandler) MyResult(c *fiber.Ctx) error {
	return h.withActorID(c, func(id uuid.UUID, actor *serviceActor) error {
		res, err := h.svc.MyResult(c.Context(), id, actor)
		if err != nil {
			return respondContestError(c, err)
		}
		return contestOK(c, "Success", res)
	})
}

// Leaderboard — #16 GET /contests/:id/leaderboard (OptionalAuth).
func (h *ContestHandler) Leaderboard(c *fiber.Ctx) error {
	return withContestID(c, func(id uuid.UUID) error {
		actor, err := h.actor(c)
		if err != nil {
			return respondContestError(c, err)
		}
		res, err := h.svc.Leaderboard(c.Context(), id, actor, c.QueryInt("page", 1), c.QueryInt("limit", 12))
		if err != nil {
			return respondContestError(c, err)
		}
		return contestOK(c, "Success", res)
	})
}

// Certificate — #17 GET /contests/:id/certificate.
func (h *ContestHandler) Certificate(c *fiber.Ctx) error {
	return h.withActorID(c, func(id uuid.UUID, actor *serviceActor) error {
		res, err := h.svc.Certificate(c.Context(), id, actor)
		if err != nil {
			return respondContestError(c, err)
		}
		return contestOK(c, "Success", res)
	})
}
