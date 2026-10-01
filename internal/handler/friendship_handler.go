package handler

import (
	"errors"
	"math"
	"log"
	"strconv"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"study.com/v1/internal/dto"
	"study.com/v1/internal/service"
)

// FriendshipHandler — /api/friends (contract-api.md §1). Mọi route đã qua AuthMiddleware; vai trò STUDENT
// được kiểm ở service (FRIEND_ROLE_NOT_ALLOWED).
type FriendshipHandler struct {
	svc *service.FriendshipService
}

func NewFriendshipHandler(svc *service.FriendshipService) *FriendshipHandler {
	return &FriendshipHandler{svc: svc}
}

// friendErrors ánh xạ lỗi service → (HTTP status, code, message tiếng Việt) đúng bảng "Mã lỗi bạn bè".
// Thông điệp cố ý trung tính ở các mã có thể lộ thông tin (cooldown không nói "bị từ chối", chặn không
// phân biệt ai chặn ai).
var friendErrors = []struct {
	err    error
	status int
	code   string
	msg    string
}{
	{service.ErrFriendSelfRequest, fiber.StatusBadRequest, "FRIEND_SELF_REQUEST", "Bạn không thể tự kết bạn với chính mình"},
	{service.ErrFriendRoleNotAllowed, fiber.StatusForbidden, "FRIEND_ROLE_NOT_ALLOWED", "Tính năng bạn bè chỉ dành cho học viên"},
	{service.ErrFriendRequestNotAllowed, fiber.StatusForbidden, "FRIEND_REQUEST_NOT_ALLOWED", "Bạn không thể gửi lời mời kết bạn cho người này"},
	{service.ErrFriendUserNotFound, fiber.StatusNotFound, "FRIEND_USER_NOT_FOUND", "Không tìm thấy người dùng"},
	{service.ErrFriendRequestNotFound, fiber.StatusNotFound, "FRIEND_REQUEST_NOT_FOUND", "Không tìm thấy lời mời kết bạn"},
	{service.ErrFriendNotFound, fiber.StatusNotFound, "FRIEND_NOT_FOUND", "Hai bạn chưa là bạn bè"},
	{service.ErrFriendAlreadyFriends, fiber.StatusConflict, "FRIEND_ALREADY_FRIENDS", "Hai bạn đã là bạn bè"},
	{service.ErrFriendRequestExists, fiber.StatusConflict, "FRIEND_REQUEST_EXISTS", "Bạn đã gửi lời mời cho người này, đang chờ phản hồi"},
	{service.ErrFriendRequestNotPending, fiber.StatusConflict, "FRIEND_REQUEST_NOT_PENDING", "Lời mời này không còn chờ phản hồi"},
	{service.ErrFriendRequestCooldown, fiber.StatusConflict, "FRIEND_REQUEST_COOLDOWN", "Chưa thể gửi lời mời cho người này lúc này, vui lòng thử lại sau"},
	{service.ErrFriendLimitReached, fiber.StatusConflict, "FRIEND_LIMIT_REACHED", "Một trong hai bạn đã đạt số lượng bạn bè tối đa"},
	{service.ErrFriendDailyLimit, fiber.StatusTooManyRequests, "FRIEND_DAILY_LIMIT_REACHED", "Bạn đã gửi quá nhiều lời mời trong 24 giờ qua"},
	{service.ErrFriendPendingLimit, fiber.StatusTooManyRequests, "FRIEND_PENDING_LIMIT_REACHED", "Bạn đang có quá nhiều lời mời chờ phản hồi"},
	{service.ErrFriendQueryTooShort, fiber.StatusBadRequest, "ERR_VALIDATION", "Từ khoá tìm kiếm cần ít nhất 3 ký tự"},
	{service.ErrFriendInvalidDirection, fiber.StatusBadRequest, "ERR_VALIDATION", "direction chỉ nhận incoming hoặc outgoing"},
}

func (h *FriendshipHandler) fail(c *fiber.Ctx, err error) error {
	for _, m := range friendErrors {
		if errors.Is(err, m.err) {
			body := fiber.Map{"message": m.msg, "code": m.code}
			// Cooldown: báo số giây còn phải chờ để web hiển thị đếm ngược (làm tròn lên).
			var cd *service.FriendCooldownError
			if errors.As(err, &cd) {
				body["retry_after"] = int64(math.Ceil(cd.RetryAfter.Seconds()))
			}
			return c.Status(m.status).JSON(body)
		}
	}
	// Lỗi hạ tầng: ghi log, không trả chi tiết nội bộ ra client.
	log.Printf("friendship: lỗi không xác định %s %s: %v", c.Method(), c.Path(), err)
	return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
		"message": "Lỗi hệ thống, vui lòng thử lại sau", "code": "ERR_INTERNAL",
	})
}

func badID(c *fiber.Ctx) error {
	return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"message": "ID không hợp lệ", "code": "ERR_INVALID_ID"})
}

// paging đọc page/limit; giá trị sai để service chuẩn hoá về mặc định.
func paging(c *fiber.Ctx) (int, int) {
	page, _ := strconv.Atoi(c.Query("page", "1"))
	limit, _ := strconv.Atoi(c.Query("limit", "20"))
	return page, limit
}

func ok(c *fiber.Ctx, msg string, data any) error {
	return c.JSON(fiber.Map{"message": msg, "data": data})
}

// parseTarget đọc {user_id}. Trả (id, "") khi hợp lệ; ngược lại trả mã lỗi để caller `return badBody(c, code)`:
// body hỏng → ERR_INVALID_BODY, user_id thiếu/không phải UUID → ERR_INVALID_ID. (Không ghi phản hồi ở đây:
// c.JSON trả nil khi thành công nên gộp "ghi phản hồi" vào giá trị error của hàm này khiến caller đi tiếp
// với uuid.Nil.)
func parseTarget(c *fiber.Ctx) (uuid.UUID, string) {
	var req dto.FriendTargetRequest
	if err := c.BodyParser(&req); err != nil {
		return uuid.Nil, "ERR_INVALID_BODY"
	}
	id, err := uuid.Parse(req.UserID)
	if err != nil {
		return uuid.Nil, "ERR_INVALID_ID"
	}
	return id, ""
}

func badBody(c *fiber.Ctx, code string) error {
	if code == "ERR_INVALID_ID" {
		return badID(c)
	}
	return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"message": "Dữ liệu gửi lên không hợp lệ", "code": code})
}

// GET /api/friends
func (h *FriendshipHandler) ListFriends(c *fiber.Ctx) error {
	me, err := parseUserID(c)
	if err != nil {
		return err
	}
	page, limit := paging(c)
	res, err := h.svc.ListFriends(c.Context(), me, c.Query("keyword"), page, limit)
	if err != nil {
		return h.fail(c, err)
	}
	return ok(c, "Friends retrieved successfully", res)
}

// GET /api/friends/summary
func (h *FriendshipHandler) Summary(c *fiber.Ctx) error {
	me, err := parseUserID(c)
	if err != nil {
		return err
	}
	res, err := h.svc.Summary(c.Context(), me)
	if err != nil {
		return h.fail(c, err)
	}
	return ok(c, "Friend summary retrieved successfully", res)
}

// GET /api/friends/requests
func (h *FriendshipHandler) ListRequests(c *fiber.Ctx) error {
	me, err := parseUserID(c)
	if err != nil {
		return err
	}
	page, limit := paging(c)
	res, err := h.svc.ListRequests(c.Context(), me, c.Query("direction"), page, limit)
	if err != nil {
		return h.fail(c, err)
	}
	return ok(c, "Friend requests retrieved successfully", res)
}

// POST /api/friends/requests — 201 khi tạo lời mời mới, 200 khi tự chấp nhận (đối phương đã gửi trước).
func (h *FriendshipHandler) SendRequest(c *fiber.Ctx) error {
	me, err := parseUserID(c)
	if err != nil {
		return err
	}
	target, bad := parseTarget(c)
	if bad != "" {
		return badBody(c, bad)
	}
	out, err := h.svc.SendRequest(c.Context(), me, target)
	if err != nil {
		return h.fail(c, err)
	}
	if out.AutoAccepted {
		return ok(c, "Friend request accepted", out.Result)
	}
	return c.Status(fiber.StatusCreated).JSON(fiber.Map{"message": "Friend request sent", "data": out.Result})
}

// POST /api/friends/requests/:id/accept
func (h *FriendshipHandler) AcceptRequest(c *fiber.Ctx) error {
	me, err := parseUserID(c)
	if err != nil {
		return err
	}
	id, perr := uuid.Parse(c.Params("id"))
	if perr != nil {
		return badID(c)
	}
	res, err := h.svc.AcceptRequest(c.Context(), me, id)
	if err != nil {
		return h.fail(c, err)
	}
	return ok(c, "Friend request accepted", res)
}

// POST /api/friends/requests/:id/decline
func (h *FriendshipHandler) DeclineRequest(c *fiber.Ctx) error {
	me, err := parseUserID(c)
	if err != nil {
		return err
	}
	id, perr := uuid.Parse(c.Params("id"))
	if perr != nil {
		return badID(c)
	}
	res, err := h.svc.DeclineRequest(c.Context(), me, id)
	if err != nil {
		return h.fail(c, err)
	}
	return ok(c, "Friend request declined", res)
}

// DELETE /api/friends/requests/:id
func (h *FriendshipHandler) CancelRequest(c *fiber.Ctx) error {
	me, err := parseUserID(c)
	if err != nil {
		return err
	}
	id, perr := uuid.Parse(c.Params("id"))
	if perr != nil {
		return badID(c)
	}
	if err := h.svc.CancelRequest(c.Context(), me, id); err != nil {
		return h.fail(c, err)
	}
	return ok(c, "Friend request cancelled", nil)
}

// DELETE /api/friends/:userId
func (h *FriendshipHandler) Unfriend(c *fiber.Ctx) error {
	me, err := parseUserID(c)
	if err != nil {
		return err
	}
	other, perr := uuid.Parse(c.Params("userId"))
	if perr != nil {
		return badID(c)
	}
	if err := h.svc.Unfriend(c.Context(), me, other); err != nil {
		return h.fail(c, err)
	}
	return ok(c, "Friend removed", nil)
}

// GET /api/friends/search
func (h *FriendshipHandler) Search(c *fiber.Ctx) error {
	me, err := parseUserID(c)
	if err != nil {
		return err
	}
	limit, _ := strconv.Atoi(c.Query("limit", "0"))
	res, err := h.svc.Search(c.Context(), me, c.Query("q"), limit)
	if err != nil {
		return h.fail(c, err)
	}
	return ok(c, "Search completed successfully", res)
}

// GET /api/friends/relationship/:userId
func (h *FriendshipHandler) Relationship(c *fiber.Ctx) error {
	me, err := parseUserID(c)
	if err != nil {
		return err
	}
	other, perr := uuid.Parse(c.Params("userId"))
	if perr != nil {
		return badID(c)
	}
	res, err := h.svc.Relationship(c.Context(), me, other)
	if err != nil {
		return h.fail(c, err)
	}
	return ok(c, "Relationship retrieved successfully", res)
}

// GET /api/friends/blocks
func (h *FriendshipHandler) ListBlocks(c *fiber.Ctx) error {
	me, err := parseUserID(c)
	if err != nil {
		return err
	}
	page, limit := paging(c)
	res, err := h.svc.ListBlocks(c.Context(), me, page, limit)
	if err != nil {
		return h.fail(c, err)
	}
	return ok(c, "Blocks retrieved successfully", res)
}

// POST /api/friends/blocks
func (h *FriendshipHandler) Block(c *fiber.Ctx) error {
	me, err := parseUserID(c)
	if err != nil {
		return err
	}
	target, bad := parseTarget(c)
	if bad != "" {
		return badBody(c, bad)
	}
	if err := h.svc.Block(c.Context(), me, target); err != nil {
		return h.fail(c, err)
	}
	return c.Status(fiber.StatusCreated).JSON(fiber.Map{"message": "User blocked", "data": nil})
}

// DELETE /api/friends/blocks/:userId
func (h *FriendshipHandler) Unblock(c *fiber.Ctx) error {
	me, err := parseUserID(c)
	if err != nil {
		return err
	}
	target, perr := uuid.Parse(c.Params("userId"))
	if perr != nil {
		return badID(c)
	}
	if err := h.svc.Unblock(c.Context(), me, target); err != nil {
		return h.fail(c, err)
	}
	return ok(c, "User unblocked", nil)
}
