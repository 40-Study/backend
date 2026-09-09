package handler

import (
	"strconv"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"study.com/v1/internal/dto"
	"study.com/v1/internal/middleware"
	"study.com/v1/internal/service"
	"study.com/v1/internal/utils"
)

type OrderHandler struct {
	orderService   service.OrderServiceInterface
	paymentService service.PaymentServiceInterface
	permChecker    *middleware.PermissionChecker
}

func NewOrderHandler(orderService service.OrderServiceInterface, paymentService service.PaymentServiceInterface, permChecker *middleware.PermissionChecker) *OrderHandler {
	return &OrderHandler{
		orderService:   orderService,
		paymentService: paymentService,
		permChecker:    permChecker,
	}
}

// orderErrorStatus ánh xạ lỗi phân quyền (H-06, audit 260909 vòng 2) sang HTTP 403; trả 0 khi
// không nhận diện được để caller giữ nguyên xử lý hiện có.
func orderErrorStatus(err error) int {
	switch err {
	case service.ErrOrderForbidden:
		return fiber.StatusForbidden
	default:
		return 0
	}
}

// getAuthUserID lấy user_id đã được AuthMiddleware set vào Locals dưới dạng uuid.UUID.
//
// H-06 (audit 260909 vòng 2) — bug tiềm ẩn tìm thấy khi sửa IDOR: TOÀN BỘ handler trong file
// này trước đây đọc Locals("user_id") rồi ép kiểu ".(string)" (userIDStr.(string)), trong khi
// AuthMiddleware.set thẳng uuid.UUID (c.Locals("user_id", claims.UserID) — xem
// auth_middleware.go). Type assertion 1 giá trị sai kiểu sẽ PANIC (không phải trả lỗi) —
// nghĩa là MỌI request đã đăng nhập tới /api/orders/* đều panic (bị recover middleware
// bắt lại thành 500) trước khi chạy tới bất kỳ logic nào, kể cả check quyền sở hữu H-06 đang
// sửa. Không thể verify IDOR fix nếu handler không chạy được tới đó nên phải sửa cùng lúc.
func getAuthUserID(c *fiber.Ctx) (uuid.UUID, bool) {
	userID, ok := c.Locals("user_id").(uuid.UUID)
	return userID, ok
}

func (h *OrderHandler) CreateOrder(c *fiber.Ctx) error {
	userID, ok := getAuthUserID(c)
	if !ok {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"code":    "ERR_UNAUTHORIZED",
			"message": "Unauthorized",
		})
	}

	var req dto.CreateOrderRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"code":    "ERR_INVALID_REQUEST",
			"message": "Invalid request body",
		})
	}

	// M-01 (audit 260909 vòng 2): trước đây handler này không gọi ValidateStruct lần nào —
	// tag validate trên DTO (nếu có) hoàn toàn vô tác dụng.
	if errs := utils.ValidateStruct(req); len(errs) > 0 {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"code":    "ERR_VALIDATION",
			"message": "Validation failed",
			"errors":  errs,
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
	userID, ok := getAuthUserID(c)
	if !ok {
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

	isAdmin := isAdminActor(c, h.permChecker, userID)
	order, err := h.orderService.GetOrderByID(c.Context(), orderID, userID, isAdmin)
	if err != nil {
		if status := orderErrorStatus(err); status != 0 {
			return c.Status(status).JSON(fiber.Map{
				"code":    "ERR_FORBIDDEN",
				"message": err.Error(),
			})
		}
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"code":    "ERR_NOT_FOUND",
			"message": "Order not found",
		})
	}

	return c.JSON(order)
}

// GetUserOrders - GET /api/v1/orders/me
func (h *OrderHandler) GetUserOrders(c *fiber.Ctx) error {
	userID, ok := getAuthUserID(c)
	if !ok {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"code":    "ERR_UNAUTHORIZED",
			"message": "Unauthorized",
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
	userID, ok := getAuthUserID(c)
	if !ok {
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

	var req dto.CancelOrderRequest
	c.BodyParser(&req)

	isAdmin := isAdminActor(c, h.permChecker, userID)
	err = h.orderService.CancelOrder(c.Context(), userID, orderID, isAdmin, req.Reason)
	if err != nil {
		if status := orderErrorStatus(err); status != 0 {
			return c.Status(status).JSON(fiber.Map{
				"code":    "ERR_FORBIDDEN",
				"message": err.Error(),
			})
		}
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
	userID, ok := getAuthUserID(c)
	if !ok {
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

	var req dto.CreatePaymentIntentRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"code":    "ERR_INVALID_REQUEST",
			"message": "Invalid request body",
		})
	}

	// M-01 (audit 260909 vòng 2): xem ghi chú ở CreateOrder.
	if errs := utils.ValidateStruct(req); len(errs) > 0 {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"code":    "ERR_VALIDATION",
			"message": "Validation failed",
			"errors":  errs,
		})
	}

	isAdmin := isAdminActor(c, h.permChecker, userID)
	paymentIntent, err := h.paymentService.CreatePaymentIntent(c.Context(), userID, orderID, isAdmin, req.PaymentMethod)
	if err != nil {
		if status := orderErrorStatus(err); status != 0 {
			return c.Status(status).JSON(fiber.Map{
				"code":    "ERR_FORBIDDEN",
				"message": err.Error(),
			})
		}
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
	// H-06 (audit 260909 vòng 2): trước đây route này KHÔNG đọc user_id gì cả — bất kỳ user
	// đăng nhập nào biết orderID cũng xem được trạng thái thanh toán đơn hàng của người khác.
	userID, ok := getAuthUserID(c)
	if !ok {
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

	isAdmin := isAdminActor(c, h.permChecker, userID)
	status, err := h.paymentService.GetPaymentStatus(c.Context(), orderID, userID, isAdmin)
	if err != nil {
		if respStatus := orderErrorStatus(err); respStatus != 0 {
			return c.Status(respStatus).JSON(fiber.Map{
				"code":    "ERR_FORBIDDEN",
				"message": err.Error(),
			})
		}
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
	// H-06: cùng lý do GetPaymentStatus — route trước đây không đọc user_id, ai cũng trigger
	// check + xem kết quả thanh toán đơn hàng của người khác được.
	userID, ok := getAuthUserID(c)
	if !ok {
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

	isAdmin := isAdminActor(c, h.permChecker, userID)
	status, err := h.paymentService.CheckAndProcessPayment(c.Context(), orderID, userID, isAdmin)
	if err != nil {
		if respStatus := orderErrorStatus(err); respStatus != 0 {
			return c.Status(respStatus).JSON(fiber.Map{
				"code":    "ERR_FORBIDDEN",
				"message": err.Error(),
			})
		}
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"code":    "ERR_CHECK_PAYMENT",
			"message": err.Error(),
		})
	}

	return c.JSON(status)
}
