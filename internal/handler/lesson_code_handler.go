package handler

import (
	"context"
	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"study.com/v1/internal/dto"
	"study.com/v1/internal/service"
)

type lessonCodeService interface {
	LessonCode(context.Context, uuid.UUID, uuid.UUID, uuid.UUID, bool) (*service.LessonCodeDetail, error)
	JudgeLessonCode(context.Context, uuid.UUID, uuid.UUID, uuid.UUID, bool, dto.ContestCodeRequest, bool) (*dto.ContestCodeResult, error)
}

func (h *ExerciseHandler) LessonCode(c *fiber.Ctx) error    { return h.lessonCodeAction(c, "read") }
func (h *ExerciseHandler) RunLessonCode(c *fiber.Ctx) error { return h.lessonCodeAction(c, "run") }
func (h *ExerciseHandler) SubmitLessonCode(c *fiber.Ctx) error {
	return h.lessonCodeAction(c, "submit")
}
func (h *ExerciseHandler) lessonCodeAction(c *fiber.Ctx, action string) error {
	userID, ok := c.Locals("user_id").(uuid.UUID)
	if !ok {
		return c.SendStatus(401)
	}
	lessonID, err := uuid.Parse(c.Params("lessonId"))
	if err != nil {
		return c.SendStatus(400)
	}
	exerciseID, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.SendStatus(400)
	}
	svc, ok := h.service.(lessonCodeService)
	if !ok {
		return c.SendStatus(503)
	}
	admin := isAdminActor(c, h.permChecker, userID)
	c.Set("Cache-Control", "no-store")
	if action == "read" {
		out, err := svc.LessonCode(c.Context(), lessonID, exerciseID, userID, admin)
		if err != nil {
			return respondContestError(c, err)
		}
		return contestOK(c, "OK", out)
	}
	if action == "submit" {
		role, _ := c.Locals("active_role").(string)
		if role != "STUDENT" {
			return c.Status(403).JSON(fiber.Map{"message": "Chỉ học sinh được nộp bài."})
		}
	}
	if len(c.Body()) > 100000 {
		return c.SendStatus(413)
	}
	var req dto.ContestCodeRequest
	if c.BodyParser(&req) != nil {
		return c.SendStatus(400)
	}
	out, err := svc.JudgeLessonCode(c.Context(), lessonID, exerciseID, userID, admin, req, action == "submit")
	if err != nil {
		return respondContestError(c, err)
	}
	return contestOK(c, "Đã chạy bằng Judge0", out)
}
