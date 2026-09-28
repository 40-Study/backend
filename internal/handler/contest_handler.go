package handler

import (
	"context"
	"errors"
	"log"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"study.com/v1/internal/model"
	"study.com/v1/internal/service"
	"study.com/v1/internal/utils"
)

// ContestHandler — MVP "Cuộc thi" (contract §2.2). Handler chỉ parse/validate request, dựng
// ContestActor và ánh xạ lỗi service sang (HTTP, code) theo §2.4; mọi luật nghiệp vụ ở service.
// Tách file: contest_manage_handler.go (giảng viên), contest_play_handler.go (thí sinh),
// contest_admin_handler.go (admin).
type ContestHandler struct {
	svc   *service.ContestService
	perms ContestPermissionChecker
}

// ContestPermissionChecker — phần PermissionChecker cần để tính is_admin (contract §0).
type ContestPermissionChecker interface {
	HasPermission(ctx context.Context, userID uuid.UUID, activeOrgID *uuid.UUID, permission string) (bool, error)
}

func NewContestHandler(svc *service.ContestService, perms ContestPermissionChecker) *ContestHandler {
	return &ContestHandler{svc: svc, perms: perms}
}

const contestApprovePermission = "CONTESTS_APPROVE_ALL"

// actor dựng người gọi từ Locals của AuthMiddleware; (nil, nil) = khách (OptionalAuth không có token).
func (h *ContestHandler) actor(c *fiber.Ctx) (*service.ContestActor, error) {
	userID, ok := c.Locals("user_id").(uuid.UUID)
	if !ok || userID == uuid.Nil {
		return nil, nil
	}
	var orgID *uuid.UUID
	if id, ok := c.Locals("active_org_id").(uuid.UUID); ok {
		orgID = &id
	}
	isAdmin, err := h.perms.HasPermission(c.Context(), userID, orgID, contestApprovePermission)
	if err != nil {
		return nil, err
	}
	role, _ := c.Locals("active_role").(string)
	return &service.ContestActor{UserID: userID, ActiveRole: role, IsAdmin: isAdmin}, nil
}

// requireActor — route có AuthMiddleware nên luôn có user; thiếu = lỗi cấu hình route → 401.
func (h *ContestHandler) requireActor(c *fiber.Ctx) (*service.ContestActor, error) {
	a, err := h.actor(c)
	if err == nil && a == nil {
		return nil, errUnauthenticated
	}
	return a, err
}

var errUnauthenticated = errors.New("unauthenticated")

// respondContestError ánh xạ lỗi theo contract §2.4; lỗi không xác định KHÔNG lộ err.Error().
func respondContestError(c *fiber.Ctx, err error) error {
	var ce *service.ContestError
	var ve *service.ContestValidationError
	switch {
	case errors.As(err, &ce):
		body := fiber.Map{"message": ce.Message, "code": ce.Code}
		if ce.Details != nil {
			body["details"] = ce.Details
		}
		return c.Status(ce.Status).JSON(body)
	case errors.As(err, &ve):
		return contestValidationFailed(c, ve.Errors)
	case errors.Is(err, errUnauthenticated):
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"message": "Unauthorized"})
	}
	log.Printf("[ERROR] %s %s: %v", c.Method(), c.Path(), err)
	return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"message": "Đã có lỗi xảy ra", "error": "INTERNAL_ERROR"})
}

func contestValidationFailed(c *fiber.Ctx, errs []utils.ValidationError) error {
	return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"message": "Validation failed", "errors": errs})
}

func contestInvalidQuery(c *fiber.Ctx, field string) error {
	return contestValidationFailed(c, []utils.ValidationError{{Field: field, Tag: "oneof", Message: field + " is invalid"}})
}

// withContestID parse :id rồi gọi fn; id sai → 400 INVALID_ID.
func withContestID(c *fiber.Ctx, fn func(id uuid.UUID) error) error {
	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"message": "Invalid contest ID", "code": "INVALID_ID"})
	}
	return fn(id)
}

// parseBody + validate tag; trả (false, response) khi lỗi.
func parseContestBody(c *fiber.Ctx, out interface{}) (bool, error) {
	if err := c.BodyParser(out); err != nil {
		return false, contestValidationFailed(c, []utils.ValidationError{{Field: "body", Tag: "json", Message: "Invalid request body"}})
	}
	if errs := utils.ValidateStruct(out); len(errs) > 0 {
		return false, contestValidationFailed(c, errs)
	}
	return true, nil
}

func contestOK(c *fiber.Ctx, message string, data interface{}) error {
	return c.JSON(fiber.Map{"message": message, "data": data})
}

// listQuery đọc status/phase/q/course_id/page/limit và kiểm status/phase thuộc tập cho phép.
func listQuery(c *fiber.Ctx, phases []string) (service.ContestListQuery, string) {
	q := service.ContestListQuery{Status: c.Query("status"), Phase: c.Query("phase"), Q: c.Query("q"),
		Page: c.QueryInt("page", 1), Limit: c.QueryInt("limit", 12)}
	if q.Status != "" && !contestInList(model.ContestStatuses, q.Status) {
		return q, "status"
	}
	if q.Phase != "" && !contestInList(phases, q.Phase) {
		return q, "phase"
	}
	if raw := c.Query("course_id"); raw != "" {
		id, err := uuid.Parse(raw)
		if err != nil {
			return q, "course_id"
		}
		q.CourseID = &id
	}
	return q, ""
}

func contestInList(list []string, v string) bool {
	for _, s := range list {
		if s == v {
			return true
		}
	}
	return false
}

var publicListPhases = []string{model.ContestPhaseUpcoming, model.ContestPhaseActive, model.ContestPhaseEnded, model.ContestPhaseFinalized}

// ListContests — #1 GET /contests (công khai).
func (h *ContestHandler) ListContests(c *fiber.Ctx) error {
	q, bad := listQuery(c, publicListPhases)
	if bad != "" || q.Status != "" {
		if bad == "" {
			bad = "status"
		}
		return contestInvalidQuery(c, bad)
	}
	res, err := h.svc.ListPublic(c.Context(), q)
	if err != nil {
		return respondContestError(c, err)
	}
	return contestOK(c, "Success", res)
}

// GetContest — #7 GET /contests/:slug (OptionalAuth).
func (h *ContestHandler) GetContest(c *fiber.Ctx) error {
	actor, err := h.actor(c)
	if err != nil {
		return respondContestError(c, err)
	}
	res, err := h.svc.Detail(c.Context(), c.Params("slug"), actor)
	if err != nil {
		return respondContestError(c, err)
	}
	return contestOK(c, "Success", res)
}
