package handler

import (
	"strconv"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"study.com/v1/internal/service"
)

// WalletHandler handles /api/wallet routes
type WalletHandler struct {
	walletService service.WalletServiceInterface
}

func NewWalletHandler(walletService service.WalletServiceInterface) *WalletHandler {
	return &WalletHandler{walletService: walletService}
}

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
	page, _ := strconv.Atoi(c.Query("page", "1"))
	limit, _ := strconv.Atoi(c.Query("limit", "20"))

	if page < 1 {
		page = 1
	}
	if limit < 1 || limit > 100 {
		limit = 20
	}

	result, err := h.walletService.GetTransactions(c.Context(), userID, txType, page, limit)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"code":    "ERR_GET_TRANSACTIONS",
			"message": err.Error(),
		})
	}

	return c.JSON(result)
}

// parseUserID extracts and validates user_id from Fiber locals (set by AuthMiddleware)
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
