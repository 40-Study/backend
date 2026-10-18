package handler

import (
	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"study.com/v1/internal/dto"
)

func (h *ContestHandler) CodeSession(c *fiber.Ctx) error { return h.codeSessionAction(c, "read") }
func (h *ContestHandler) StartCode(c *fiber.Ctx) error   { return h.codeSessionAction(c, "start") }
func (h *ContestHandler) FinishCode(c *fiber.Ctx) error  { return h.codeSessionAction(c, "finish") }
func (h *ContestHandler) codeSessionAction(c *fiber.Ctx, action string) error {
	a, err := h.requireActor(c)
	if err != nil {
		return respondContestError(c, err)
	}
	return withContestID(c, func(id uuid.UUID) error {
		var out *dto.ContestCodeSession
		switch action {
		case "start":
			out, err = h.svc.StartCode(c.Context(), id, a)
		case "finish":
			out, err = h.svc.FinishCode(c.Context(), id, a)
		default:
			out, err = h.svc.CodeSession(c.Context(), id, a)
		}
		if err != nil {
			return respondContestError(c, err)
		}
		c.Set("Cache-Control", "no-store")
		return contestOK(c, "OK", out)
	})
}
func (h *ContestHandler) RunContestCode(c *fiber.Ctx) error    { return h.judgeContestCode(c, false) }
func (h *ContestHandler) SubmitContestCode(c *fiber.Ctx) error { return h.judgeContestCode(c, true) }
func (h *ContestHandler) judgeContestCode(c *fiber.Ctx, submit bool) error {
	a, err := h.requireActor(c)
	if err != nil {
		return respondContestError(c, err)
	}
	if len(c.Body()) > 100000 {
		return c.Status(413).JSON(fiber.Map{"message": "Mã nguồn quá dài"})
	}
	var req dto.ContestCodeRequest
	if err := c.BodyParser(&req); err != nil {
		return contestInvalidQuery(c, "body")
	}
	return withContestID(c, func(id uuid.UUID) error {
		out, err := h.svc.JudgeCode(c.Context(), id, a, req, submit)
		if err != nil {
			return respondContestError(c, err)
		}
		return contestOK(c, "Đã chạy bằng Judge0", out)
	})
}
