package router

import (
	"github.com/gofiber/fiber/v2"
	"github.com/redis/go-redis/v9"
	"study.com/v1/internal/config"
	"study.com/v1/internal/handler"
	"study.com/v1/internal/middleware"
)

func SetupEnrollmentRoutes(
	api fiber.Router,
	cfg *config.Config,
	enrollmentHandler *handler.EnrollmentHandler,
	redis *redis.Client,
) {
	auth := middleware.AuthMiddleware(cfg, redis)

	// MED-3 (review 260915) + N4 (review vong 2, 260915): cung ALLOWED_ORIGINS voi middleware
	// cors o app.go, gio qua mot ham SSOT duy nhat (config.Config.ResolvedAllowedOrigins) — tu
	// an toan voi cfg=nil (test router-level goi ham nay voi cfg=nil, xem
	// TestProgressBeaconRoute_IsRegistered) va tu log canh bao khi ALLOWED_ORIGINS="*".
	allowedOrigins := cfg.ResolvedAllowedOrigins()

	// Enroll/Unenroll under courses
	courses := api.Group("/courses")
	{
		courses.Post("/:courseId/enroll", auth, enrollmentHandler.Enroll)
		courses.Delete("/:courseId/enroll", auth, enrollmentHandler.Unenroll)
		courses.Get("/:courseId/enrollments", auth, enrollmentHandler.GetCourseEnrollments)
		// L-04 (audit 260909): endpoint debug (lộ enrollment đã soft-delete + PII, không giới
		// hạn chỉ giảng viên khóa học) đã gỡ khỏi router — không nên tồn tại ở production.
	}

	// My enrollments
	enrollments := api.Group("/enrollments", auth)
	{
		enrollments.Get("/", enrollmentHandler.GetMyEnrollments)
		enrollments.Get("/:id", enrollmentHandler.GetEnrollmentDetail)
	}

	// Lesson progress
	lessons := api.Group("/lessons", auth)
	{
		lessons.Put("/:lessonId/progress", enrollmentHandler.UpdateLessonProgress)
	}

	// Beacon tu trinh phat video khi dong tab (navigator.sendBeacon khong the
	// dat header nen lessonId nam trong body, khong phai path). Xem
	// EnrollmentHandler.TrackProgressBeacon. Thay cho progress_router.go von bi
	// comment toan bo — khong dung mot ProgressHandler rieng de tranh nhan doi
	// logic ghi tien do.
	//
	// MED-3 (review 260915): sendBeacon dung Content-Type "text/plain" nen day la simple
	// request — khong preflight, cors.New khong chan duoc. SameOriginRequired dung truoc auth de
	// tu choi som request tu origin la, khong ton chi phi xac thuc token/Redis cho request se bi
	// tu choi.
	api.Post("/progress", middleware.SameOriginRequired(allowedOrigins), auth, enrollmentHandler.TrackProgressBeacon)
}
