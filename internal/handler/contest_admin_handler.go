package handler

import (
	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"study.com/v1/internal/dto"
	"study.com/v1/internal/model"
	"study.com/v1/internal/service"
)

// Route admin (quyền CONTESTS_APPROVE_ALL gắn ở router) — contract #19–#23 và chốt kết quả
// (ĐÍNH CHÍNH 28/09: POST /admin/contests/:id/finalize, chỉ admin).

type serviceActor = service.ContestActor

// AdminListContests — #19 GET /admin/contests.
func (h *ContestHandler) AdminListContests(c *fiber.Ctx) error {
	actor, err := h.requireActor(c)
	if err != nil {
		return respondContestError(c, err)
	}
	q, bad := listQuery(c, model.ContestPhases)
	if bad != "" {
		return contestInvalidQuery(c, bad)
	}
	res, err := h.svc.ListAdmin(c.Context(), actor, q)
	if err != nil {
		return respondContestError(c, err)
	}
	return contestOK(c, "Success", res)
}

// parseOptionalPrizes: body rỗng → nil (giữ giải hiện có); có body → validate như PrizesRequest.
func parseOptionalPrizes(c *fiber.Ctx) ([]dto.ContestPrizeInput, bool, error) {
	if len(c.Body()) == 0 {
		return nil, true, nil
	}
	var req dto.ContestPrizesRequest
	if okBody, resp := parseContestBody(c, &req); !okBody {
		return nil, false, resp
	}
	if req.Prizes == nil { // body {} không có "prizes" → giữ nguyên giải
		return nil, true, nil
	}
	return req.Prizes, true, nil
}

// ApproveContest — #20 POST /admin/contests/:id/approve (body PrizesRequest tuỳ chọn).
func (h *ContestHandler) ApproveContest(c *fiber.Ctx) error {
	return h.withActorID(c, func(id uuid.UUID, actor *serviceActor) error {
		prizes, okBody, resp := parseOptionalPrizes(c)
		if !okBody {
			return resp
		}
		res, err := h.svc.Approve(c.Context(), id, actor, prizes)
		if err != nil {
			return respondContestError(c, err)
		}
		return contestOK(c, "Contest approved", res)
	})
}

func parseReason(c *fiber.Ctx) (string, bool, error) {
	var req dto.ContestReasonRequest
	if err := c.BodyParser(&req); err != nil && len(c.Body()) > 0 {
		return "", false, contestInvalidQuery(c, "body")
	}
	return req.Reason, true, nil
}

// RejectContest — #21 POST /admin/contests/:id/reject.
func (h *ContestHandler) RejectContest(c *fiber.Ctx) error {
	return h.withActorID(c, func(id uuid.UUID, actor *serviceActor) error {
		reason, okBody, resp := parseReason(c)
		if !okBody {
			return resp
		}
		res, err := h.svc.Reject(c.Context(), id, actor, reason)
		if err != nil {
			return respondContestError(c, err)
		}
		return contestOK(c, "Contest rejected", res)
	})
}

// CancelContest — #22 POST /admin/contests/:id/cancel.
func (h *ContestHandler) CancelContest(c *fiber.Ctx) error {
	return h.withActorID(c, func(id uuid.UUID, actor *serviceActor) error {
		reason, okBody, resp := parseReason(c)
		if !okBody {
			return resp
		}
		res, err := h.svc.Cancel(c.Context(), id, actor, reason)
		if err != nil {
			return respondContestError(c, err)
		}
		return contestOK(c, "Contest cancelled", res)
	})
}

// UpdatePrizes — #23 PUT /admin/contests/:id/prizes.
func (h *ContestHandler) UpdatePrizes(c *fiber.Ctx) error {
	return h.withActorID(c, func(id uuid.UUID, actor *serviceActor) error {
		var req dto.ContestPrizesRequest
		if okBody, resp := parseContestBody(c, &req); !okBody {
			return resp
		}
		res, err := h.svc.UpdatePrizes(c.Context(), id, actor, req.Prizes)
		if err != nil {
			return respondContestError(c, err)
		}
		return contestOK(c, "Prizes updated", res)
	})
}

// FinalizeContest — POST /admin/contests/:id/finalize.
func (h *ContestHandler) FinalizeContest(c *fiber.Ctx) error {
	return h.withActorID(c, func(id uuid.UUID, actor *serviceActor) error {
		res, err := h.svc.Finalize(c.Context(), id, actor)
		if err != nil {
			return respondContestError(c, err)
		}
		return contestOK(c, "Contest finalized", res)
	})
}
