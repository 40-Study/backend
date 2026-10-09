package handler

import (
	"errors"
	"log"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"study.com/v1/internal/dto"
	"study.com/v1/internal/middleware"
	"study.com/v1/internal/model"
	"study.com/v1/internal/repository"
	"study.com/v1/internal/service"
	"study.com/v1/internal/utils"
)

// AdminOrderHandler — đơn hàng admin + hoàn tiền + báo cáo doanh thu nền tảng thật + cấu hình %
// phí nền tảng (quyết định chủ dự án 27/09/2026). Route gate bằng permission ở router
// (PAYMENTS_MANAGE / DASHBOARD_VIEW_GLOBAL / SYSTEM_SETTINGS_MANAGE) — handler không tự kiểm
// quyền lại (đã qua middleware trước khi tới đây).
type AdminOrderHandler struct {
	orderService       service.OrderServiceInterface
	adminOrderService  service.AdminOrderServiceInterface
	platformFeeService service.PlatformSettingServiceInterface
}

func NewAdminOrderHandler(
	orderService service.OrderServiceInterface,
	adminOrderService service.AdminOrderServiceInterface,
	platformFeeService service.PlatformSettingServiceInterface,
) *AdminOrderHandler {
	return &AdminOrderHandler{
		orderService:       orderService,
		adminOrderService:  adminOrderService,
		platformFeeService: platformFeeService,
	}
}

// ListOrders - GET /api/orders/admin
func (h *AdminOrderHandler) ListOrders(c *fiber.Ctx) error {
	page, _ := strconv.Atoi(c.Query("page", "1"))
	limit, _ := strconv.Atoi(c.Query("limit", "20"))

	status := c.Query("status", "")
	// B7 (QA vòng 2 N-06): status lạ (vd "bogus") trước đây lọt xuống WHERE và trả 200 rỗng, khiến
	// client tưởng "không có đơn". Kiểm theo OrderStatuses (SSOT của CHECK constraint).
	if status != "" && !slices.Contains(model.OrderStatuses, status) {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Trạng thái đơn hàng không hợp lệ",
			"error":   "invalid_status",
		})
	}

	filter := repository.AdminOrderFilter{
		Status: status,
		Page:   page,
		Limit:  limit,
	}

	if userIDStr := c.Query("user_id", ""); userIDStr != "" {
		userID, err := uuid.Parse(userIDStr)
		if err != nil {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
				"message": "Invalid user_id",
				"error":   "invalid_user_id",
			})
		}
		filter.UserID = &userID
	}

	if fromStr := c.Query("from", ""); fromStr != "" {
		from, err := time.Parse(time.RFC3339, fromStr)
		if err != nil {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
				"message": "Invalid from (RFC3339 required)",
				"error":   "invalid_from",
			})
		}
		filter.From = &from
	}

	if toStr := c.Query("to", ""); toStr != "" {
		to, err := time.Parse(time.RFC3339, toStr)
		if err != nil {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
				"message": "Invalid to (RFC3339 required)",
				"error":   "invalid_to",
			})
		}
		filter.To = &to
	}

	resp, err := h.adminOrderService.ListOrders(c.Context(), filter)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"message": "Failed to list orders",
			"error":   err.Error(),
		})
	}

	return c.JSON(fiber.Map{"message": "Success", "data": resp})
}

// GetOrder - GET /api/orders/admin/:id — tái dùng OrderService.GetOrderByID(isAdmin=true), KHÔNG
// sửa service (phase-02 contract).
func (h *AdminOrderHandler) GetOrder(c *fiber.Ctx) error {
	orderID, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Invalid order ID",
			"error":   "invalid_id",
		})
	}

	// actorUserID không quan trọng ở đây vì isAdmin=true bỏ qua check chủ sở hữu — dùng
	// uuid.Nil an toàn (GetOrderByID chỉ so sánh order.UserID != actorUserID KHI !isAdmin).
	order, err := h.orderService.GetOrderByID(c.Context(), orderID, uuid.Nil, true)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"message": "Order not found",
			"error":   "not_found",
		})
	}

	return c.JSON(fiber.Map{"message": "Success", "data": order})
}

// RefundOrder - POST /api/orders/admin/:id/refund
func (h *AdminOrderHandler) RefundOrder(c *fiber.Ctx) error {
	actorID, ok := getAuthUserID(c)
	if !ok {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"message": "Unauthorized"})
	}

	orderID, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Invalid order ID",
			"error":   "invalid_id",
		})
	}

	var req dto.RefundOrderRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Invalid request body",
			"error":   "invalid_request",
		})
	}
	// Review #76 MINOR: trim TRƯỚC khi validate. Trước đây validate chạy trên chuỗi thô nên
	// transaction_ref = "   " lọt qua `required` rồi bị lưu thành "" (trái quyết định #1: hoàn tiền
	// phải kèm mã giao dịch).
	req.TransactionRef = strings.TrimSpace(req.TransactionRef)
	if errs := utils.ValidateStruct(req); len(errs) > 0 {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Validation failed",
			"errors":  errs,
		})
	}

	resp, err := h.adminOrderService.RefundOrder(c.Context(), actorID, orderID, req.Reason, req.RefundMethod, req.TransactionRef)
	if err != nil {
		switch {
		case errors.Is(err, service.ErrOrderNotFound):
			return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
				"message": "Order not found",
				"error":   "not_found",
			})
		case errors.Is(err, service.ErrOrderAlreadyRefunded):
			return c.Status(fiber.StatusConflict).JSON(fiber.Map{
				"message": "Order already refunded",
				"error":   "already_refunded",
			})
		case errors.Is(err, service.ErrOrderNotRefundable):
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
				"message": "Only completed orders can be refunded",
				"error":   "invalid_status",
			})
		default:
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
				"message": "Failed to refund order",
				"error":   err.Error(),
			})
		}
	}

	return c.JSON(fiber.Map{"message": "Order refunded", "data": resp})
}

// MarkLatePaymentRefunded - POST /api/orders/admin/:id/late-refund
//
// Admin xác nhận đã hoàn khoản tiền về muộn của đơn đã đóng. Body tuỳ chọn ({note, transaction_ref}),
// có thể để trống. Idempotent: gọi lại vẫn 200 (already_recorded = true).
func (h *AdminOrderHandler) MarkLatePaymentRefunded(c *fiber.Ctx) error {
	actorID, ok := getAuthUserID(c)
	if !ok {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"message": "Unauthorized"})
	}

	orderID, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Invalid order ID",
			"error":   "invalid_id",
		})
	}

	var req dto.LateRefundRequest
	if len(c.Body()) > 0 {
		if err := c.BodyParser(&req); err != nil {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
				"message": "Invalid request body",
				"error":   "invalid_request",
			})
		}
	}
	req.Note = strings.TrimSpace(req.Note)
	req.TransactionRef = strings.TrimSpace(req.TransactionRef)
	if errs := utils.ValidateStruct(req); len(errs) > 0 {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Validation failed",
			"errors":  errs,
		})
	}

	resp, err := h.adminOrderService.MarkLatePaymentRefunded(c.Context(), actorID, orderID, req.Note, req.TransactionRef, req.Refs)
	if err != nil {
		switch {
		case errors.Is(err, service.ErrOrderNotFound):
			return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
				"message": "Order not found",
				"error":   "not_found",
			})
		case errors.Is(err, service.ErrLateRefundUnknownRef):
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
				"message": "One or more refunded transactions do not belong to this order",
				"error":   "unknown_late_payment",
			})
		case errors.Is(err, service.ErrLateRefundNotNeeded):
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
				"message": "Order has no late payment awaiting refund",
				"error":   "refund_not_needed",
			})
		default:
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
				"message": "Failed to record late refund",
				"error":   err.Error(),
			})
		}
	}

	return c.JSON(fiber.Map{"message": "Late refund recorded", "data": resp})
}

// GetRevenueReport - GET /api/admin/reports/revenue
func (h *AdminOrderHandler) GetRevenueReport(c *fiber.Ctx) error {
	to := time.Now()
	from := to.AddDate(0, 0, -30)

	if fromStr := c.Query("from", ""); fromStr != "" {
		parsed, err := time.Parse(time.RFC3339, fromStr)
		if err != nil {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
				"message": "Invalid from (RFC3339 required)",
				"error":   "invalid_from",
			})
		}
		from = parsed
	}
	if toStr := c.Query("to", ""); toStr != "" {
		parsed, err := time.Parse(time.RFC3339, toStr)
		if err != nil {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
				"message": "Invalid to (RFC3339 required)",
				"error":   "invalid_to",
			})
		}
		to = parsed
	}

	report, err := h.adminOrderService.GetRevenueReport(c.Context(), from, to)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"message": "Failed to get revenue report",
			"error":   err.Error(),
		})
	}

	return c.JSON(fiber.Map{"message": "Success", "data": report})
}

// GetPlatformFeeSetting - GET /api/admin/settings/platform-fee
func (h *AdminOrderHandler) GetPlatformFeeSetting(c *fiber.Ctx) error {
	percent, err := h.platformFeeService.GetPlatformFeePercent(c.Context())
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"message": "Failed to get platform fee setting",
			"error":   err.Error(),
		})
	}
	return c.JSON(fiber.Map{
		"message": "Success",
		"data":    dto.PlatformFeeSettingResponse{PlatformFeePercent: percent},
	})
}

// UpdatePlatformFeeSetting - PUT /api/admin/settings/platform-fee
func (h *AdminOrderHandler) UpdatePlatformFeeSetting(c *fiber.Ctx) error {
	actorID, ok := getAuthUserID(c)
	if !ok {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"message": "Unauthorized"})
	}

	var req dto.UpdatePlatformFeeRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Invalid request body",
			"error":   "invalid_request",
		})
	}

	// Giá trị cũ để ghi nhật ký (old/new). Đọc lỗi thì chỉ log: không chặn việc đổi phí, nhật ký thiếu "old".
	oldPercent, oldErr := h.platformFeeService.GetPlatformFeePercent(c.Context())
	if oldErr != nil {
		log.Printf("[Audit] không đọc được phí nền tảng cũ trước khi cập nhật: %v", oldErr)
	}

	if err := h.platformFeeService.SetPlatformFeePercent(c.Context(), actorID, req.PlatformFeePercent); err != nil {
		if errors.Is(err, service.ErrInvalidPlatformFeePercent) {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
				"message": "platform_fee_percent must be between 0 and 100",
				"error":   "invalid_percent",
			})
		}
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"message": "Failed to update platform fee setting",
			"error":   err.Error(),
		})
	}

	feeMeta := map[string]any{"new": req.PlatformFeePercent.String()}
	if oldErr == nil {
		feeMeta["old"] = oldPercent.String()
	}
	middleware.SetAuditTarget(c, "platform_fee")
	middleware.SetAuditMeta(c, feeMeta)

	return c.JSON(fiber.Map{
		"message": "Platform fee updated",
		"data":    dto.PlatformFeeSettingResponse{PlatformFeePercent: req.PlatformFeePercent},
	})
}
