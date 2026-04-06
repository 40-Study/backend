package handler

import (
	"strconv"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"study.com/v1/internal/dto"
	"study.com/v1/internal/service"
)

// WalletHandler handles /api/wallet routes
type WalletHandler struct {
	walletService service.WalletServiceInterface
}

func NewWalletHandler(walletService service.WalletServiceInterface) *WalletHandler {
	return &WalletHandler{walletService: walletService}
}

// ─── Student endpoints ──────────────────────────────────────────────────────

// GetWallet - GET /api/wallet/me
func (h *WalletHandler) GetWallet(c *fiber.Ctx) error {
	userID, err := parseUserID(c)
	if err != nil {
		return err
	}

	wallet, err := h.walletService.GetWallet(c.Context(), userID)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"code":    "ERR_GET_WALLET",
			"message": err.Error(),
		})
	}

	return c.JSON(wallet)
}

// GetTransactions - GET /api/wallet/transactions?type=income|expense&page=1&limit=20
func (h *WalletHandler) GetTransactions(c *fiber.Ctx) error {
	userID, err := parseUserID(c)
	if err != nil {
		return err
	}

	txType := c.Query("type", "")
	page, limit := parsePagination(c)

	result, err := h.walletService.GetTransactions(c.Context(), userID, txType, page, limit)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"code":    "ERR_GET_TRANSACTIONS",
			"message": err.Error(),
		})
	}

	return c.JSON(result)
}

// ─── Teacher endpoints ──────────────────────────────────────────────────────

// GetTeacherWallet - GET /api/wallet/teacher/me
func (h *WalletHandler) GetTeacherWallet(c *fiber.Ctx) error {
	userID, err := parseUserID(c)
	if err != nil {
		return err
	}

	wallet, err := h.walletService.GetTeacherWallet(c.Context(), userID)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"code":    "ERR_GET_TEACHER_WALLET",
			"message": err.Error(),
		})
	}

	return c.JSON(wallet)
}

// GetTeacherTransactions - GET /api/wallet/teacher/transactions?type=income|refund&page=1&limit=20
func (h *WalletHandler) GetTeacherTransactions(c *fiber.Ctx) error {
	userID, err := parseUserID(c)
	if err != nil {
		return err
	}

	txType := c.Query("type", "")
	page, limit := parsePagination(c)

	result, err := h.walletService.GetTeacherTransactions(c.Context(), userID, txType, page, limit)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"code":    "ERR_GET_TEACHER_TRANSACTIONS",
			"message": err.Error(),
		})
	}

	return c.JSON(result)
}

// UpdateBankInfo - PUT /api/wallet/teacher/bank-info
func (h *WalletHandler) UpdateBankInfo(c *fiber.Ctx) error {
	userID, err := parseUserID(c)
	if err != nil {
		return err
	}

	var req dto.UpdateBankInfoRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"code":    "ERR_INVALID_BODY",
			"message": "Invalid request body",
		})
	}

	if req.BankName == "" || req.BankAccountNumber == "" || req.BankAccountName == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"code":    "ERR_VALIDATION",
			"message": "bank_name, bank_account_number, and bank_account_name are required",
		})
	}

	if err := h.walletService.UpdateBankInfo(c.Context(), userID, req); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"code":    "ERR_UPDATE_BANK_INFO",
			"message": err.Error(),
		})
	}

	return c.JSON(fiber.Map{"message": "Bank info updated"})
}

// ─── Helpers ────────────────────────────────────────────────────────────────

func parseUserID(c *fiber.Ctx) (uuid.UUID, error) {
	userIDStr := c.Locals("user_id")
	if userIDStr == nil {
		c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"code":    "ERR_UNAUTHORIZED",
			"message": "Unauthorized",
		})
		return uuid.Nil, fiber.ErrUnauthorized
	}

	userID, err := uuid.Parse(userIDStr.(string))
	if err != nil {
		c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"code":    "ERR_INVALID_USER",
			"message": "Invalid user ID",
		})
		return uuid.Nil, fiber.ErrBadRequest
	}

	return userID, nil
}

func parsePagination(c *fiber.Ctx) (int, int) {
	page, _ := strconv.Atoi(c.Query("page", "1"))
	limit, _ := strconv.Atoi(c.Query("limit", "20"))
	if page < 1 {
		page = 1
	}
	if limit < 1 || limit > 100 {
		limit = 20
	}
	return page, limit
}
