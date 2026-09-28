package handler

import (
	"errors"
	"log"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"study.com/v1/internal/dto"
	"study.com/v1/internal/service"
	"study.com/v1/internal/utils"
)

// ParentLinkHandler — API liên kết phụ huynh-học sinh do phụ huynh khởi xướng (QA vòng 2 lane E).
// Mọi lỗi trả envelope {"message","code"} với thông điệp tiếng Việt.
type ParentLinkHandler struct {
	svc service.ParentLinkServiceInterface
}

func NewParentLinkHandler(svc service.ParentLinkServiceInterface) *ParentLinkHandler {
	return &ParentLinkHandler{svc: svc}
}

func linkFail(c *fiber.Ctx, status int, code, msg string) error {
	return c.Status(status).JSON(fiber.Map{"message": msg, "code": code})
}

// linkServiceError đổi lỗi service ra response. Lỗi nghiệp vụ mang sẵn status/code; lỗi hạ tầng
// chỉ ghi log đầy đủ, client nhận thông điệp chung (không lộ chuỗi lỗi DB).
func linkServiceError(c *fiber.Ctx, op string, err error) error {
	var le *service.ParentLinkError
	if errors.As(err, &le) {
		return linkFail(c, le.Status, le.Code, le.Message)
	}
	log.Printf("[ParentLink] %s: %v", op, err)
	return linkFail(c, fiber.StatusInternalServerError, "INTERNAL_ERROR", "Có lỗi xảy ra, vui lòng thử lại sau.")
}

func linkUserID(c *fiber.Ctx) (uuid.UUID, bool) {
	id, ok := c.Locals("user_id").(uuid.UUID)
	return id, ok && id != uuid.Nil
}

func (h *ParentLinkHandler) withUser(c *fiber.Ctx, fn func(userID uuid.UUID) error) error {
	userID, ok := linkUserID(c)
	if !ok {
		return linkFail(c, fiber.StatusUnauthorized, "UNAUTHORIZED", "Phiên đăng nhập không hợp lệ.")
	}
	return fn(userID)
}

func parseLinkParamID(c *fiber.Ctx, name string) (uuid.UUID, error) {
	id, err := uuid.Parse(c.Params(name))
	if err != nil {
		return uuid.Nil, linkFail(c, fiber.StatusBadRequest, "INVALID_ID", "Mã không hợp lệ.")
	}
	return id, nil
}

// CreateRequest POST /family/link-requests — phụ huynh gửi yêu cầu tới email học sinh.
func (h *ParentLinkHandler) CreateRequest(c *fiber.Ctx) error {
	return h.withUser(c, func(userID uuid.UUID) error {
		var req dto.CreateParentLinkRequestDto
		if err := c.BodyParser(&req); err != nil {
			return linkFail(c, fiber.StatusBadRequest, "INVALID_BODY", "Dữ liệu gửi lên không hợp lệ.")
		}
		if errs := utils.ValidateStruct(req); len(errs) > 0 {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
				"message": "Vui lòng nhập email học sinh hợp lệ và chọn mối quan hệ.",
				"code":    "VALIDATION_FAILED",
				"errors":  errs,
			})
		}
		out, err := h.svc.CreateRequest(c.Context(), userID, req)
		if err != nil {
			return linkServiceError(c, "create", err)
		}
		return c.Status(fiber.StatusCreated).JSON(fiber.Map{"message": "Đã gửi yêu cầu liên kết, chờ con xác nhận.", "data": out})
	})
}

// ListSent GET /family/link-requests/sent — phụ huynh xem yêu cầu đã gửi.
func (h *ParentLinkHandler) ListSent(c *fiber.Ctx) error {
	return h.withUser(c, func(userID uuid.UUID) error {
		out, err := h.svc.ListSent(c.Context(), userID)
		if err != nil {
			return linkServiceError(c, "list sent", err)
		}
		return c.JSON(fiber.Map{"message": "success", "data": out})
	})
}

// Cancel POST /family/link-requests/:id/cancel — phụ huynh rút yêu cầu đang chờ.
func (h *ParentLinkHandler) Cancel(c *fiber.Ctx) error {
	return h.withUser(c, func(userID uuid.UUID) error {
		id, err := parseLinkParamID(c, "id")
		if err != nil {
			return err
		}
		if err := h.svc.Cancel(c.Context(), userID, id); err != nil {
			return linkServiceError(c, "cancel", err)
		}
		return c.JSON(fiber.Map{"message": "Đã huỷ yêu cầu liên kết."})
	})
}

// ListIncoming GET /family/link-requests/incoming — học sinh xem yêu cầu chờ mình xác nhận.
func (h *ParentLinkHandler) ListIncoming(c *fiber.Ctx) error {
	return h.withUser(c, func(userID uuid.UUID) error {
		out, err := h.svc.ListIncoming(c.Context(), userID)
		if err != nil {
			return linkServiceError(c, "list incoming", err)
		}
		return c.JSON(fiber.Map{"message": "success", "data": out})
	})
}

// Respond POST /family/link-requests/:id/respond — học sinh xác nhận/từ chối.
func (h *ParentLinkHandler) Respond(c *fiber.Ctx) error {
	return h.withUser(c, func(userID uuid.UUID) error {
		id, err := parseLinkParamID(c, "id")
		if err != nil {
			return err
		}
		var req dto.RespondParentLinkRequestDto
		if err := c.BodyParser(&req); err != nil {
			return linkFail(c, fiber.StatusBadRequest, "INVALID_BODY", "Dữ liệu gửi lên không hợp lệ.")
		}
		if errs := utils.ValidateStruct(req); len(errs) > 0 {
			return linkFail(c, fiber.StatusBadRequest, "INVALID_ACTION", "Hành động không hợp lệ, chỉ nhận 'accept' hoặc 'reject'.")
		}
		if err := h.svc.Respond(c.Context(), userID, id, req.Action); err != nil {
			return linkServiceError(c, "respond", err)
		}
		msg := "Đã từ chối yêu cầu liên kết."
		if req.Action == "accept" {
			msg = "Đã xác nhận liên kết với phụ huynh."
		}
		return c.JSON(fiber.Map{"message": msg})
	})
}

// ListLinkedParents GET /family/parents — học sinh xem phụ huynh đang liên kết.
func (h *ParentLinkHandler) ListLinkedParents(c *fiber.Ctx) error {
	return h.withUser(c, func(userID uuid.UUID) error {
		out, err := h.svc.ListLinkedParents(c.Context(), userID)
		if err != nil {
			return linkServiceError(c, "list parents", err)
		}
		return c.JSON(fiber.Map{"message": "success", "data": out})
	})
}

// UnlinkChild DELETE /family/children/:id — phụ huynh huỷ liên kết với con.
func (h *ParentLinkHandler) UnlinkChild(c *fiber.Ctx) error {
	return h.withUser(c, func(userID uuid.UUID) error {
		childID, err := parseLinkParamID(c, "id")
		if err != nil {
			return err
		}
		if err := h.svc.UnlinkByParent(c.Context(), userID, childID); err != nil {
			return linkServiceError(c, "unlink child", err)
		}
		return c.JSON(fiber.Map{"message": "Đã huỷ liên kết."})
	})
}

// UnlinkParent DELETE /family/parents/:id — học sinh huỷ liên kết với phụ huynh.
func (h *ParentLinkHandler) UnlinkParent(c *fiber.Ctx) error {
	return h.withUser(c, func(userID uuid.UUID) error {
		parentID, err := parseLinkParamID(c, "id")
		if err != nil {
			return err
		}
		if err := h.svc.UnlinkByStudent(c.Context(), userID, parentID); err != nil {
			return linkServiceError(c, "unlink parent", err)
		}
		return c.JSON(fiber.Map{"message": "Đã huỷ liên kết."})
	})
}
