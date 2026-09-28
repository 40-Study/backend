package router

import (
	"github.com/gofiber/fiber/v2"
	"github.com/redis/go-redis/v9"
	"study.com/v1/internal/config"
	"study.com/v1/internal/handler"
	"study.com/v1/internal/middleware"
)

// SetupWithdrawalRoutes — Phase 4 rút tiền giảng viên (2026-09-28).
//
// Middleware gắn trực tiếp trên TỪNG route (không group.Use) để tránh lỗi H-01: Use() khớp theo
// PREFIX nên sẽ ảnh hưởng route khác đăng ký sau dưới cùng tiền tố /admin hay /wallet.
//
// Giáo viên: chỉ cần đăng nhập — đây là thao tác trên ví của chính mình, teacherID luôn lấy từ
// token, service từ chối nếu không có hồ sơ giáo viên. Admin: WALLET_WITHDRAWALS_MANAGE.
func SetupWithdrawalRoutes(
	api fiber.Router,
	cfg *config.Config,
	h *handler.WithdrawalHandler,
	redis *redis.Client,
	permChecker *middleware.PermissionChecker,
) {
	auth := middleware.AuthMiddleware(cfg, redis)
	manage := permChecker.RequirePermissions("WALLET_WITHDRAWALS_MANAGE")

	teacher := api.Group("/wallet/teacher/withdrawals")
	teacher.Post("/", auth, h.CreateWithdrawal)
	teacher.Get("/", auth, h.ListMyWithdrawals)
	// Q2 (QA vòng 2): giảng viên tự huỷ yêu cầu còn pending của chính mình.
	teacher.Post("/:id/cancel", auth, h.CancelMyWithdrawal)

	admin := api.Group("/admin/withdrawals")
	admin.Get("/", auth, manage, h.AdminListWithdrawals)
	admin.Get("/negative-balances", auth, manage, h.AdminNegativeBalances)
	admin.Post("/:id/approve", auth, manage, h.ApproveWithdrawal)
	admin.Post("/:id/reject", auth, manage, h.RejectWithdrawal)
	admin.Post("/:id/mark-completed", auth, manage, h.MarkWithdrawalCompleted)
}
