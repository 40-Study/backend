package handler

import (
	"errors"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"study.com/v1/internal/middleware"
	"study.com/v1/internal/model"
	"study.com/v1/internal/service"
)

type LeaderboardHandlerInterface interface {
	GetLeaderboard(c *fiber.Ctx) error
	GetMyRank(c *fiber.Ctx) error
	GetMyClassBoards(c *fiber.Ctx) error
}

type LeaderboardHandler struct {
	svc         service.LeaderboardServiceInterface
	permChecker *middleware.PermissionChecker
}

func NewLeaderboardHandler(svc service.LeaderboardServiceInterface, permChecker *middleware.PermissionChecker) *LeaderboardHandler {
	return &LeaderboardHandler{svc: svc, permChecker: permChecker}
}

// leaderboardPeriodFromQuery đọc kỳ xếp hạng từ query.
// Nhận cả `period` (tên cũ, giữ cho client đang dùng) và `period_type` (tên web gửi, trùng field
// period_type trong response). Có cả hai thì ưu tiên `period` để client cũ không đổi hành vi.
// Rỗng cả hai → all_time như trước. Giá trị ngoài enum (kể cả "daily" mà web có tab "Ngày")
// → lỗi 400, KHÔNG âm thầm lùi về all_time — đó chính là lỗi cũ làm mọi tab ra all_time.
func leaderboardPeriodFromQuery(c *fiber.Ctx) (string, error) {
	p := c.Query("period")
	if p == "" {
		p = c.Query("period_type")
	}
	if p == "" {
		return model.LeaderboardPeriodAllTime, nil
	}
	if !model.IsValidLeaderboardPeriodType(p) {
		return "", service.ErrInvalidLeaderboardPeriod
	}
	return p, nil
}

// GetLeaderboard GET /leaderboard?period=weekly|monthly|all_time&limit=100
// (hoặc ?period_type=...; `period` thắng nếu gửi cả hai; giá trị sai → 400)
func (h *LeaderboardHandler) GetLeaderboard(c *fiber.Ctx) error {
	periodType, err := leaderboardPeriodFromQuery(c)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}
	limit := c.QueryInt("limit", 100)

	// Route công khai nhưng theo người xem (OptionalAuth): khách không có user_id. Chính chủ và admin
	// (SYSTEM_SETTINGS_MANAGE) thấy tên thật của người đặt ẩn danh; người khác thì không.
	var viewer *service.LeaderboardViewer
	if uid, ok := c.Locals("user_id").(uuid.UUID); ok && uid != uuid.Nil {
		viewer = &service.LeaderboardViewer{UserID: uid, IsAdmin: isAdminActor(c, h.permChecker, uid)}
	}

	// B-20: class_id trước đây bị bỏ qua im lặng nên không có bảng xếp hạng theo lớp. Sai UUID là 400.
	var classID *uuid.UUID
	if raw := c.Query("class_id"); raw != "" {
		id, perr := uuid.Parse(raw)
		if perr != nil {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid class_id"})
		}
		classID = &id
	}

	resp, err := h.svc.GetLeaderboard(c.Context(), periodType, limit, viewer, classID)
	if err != nil {
		if errors.Is(err, service.ErrInvalidLeaderboardPeriod) {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
		}
		if errors.Is(err, service.ErrLeaderboardClassNotFound) {
			return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": err.Error()})
		}
		// Lỗi khác (DB/SQL...) không được lộ ra client: log phía server, trả thông điệp chung.
		return RespondServiceError(c, err, "Không thể tải bảng xếp hạng")
	}
	return c.JSON(fiber.Map{"data": resp})
}

// GetMyClassBoards GET /leaderboard/classes: các lớp người gọi mở được bảng xếp hạng riêng (?class_id= của GET /leaderboard).
func (h *LeaderboardHandler) GetMyClassBoards(c *fiber.Ctx) error {
	userID, ok := c.Locals("user_id").(uuid.UUID)
	if !ok || userID == uuid.Nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "unauthorized"})
	}
	classes, err := h.svc.ListMyClassBoards(c.Context(), userID)
	if err != nil {
		return RespondServiceError(c, err, "Không thể tải danh sách lớp")
	}
	return c.JSON(fiber.Map{"data": classes})
}

// GetMyRank GET /leaderboard/me?period=weekly|monthly|all_time
// (hoặc ?period_type=...; `period` thắng nếu gửi cả hai; giá trị sai → 400)
func (h *LeaderboardHandler) GetMyRank(c *fiber.Ctx) error {
	userID, ok := c.Locals("user_id").(uuid.UUID)
	if !ok || userID == uuid.Nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "unauthorized"})
	}

	periodType, err := leaderboardPeriodFromQuery(c)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}

	resp, err := h.svc.GetMyRank(c.Context(), userID, periodType)
	if err != nil {
		if errors.Is(err, service.ErrInvalidLeaderboardPeriod) {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
		}
		// Lỗi khác (DB/SQL...) không được lộ ra client: log phía server, trả thông điệp chung.
		return RespondServiceError(c, err, "Không thể tải thứ hạng của bạn")
	}
	return c.JSON(fiber.Map{"data": resp})
}
