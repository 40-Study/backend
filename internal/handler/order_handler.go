package handler

import (
	"strconv"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"study.com/v1/internal/dto"
	"study.com/v1/internal/service"
)

type OrderHandler struct {
	orderService   service.OrderServiceInterface
	paymentService service.PaymentServiceInterface
}

func NewOrderHandler(orderService service.OrderServiceInterface, paymentService service.PaymentServiceInterface) *OrderHandler {
	return &OrderHandler{
		orderService:   orderService,
		paymentService: paymentService,
	}
}

func (h *OrderHandler) CreateOrder(c *fiber.Ctx) error {
	userIDStr := c.Locals("user_id")
	if userIDStr == nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"code":    "ERR_UNAUTHORIZED",
			"message": "Unauthorized",
		})
	}

	userID, err := uuid.Parse(userIDStr.(string))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"code":    "ERR_INVALID_USER",
			"message": "Invalid user ID",
		})
	}

	var req dto.CreateOrderRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"code":    "ERR_INVALID_REQUEST",
			"message": "Invalid request body",
		})
	}

	order, err := h.orderService.CreateOrder(c.Context(), userID, req)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"code":    "ERR_CREATE_ORDER",
			"message": err.Error(),
		})
	}

	return c.Status(fiber.StatusCreated).JSON(order)
}

// GetOrder - GET /api/v1/orders/:id
func (h *OrderHandler) GetOrder(c *fiber.Ctx) error {
	userIDStr := c.Locals("user_id")
	if userIDStr == nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"code":    "ERR_UNAUTHORIZED",
			"message": "Unauthorized",
		})
	}

	orderID, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"code":    "ERR_INVALID_ID",
			"message": "Invalid order ID",
		})
	}

	order, err := h.orderService.GetOrderByID(c.Context(), orderID)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"code":    "ERR_NOT_FOUND",
			"message": "Order not found",
		})
	}

	return c.JSON(order)
}

// GetUserOrders - GET /api/v1/orders/me
func (h *OrderHandler) GetUserOrders(c *fiber.Ctx) error {
	userIDStr := c.Locals("user_id")
	if userIDStr == nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"code":    "ERR_UNAUTHORIZED",
			"message": "Unauthorized",
		})
	}

	userID, err := uuid.Parse(userIDStr.(string))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"code":    "ERR_INVALID_USER",
			"message": "Invalid user ID",
		})
	}

	page, _ := strconv.Atoi(c.Query("page", "1"))
	limit, _ := strconv.Atoi(c.Query("limit", "10"))
	status := c.Query("status", "")

	orders, err := h.orderService.GetUserOrders(c.Context(), userID, page, limit, status)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"code":    "ERR_GET_ORDERS",
			"message": err.Error(),
		})
	}

	return c.JSON(orders)
}

// CancelOrder - POST /api/v1/orders/:id/cancel
func (h *OrderHandler) CancelOrder(c *fiber.Ctx) error {
	userIDStr := c.Locals("user_id")
	if userIDStr == nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"code":    "ERR_UNAUTHORIZED",
			"message": "Unauthorized",
		})
	}

	userID, err := uuid.Parse(userIDStr.(string))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"code":    "ERR_INVALID_USER",
			"message": "Invalid user ID",
		})
	}

	orderID, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"code":    "ERR_INVALID_ID",
			"message": "Invalid order ID",
		})
	}

	var req dto.CancelOrderRequest
	c.BodyParser(&req)

	err = h.orderService.CancelOrder(c.Context(), userID, orderID, req.Reason)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"code":    "ERR_CANCEL_ORDER",
			"message": err.Error(),
		})
	}

	return c.JSON(fiber.Map{
		"message": "Order cancelled successfully",
	})
}

// CreatePaymentIntent - POST /api/v1/orders/:id/payment-intent
func (h *OrderHandler) CreatePaymentIntent(c *fiber.Ctx) error {
	userIDStr := c.Locals("user_id")
	if userIDStr == nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"code":    "ERR_UNAUTHORIZED",
			"message": "Unauthorized",
		})
	}

	userID, err := uuid.Parse(userIDStr.(string))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"code":    "ERR_INVALID_USER",
			"message": "Invalid user ID",
		})
	}

	orderID, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"code":    "ERR_INVALID_ID",
			"message": "Invalid order ID",
		})
	}

	var req dto.CreatePaymentIntentRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"code":    "ERR_INVALID_REQUEST",
			"message": "Invalid request body",
		})
	}

	paymentIntent, err := h.paymentService.CreatePaymentIntent(c.Context(), userID, orderID, req.PaymentMethod)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"code":    "ERR_CREATE_PAYMENT",
			"message": err.Error(),
		})
	}

	return c.JSON(paymentIntent)
}

// GetPaymentStatus - GET /api/v1/orders/:id/payment-status
// This will call gRPC to check transaction status
func (h *OrderHandler) GetPaymentStatus(c *fiber.Ctx) error {
	orderID, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"code":    "ERR_INVALID_ID",
			"message": "Invalid order ID",
		})
	}

	status, err := h.paymentService.GetPaymentStatus(c.Context(), orderID)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"code":    "ERR_NOT_FOUND",
			"message": err.Error(),
		})
	}

	return c.JSON(status)
}

// CheckPayment - POST /api/v1/orders/:id/check-payment
// Explicitly trigger a check with the transaction service via gRPC
func (h *OrderHandler) CheckPayment(c *fiber.Ctx) error {
	orderID, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"code":    "ERR_INVALID_ID",
			"message": "Invalid order ID",
		})
	}

	status, err := h.paymentService.CheckAndProcessPayment(c.Context(), orderID)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"code":    "ERR_CHECK_PAYMENT",
			"message": err.Error(),
		})
	}

	return c.JSON(status)
}
