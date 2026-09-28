package handler

import (
	"errors"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"study.com/v1/internal/dto"
	"study.com/v1/internal/model"
	"study.com/v1/internal/service"
)

// WithdrawalHandler — Phase 4 rút tiền giảng viên. Envelope: OK {message,data}; lỗi validate
// {message,errors}; lỗi khác {message,error,data?}.
type WithdrawalHandler struct {
	svc service.WithdrawalServiceInterface
}

func NewWithdrawalHandler(svc service.WithdrawalServiceInterface) *WithdrawalHandler {
	return &WithdrawalHandler{svc: svc}
}

// withdrawalErrorMap: lỗi nghiệp vụ -> (HTTP status, mã `error`).
var withdrawalErrorMap = []struct {
	kind    error
	status  int
	code    string
	message string
}{
	{service.ErrWithdrawalInvalidAmount, fiber.StatusBadRequest, "invalid_amount", "Amount must be greater than 0"},
	{service.ErrWithdrawalBelowMinimum, fiber.StatusBadRequest, "below_minimum", "Amount is below the minimum withdrawal"},
	{service.ErrWithdrawalBankInfoRequired, fiber.StatusBadRequest, "bank_info_required", "Bank info required before withdrawal"},
	{service.ErrWithdrawalNegativeBalance, fiber.StatusBadRequest, "negative_balance", "Balance is negative, withdrawals are blocked until new earnings cover it"},
	{service.ErrWithdrawalInsufficientBalance, fiber.StatusBadRequest, "insufficient_balance", "Amount exceeds available balance"},
	{service.ErrWithdrawalAlreadyOpen, fiber.StatusConflict, "withdrawal_already_open", "You already have a withdrawal request in progress"},
	{service.ErrWithdrawalTeacherProfileRequired, fiber.StatusForbidden, "teacher_profile_required", "Teacher profile required"},
	{service.ErrWithdrawalNotFound, fiber.StatusNotFound, "withdrawal_not_found", "Withdrawal not found"},
	{service.ErrWithdrawalInvalidTransition, fiber.StatusConflict, "invalid_status_transition", "Invalid status transition"},
	{service.ErrWithdrawalPayoutNegativeBalance, fiber.StatusConflict, "negative_balance", "Teacher balance is negative, this withdrawal cannot be approved or paid out"},
}

func writeWithdrawalError(c *fiber.Ctx, err error) error {
	if errors.Is(err, service.ErrWithdrawalReasonRequired) || errors.Is(err, service.ErrWithdrawalTransactionIDRequired) {
		return validationFailed(c, err.Error())
	}
	for _, m := range withdrawalErrorMap {
		if !errors.Is(err, m.kind) {
			continue
		}
		body := fiber.Map{"message": m.message, "error": m.code}
		var re *service.WithdrawalRuleError
		if errors.As(err, &re) && re.Data != nil {
			body["data"] = re.Data
		}
		return c.Status(m.status).JSON(body)
	}
	return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"message": "Internal server error", "error": "internal_error"})
}

func validationFailed(c *fiber.Ctx, msgs ...string) error {
	return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"message": "Validation failed", "errors": msgs})
}

// parseWithdrawalStatusFilter — ?status= rỗng = tất cả; giá trị ngoài enum => 400.
func parseWithdrawalStatusFilter(c *fiber.Ctx) (string, bool) {
	status := c.Query("status", "")
	if status != "" && !model.IsValidPayoutStatus(status) {
		return "", false
	}
	return status, true
}

// ─── Giáo viên ─────────────────────────────────────────────────────────────

// CreateWithdrawal - POST /api/wallet/teacher/withdrawals
func (h *WithdrawalHandler) CreateWithdrawal(c *fiber.Ctx) error {
	userID, err := parseUserID(c)
	if err != nil {
		return err
	}
	var req dto.CreateWithdrawalRequest
	if err := c.BodyParser(&req); err != nil {
		return validationFailed(c, "amount must be a number")
	}
	item, err := h.svc.Create(c.Context(), userID, req.Amount)
	if err != nil {
		return writeWithdrawalError(c, err)
	}
	return c.Status(fiber.StatusCreated).JSON(fiber.Map{"message": "Withdrawal request created", "data": item})
}

// ListMyWithdrawals - GET /api/wallet/teacher/withdrawals?status=&page=&limit=
func (h *WithdrawalHandler) ListMyWithdrawals(c *fiber.Ctx) error {
	userID, err := parseUserID(c)
	if err != nil {
		return err
	}
	status, ok := parseWithdrawalStatusFilter(c)
	if !ok {
		return validationFailed(c, "status must be one of pending, approved, rejected, completed, cancelled")
	}
	page, limit := parsePagination(c)
	res, err := h.svc.ListMine(c.Context(), userID, status, page, limit)
	if err != nil {
		return writeWithdrawalError(c, err)
	}
	return c.JSON(fiber.Map{"message": "OK", "data": res})
}

// CancelMyWithdrawal - POST /api/wallet/teacher/withdrawals/:id/cancel (Q2, QA vòng 2).
// Giảng viên lấy từ TOKEN; yêu cầu của người khác -> 404, không còn pending -> 409.
func (h *WithdrawalHandler) CancelMyWithdrawal(c *fiber.Ctx) error {
	userID, id, ok, err := h.parseActorAndID(c)
	if !ok {
		return err
	}
	res, err := h.svc.Cancel(c.Context(), userID, id)
	if err != nil {
		return writeWithdrawalError(c, err)
	}
	return c.JSON(fiber.Map{"message": "Withdrawal cancelled", "data": res})
}

// ─── Admin ─────────────────────────────────────────────────────────────────

// AdminListWithdrawals - GET /api/admin/withdrawals?status=&teacher_id=&page=&limit=
func (h *WithdrawalHandler) AdminListWithdrawals(c *fiber.Ctx) error {
	status, ok := parseWithdrawalStatusFilter(c)
	if !ok {
		return validationFailed(c, "status must be one of pending, approved, rejected, completed, cancelled")
	}
	var teacherID *uuid.UUID
	if raw := c.Query("teacher_id", ""); raw != "" {
		id, err := uuid.Parse(raw)
		if err != nil {
			return validationFailed(c, "teacher_id must be a UUID")
		}
		teacherID = &id
	}
	page, limit := parsePagination(c)
	res, err := h.svc.AdminList(c.Context(), teacherID, status, page, limit)
	if err != nil {
		return writeWithdrawalError(c, err)
	}
	return c.JSON(fiber.Map{"message": "OK", "data": res})
}

// AdminNegativeBalances - GET /api/admin/withdrawals/negative-balances
func (h *WithdrawalHandler) AdminNegativeBalances(c *fiber.Ctx) error {
	res, err := h.svc.NegativeBalances(c.Context())
	if err != nil {
		return writeWithdrawalError(c, err)
	}
	return c.JSON(fiber.Map{"message": "OK", "data": res})
}

func (h *WithdrawalHandler) parseActorAndID(c *fiber.Ctx) (uuid.UUID, uuid.UUID, bool, error) {
	actorID, err := parseUserID(c)
	if err != nil {
		return uuid.Nil, uuid.Nil, false, err
	}
	id, perr := uuid.Parse(c.Params("id"))
	if perr != nil {
		return uuid.Nil, uuid.Nil, false, c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"message": "Invalid withdrawal id", "error": "invalid_id"})
	}
	return actorID, id, true, nil
}

// ApproveWithdrawal - POST /api/admin/withdrawals/:id/approve
func (h *WithdrawalHandler) ApproveWithdrawal(c *fiber.Ctx) error {
	actorID, id, ok, err := h.parseActorAndID(c)
	if !ok {
		return err
	}
	res, err := h.svc.Approve(c.Context(), actorID, id)
	if err != nil {
		return writeWithdrawalError(c, err)
	}
	return c.JSON(fiber.Map{"message": "Withdrawal approved", "data": res})
}

// RejectWithdrawal - POST /api/admin/withdrawals/:id/reject {"reason"}
func (h *WithdrawalHandler) RejectWithdrawal(c *fiber.Ctx) error {
	actorID, id, ok, err := h.parseActorAndID(c)
	if !ok {
		return err
	}
	var req dto.RejectWithdrawalRequest
	if err := c.BodyParser(&req); err != nil {
		return validationFailed(c, "reason is required")
	}
	res, err := h.svc.Reject(c.Context(), actorID, id, req.Reason)
	if err != nil {
		return writeWithdrawalError(c, err)
	}
	return c.JSON(fiber.Map{"message": "Withdrawal rejected", "data": res})
}

// MarkWithdrawalCompleted - POST /api/admin/withdrawals/:id/mark-completed {"transaction_id"}
func (h *WithdrawalHandler) MarkWithdrawalCompleted(c *fiber.Ctx) error {
	actorID, id, ok, err := h.parseActorAndID(c)
	if !ok {
		return err
	}
	var req dto.MarkWithdrawalCompletedRequest
	if err := c.BodyParser(&req); err != nil {
		return validationFailed(c, "transaction_id is required")
	}
	res, err := h.svc.MarkCompleted(c.Context(), actorID, id, req.TransactionID)
	if err != nil {
		return writeWithdrawalError(c, err)
	}
	return c.JSON(fiber.Map{"message": "Withdrawal marked as completed", "data": res})
}
