package handler

import (
	"errors"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"study.com/v1/internal/dto"
	"study.com/v1/internal/middleware"
	"study.com/v1/internal/service"
	"study.com/v1/internal/utils"
)

// ReportModerationHandler - ADM-N4, contract: plans/261013-qa2-role-findings/contract-be3.md §1b.
type ReportModerationHandler struct {
	service service.ReportModerationServiceInterface
}

func NewReportModerationHandler(service service.ReportModerationServiceInterface) *ReportModerationHandler {
	return &ReportModerationHandler{service: service}
}

func reportActionError(c *fiber.Ctx, status int, code, message string) error {
	return c.Status(status).JSON(fiber.Map{"message": message, "code": code})
}

// ApplyAction POST /reports/:id/actions (REPORTS_MODERATE, audit report.moderate).
func (h *ReportModerationHandler) ApplyAction(c *fiber.Ctx) error {
	reportID, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return reportActionError(c, fiber.StatusBadRequest, "VALIDATION_FAILED", "Invalid report ID")
	}
	adminID, ok := c.Locals("user_id").(uuid.UUID)
	if !ok || adminID == uuid.Nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"message": "Unauthorized"})
	}
	var req dto.ReportActionRequestDTO
	if err := c.BodyParser(&req); err != nil {
		return reportActionError(c, fiber.StatusBadRequest, "VALIDATION_FAILED", "Invalid request body")
	}
	if errs := utils.ValidateStruct(req); len(errs) > 0 {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"message": "Validation failed", "code": "VALIDATION_FAILED", "errors": errs})
	}

	result, err := h.service.Apply(c.Context(), reportID, adminID, req)
	switch {
	case err == nil:
	case errors.Is(err, service.ErrReportNotFound):
		return reportActionError(c, fiber.StatusNotFound, "REPORT_NOT_FOUND", "Report not found")
	case errors.Is(err, service.ErrReportTargetNotFound):
		return reportActionError(c, fiber.StatusNotFound, "TARGET_NOT_FOUND", "Reported object no longer exists")
	case errors.Is(err, service.ErrReportActionNotApplicable):
		return reportActionError(c, fiber.StatusBadRequest, "ACTION_NOT_APPLICABLE", "This action does not apply to the reported object type")
	case errors.Is(err, service.ErrReportActionReasonRequired):
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Validation failed", "code": "VALIDATION_FAILED",
			"errors": []utils.ValidationError{{Field: "reason", Tag: "required", Message: "reason is required (at least 3 characters) for this action"}},
		})
	case errors.Is(err, service.ErrReportTargetUnchanged):
		return reportActionError(c, fiber.StatusConflict, "TARGET_STATE_UNCHANGED", "The reported object is not in a state this action can change")
	default:
		return RespondServiceError(c, err, "Failed to apply report action")
	}

	middleware.SetAuditMeta(c, map[string]any{
		"action":      result.Action,
		"target_type": result.Report.ReportedType,
		"target_id":   result.Report.ReportedID.String(),
	})
	return c.Status(fiber.StatusOK).JSON(fiber.Map{"message": "Report action applied", "data": result})
}
