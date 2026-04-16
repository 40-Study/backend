package handler

import (
	"strconv"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"study.com/v1/internal/dto"
	"study.com/v1/internal/service"
	"study.com/v1/internal/utils"
)

type CoinHandler struct {
	coinService service.CoinServiceInterface
}

func NewCoinHandler(coinService service.CoinServiceInterface) *CoinHandler {
	return &CoinHandler{coinService: coinService}
}

// ─── Wallet endpoints ───────────────────────────────────────────────────────

// GetWallet - GET /api/coins/wallet
func (h *CoinHandler) GetWallet(c *fiber.Ctx) error {
	userID, err := parseUserID(c)
	if err != nil {
		return err
	}

	result, err := h.coinService.GetWallet(c.Context(), userID)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"message": err.Error(),
		})
	}

	return c.JSON(fiber.Map{
		"message": "Wallet retrieved successfully",
		"data":    result,
	})
}

// GetTransactions - GET /api/coins/wallet/transactions
func (h *CoinHandler) GetTransactions(c *fiber.Ctx) error {
	userID, err := parseUserID(c)
	if err != nil {
		return err
	}

	page, _ := strconv.Atoi(c.Query("page", "1"))
	limit, _ := strconv.Atoi(c.Query("limit", "20"))
	txType := c.Query("type")

	result, err := h.coinService.GetTransactions(c.Context(), userID, txType, page, limit)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"message": err.Error(),
		})
	}

	return c.JSON(fiber.Map{
		"message": "Transactions retrieved successfully",
		"data":    result,
	})
}

// ─── Package endpoints ──────────────────────────────────────────────────────

// ListPackages - GET /api/coins/packages
func (h *CoinHandler) ListPackages(c *fiber.Ctx) error {
	result, err := h.coinService.ListPackages(c.Context())
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"message": err.Error(),
		})
	}

	return c.JSON(fiber.Map{
		"message": "Packages retrieved successfully",
		"data":    result,
	})
}

// GetPackage - GET /api/coins/packages/:id
func (h *CoinHandler) GetPackage(c *fiber.Ctx) error {
	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Invalid package ID",
		})
	}

	result, err := h.coinService.GetPackage(c.Context(), id)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"message": err.Error(),
		})
	}

	return c.JSON(fiber.Map{
		"message": "Package retrieved successfully",
		"data":    result,
	})
}

// ─── Purchase endpoints ─────────────────────────────────────────────────────

// CreatePurchase - POST /api/coins/purchases
func (h *CoinHandler) CreatePurchase(c *fiber.Ctx) error {
	userID, err := parseUserID(c)
	if err != nil {
		return err
	}

	var req dto.CreateCoinPurchaseRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Invalid request body",
		})
	}

	if errs := utils.ValidateStruct(req); len(errs) > 0 {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Validation failed",
			"errors":  errs,
		})
	}

	result, err := h.coinService.CreatePurchase(c.Context(), userID, req)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": err.Error(),
		})
	}

	return c.Status(fiber.StatusCreated).JSON(fiber.Map{
		"message": "Purchase created successfully",
		"data":    result,
	})
}

// ListPurchases - GET /api/coins/purchases
func (h *CoinHandler) ListPurchases(c *fiber.Ctx) error {
	userID, err := parseUserID(c)
	if err != nil {
		return err
	}

	page, _ := strconv.Atoi(c.Query("page", "1"))
	limit, _ := strconv.Atoi(c.Query("limit", "20"))

	result, err := h.coinService.ListPurchases(c.Context(), userID, page, limit)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"message": err.Error(),
		})
	}

	return c.JSON(fiber.Map{
		"message": "Purchases retrieved successfully",
		"data":    result,
	})
}

// GetPurchase - GET /api/coins/purchases/:id
func (h *CoinHandler) GetPurchase(c *fiber.Ctx) error {
	userID, err := parseUserID(c)
	if err != nil {
		return err
	}

	purchaseID, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Invalid purchase ID",
		})
	}

	result, err := h.coinService.GetPurchase(c.Context(), userID, purchaseID)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"message": err.Error(),
		})
	}

	return c.JSON(fiber.Map{
		"message": "Purchase retrieved successfully",
		"data":    result,
	})
}

// VerifyPurchase - POST /api/coins/purchases/:id/verify
func (h *CoinHandler) VerifyPurchase(c *fiber.Ctx) error {
	userID, err := parseUserID(c)
	if err != nil {
		return err
	}

	purchaseID, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Invalid purchase ID",
		})
	}

	result, err := h.coinService.VerifyPurchase(c.Context(), userID, purchaseID)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": err.Error(),
		})
	}

	return c.JSON(fiber.Map{
		"message": "Purchase verified and coins credited",
		"data":    result,
	})
}

// ─── Gift endpoints ─────────────────────────────────────────────────────────

// SendGift - POST /api/coins/gift
func (h *CoinHandler) SendGift(c *fiber.Ctx) error {
	userID, err := parseUserID(c)
	if err != nil {
		return err
	}

	var req dto.SendCoinGiftRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Invalid request body",
		})
	}

	if errs := utils.ValidateStruct(req); len(errs) > 0 {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Validation failed",
			"errors":  errs,
		})
	}

	result, err := h.coinService.SendGift(c.Context(), userID, req)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": err.Error(),
		})
	}

	return c.JSON(fiber.Map{
		"message": "Gift sent successfully",
		"data":    result,
	})
}

// ─── Admin endpoints ────────────────────────────────────────────────────────

// CreatePackage - POST /api/coins/admin/packages
func (h *CoinHandler) CreatePackage(c *fiber.Ctx) error {
	var req dto.CreateCoinPackageRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Invalid request body",
		})
	}

	if errs := utils.ValidateStruct(req); len(errs) > 0 {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Validation failed",
			"errors":  errs,
		})
	}

	result, err := h.coinService.CreatePackage(c.Context(), req)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": err.Error(),
		})
	}

	return c.Status(fiber.StatusCreated).JSON(fiber.Map{
		"message": "Package created successfully",
		"data":    result,
	})
}

// UpdatePackage - PUT /api/coins/admin/packages/:id
func (h *CoinHandler) UpdatePackage(c *fiber.Ctx) error {
	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Invalid package ID",
		})
	}

	var req dto.UpdateCoinPackageRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Invalid request body",
		})
	}

	result, err := h.coinService.UpdatePackage(c.Context(), id, req)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": err.Error(),
		})
	}

	return c.JSON(fiber.Map{
		"message": "Package updated successfully",
		"data":    result,
	})
}

// DeletePackage - DELETE /api/coins/admin/packages/:id
func (h *CoinHandler) DeletePackage(c *fiber.Ctx) error {
	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Invalid package ID",
		})
	}

	if err := h.coinService.DeletePackage(c.Context(), id); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": err.Error(),
		})
	}

	return c.JSON(fiber.Map{
		"message": "Package deleted successfully",
	})
}

// AdminAdjust - POST /api/coins/admin/adjust
func (h *CoinHandler) AdminAdjust(c *fiber.Ctx) error {
	var req dto.AdminAdjustCoinRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Invalid request body",
		})
	}

	if errs := utils.ValidateStruct(req); len(errs) > 0 {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Validation failed",
			"errors":  errs,
		})
	}

	result, err := h.coinService.AdminAdjust(c.Context(), req)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": err.Error(),
		})
	}

	return c.JSON(fiber.Map{
		"message": "Balance adjusted successfully",
		"data":    result,
	})
}
