package app

import (
	"github.com/gofiber/fiber/v2"
	"study.com/v1/internal/handler"
	"study.com/v1/internal/middleware"
	"study.com/v1/internal/repository"
	"study.com/v1/internal/router"
	"study.com/v1/internal/service"
)

// registerQA2BE3Routes đăng ký các route của lane BE3 (QA vòng 2 261013) - contract:
// plans/261013-qa2-role-findings/contract-be3.md. Đăng ký riêng (như withdrawal/friends/audit) thay vì thêm
// tham số vào SetupAllRoutes, vì router dùng chung nhiều lane đang sửa song song. Mỗi dịch vụ mới tự dựng ở
// đây từ repos/DB sẵn có nên không phải sửa Repositories/Services/Handlers.
func registerQA2BE3Routes(fiberApp *fiber.App, res *Resources, repos *Repositories, services *Services, permChecker *middleware.PermissionChecker) {
	api := fiberApp.Group("/api")

	// F-10 wishlist.
	wishlistSvc := service.NewWishlistService(repository.NewWishlistRepository(res.DB), repos.Course)
	router.SetupWishlistRoutes(api, res.Config, handler.NewWishlistHandler(wishlistSvc), res.Redis)

	// ADM-N4 kiểm duyệt từ báo cáo: đính kèm `target` vào GET /reports(/:id) và thêm POST /reports/:id/actions.
	targetResolver := service.NewReportTargetResolver(repository.NewReportTargetRepository(res.DB))
	services.Report.SetTargetResolver(targetResolver)
	moderationSvc := service.NewReportModerationService(repos.Report, repository.NewReportModerationRepository(res.DB), targetResolver, services.Notification)
	router.SetupReportModerationRoutes(api, res.Config, handler.NewReportModerationHandler(moderationSvc), res.Redis, permChecker, services.AuditLog)

	// PG-M2 trang giảng viên công khai.
	publicTeacherSvc := service.NewPublicTeacherProfileService(repository.NewPublicTeacherProfileRepository(res.DB))
	router.SetupPublicTeacherProfileRoutes(api, handler.NewPublicTeacherProfileHandler(publicTeacherSvc))
}
