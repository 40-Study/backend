package handler

import (
	"errors"
	"fmt"
	"log"

	"github.com/gofiber/fiber/v2"
	"study.com/v1/internal/dto"
	"study.com/v1/internal/middleware"
	"study.com/v1/internal/service"
	"study.com/v1/internal/utils"
)

// BroadcastDeliveredLocal: khoá c.Locals ghi số người ĐÃ nhận thông báo (cả khi đợt gửi lỗi giữa chừng), để
// middleware audit (phase 8) đọc vào metadata mà không phải parse lại response.
const BroadcastDeliveredLocal = "broadcast_delivered"

// AdminBroadcastHandler — thông báo hệ thống do quản trị viên gửi (contract C3). Mọi lỗi trả envelope
// {"message","code"} tiếng Việt; lỗi hạ tầng chỉ ghi log, client nhận thông điệp chung.
type AdminBroadcastHandler struct {
	svc service.BroadcastServiceInterface
}

func NewAdminBroadcastHandler(svc service.BroadcastServiceInterface) *AdminBroadcastHandler {
	return &AdminBroadcastHandler{svc: svc}
}

func broadcastFail(c *fiber.Ctx, status int, code, msg string) error {
	return c.Status(status).JSON(fiber.Map{"message": msg, "code": code})
}

func broadcastServiceError(c *fiber.Ctx, op string, err error) error {
	var be *service.BroadcastError
	if errors.As(err, &be) {
		return broadcastFail(c, be.Status, be.Code, be.Message)
	}
	var partial *service.BroadcastPartialError
	if errors.As(err, &partial) {
		log.Printf("[Broadcast] %s: gửi dở dang sau %d người nhận: %v", op, partial.Delivered, partial.Cause)
		c.Locals(BroadcastDeliveredLocal, partial.Delivered)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"message":   fmt.Sprintf("Gửi thông báo bị gián đoạn sau khi đã gửi tới %d người. Không gửi lại toàn bộ để tránh trùng thông báo.", partial.Delivered),
			"code":      "BROADCAST_PARTIAL",
			"delivered": partial.Delivered,
		})
	}
	log.Printf("[Broadcast] %s: %v", op, err)
	return broadcastFail(c, fiber.StatusInternalServerError, "INTERNAL_ERROR", "Có lỗi xảy ra, vui lòng thử lại sau.")
}

func broadcastInvalid(c *fiber.Ctx, msg string, errs []utils.ValidationError) error {
	body := fiber.Map{"message": msg, "code": "VALIDATION_FAILED"}
	if len(errs) > 0 {
		body["errors"] = errs
	}
	return c.Status(fiber.StatusBadRequest).JSON(body)
}

// Preview POST /admin/notifications/broadcast/preview — đếm người sẽ nhận, không gửi gì.
func (h *AdminBroadcastHandler) Preview(c *fiber.Ctx) error {
	var req dto.BroadcastPreviewRequestDTO
	if err := c.BodyParser(&req); err != nil {
		return broadcastInvalid(c, "Dữ liệu gửi lên không hợp lệ.", nil)
	}
	if errs := utils.ValidateStruct(req); len(errs) > 0 {
		return broadcastInvalid(c, "Vui lòng chọn đối tượng nhận hợp lệ.", errs)
	}
	out, err := h.svc.Preview(c.Context(), req)
	if err != nil {
		return broadcastServiceError(c, "preview", err)
	}
	return c.JSON(fiber.Map{"message": "success", "data": out})
}

// Send POST /admin/notifications/broadcast — gửi thông báo hệ thống.
func (h *AdminBroadcastHandler) Send(c *fiber.Ctx) error {
	var req dto.BroadcastRequestDTO
	if err := c.BodyParser(&req); err != nil {
		return broadcastInvalid(c, "Dữ liệu gửi lên không hợp lệ.", nil)
	}
	if errs := utils.ValidateStruct(req); len(errs) > 0 {
		return broadcastInvalid(c, "Vui lòng nhập tiêu đề (tối đa 255 ký tự), nội dung (tối đa 2000 ký tự) và chọn đối tượng nhận hợp lệ.", errs)
	}
	out, err := h.svc.Send(c.Context(), req)
	if err != nil {
		var partial *service.BroadcastPartialError
		if errors.As(err, &partial) {
			// Gửi dở dang là 500 nhưng một phần người nhận ĐÃ nhận, không thu hồi được: vẫn ghi nhật ký
			// (ngoại lệ hẹp của D6) để quản trị viên biết số người đã nhận.
			middleware.SetAuditRecordOnFailure(c)
			middleware.SetAuditMeta(c, broadcastAuditMeta(req.Audience, req.Roles, req.Title, partial.Delivered))
			middleware.SetAuditMeta(c, map[string]any{"partial": true})
		}
		return broadcastServiceError(c, "send", err)
	}
	c.Locals(BroadcastDeliveredLocal, out.RecipientCount)
	middleware.SetAuditMeta(c, broadcastAuditMeta(out.Audience, out.Roles, req.Title, out.RecipientCount))
	return c.Status(fiber.StatusCreated).JSON(fiber.Map{"message": "Đã gửi thông báo", "data": out})
}

// broadcastAuditMeta: metadata dòng nhật ký notification.broadcast (audience, roles, recipient_count, title).
// recipient_count là số người đã nhận thật (khi gửi dở dang là số đã gửi được).
func broadcastAuditMeta(audience string, roles []string, title string, delivered int64) map[string]any {
	meta := map[string]any{
		"audience":        audience,
		"recipient_count": delivered,
		"title":           title,
	}
	if len(roles) > 0 {
		meta["roles"] = roles
	}
	return meta
}
