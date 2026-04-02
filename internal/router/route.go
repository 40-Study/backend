package router

import (
	"github.com/gofiber/fiber/v2"
	"github.com/minio/minio-go/v7"
	"github.com/redis/go-redis/v9"
	"study.com/v1/internal/config"
	"study.com/v1/internal/handler"
)

func SetupAllRoutes(
	app *fiber.App,
	cfg *config.Config,
	authHandler *handler.AuthHandler,
	roleHandler *handler.RoleHandler,
	systemRoleHandler *handler.SystemRoleHandler,
	userSystemRoleHandler *handler.UserSystemRoleHandler,
	userOrgRoleHandler *handler.UserOrganizationRoleHandler,
	permissionHandler *handler.PermissionHandler,
	organizationHandler *handler.OrganizationHandler,
	profileHandler *handler.ProfileHandler,
	teacherHandler *handler.TeacherHandler,
	teacherProfileHandler *handler.TeacherProfileHandler,
	classHandler *handler.ClassHandler,
	classScheduleHandler *handler.ClassScheduleHandler,
	attendanceHandler *handler.AttendanceHandler,
	categoryHandler *handler.CategoryHandler,
	tagHandler *handler.TagHandler,
	cartHandler *handler.CartHandler,
	courseHandler *handler.CourseHandler,
	sectionHandler *handler.SectionHandler,
	lessonHandler *handler.LessonHandler,
	lessonContentHandler *handler.LessonContentHandler,
	enrollmentHandler *handler.EnrollmentHandler,
	uploadHandler *handler.UploadHandler,
	videoHandler *handler.VideoUploadHandler,
	hlsHandler *handler.HLSHandler,
	livestreamHandler *handler.LivestreamHandler,
	assignmentHandler *handler.AssignmentHandler,
	submissionHandler *handler.SubmissionHandler,
	chatHandler *handler.ChatHandler,
	whiteboardHandler *handler.WhiteboardHandler,
	analyticsHandler *handler.AnalyticsHandler,
	orderHandler *handler.OrderHandler,
	voucherHandler *handler.VoucherHandler,
	achievementHandler *handler.AchievementHandler,
	leaderboardHandler *handler.LeaderboardHandler,
	userStatsHandler *handler.UserStatsHandler,
	redis *redis.Client,
	minio *minio.Client,
) {
	api := app.Group("/api")

	// Health check
	api.Get("/health", func(c *fiber.Ctx) error {
		return c.JSON(fiber.Map{
			"status":  "ok",
			"message": "Server is running",
		})
	})

	SetupAuthRoutes(api, cfg, authHandler, redis)
	SetupOrgRoleRoutes(api, cfg, roleHandler, redis)
	SetupSystemRoleRoutes(api, cfg, systemRoleHandler, redis)
	SetupUserSystemRoleRoutes(api, cfg, userSystemRoleHandler, redis)
	SetupUserOrganizationRoleRoutes(api, cfg, userOrgRoleHandler, redis)
	SetupPermissionRoutes(api, cfg, permissionHandler, redis)
	SetupOrganizationRoutes(api, organizationHandler)
	SetupProfileRoutes(api, cfg, profileHandler, redis)
	SetupTeacherRoutes(api, cfg, teacherHandler, redis)
	SetupTeacherProfileRoutes(api, teacherProfileHandler)
	SetupClassRoutes(api, cfg, classHandler, classScheduleHandler, attendanceHandler, redis)
	SetupCategoryRoutes(api, cfg, categoryHandler, tagHandler, redis)
	SetupCartRoutes(api, cfg, cartHandler, redis)
	SetupCourseRoutes(api, cfg, courseHandler, sectionHandler, lessonHandler, redis)
	SetupLessonContentRoutes(api, cfg, lessonContentHandler, redis)
	SetupEnrollmentRoutes(api, cfg, enrollmentHandler, redis)
	SetupUploadRoutes(api, cfg, uploadHandler, redis)
	SetupVideoUploadRoutes(api, videoHandler, cfg, redis)
	SetupHLSRoutes(api, hlsHandler)
	// New livestream learning platform routes
	SetupLivestreamRoutes(api, cfg, livestreamHandler, redis)
	SetupAssignmentRoutes(api, cfg, assignmentHandler, redis)
	SetupSubmissionRoutes(api, cfg, submissionHandler, redis)
	SetupChatRoutes(api, cfg, chatHandler, redis)
	SetupWhiteboardRoutes(api, cfg, whiteboardHandler, redis)
	SetupAnalyticsRoutes(api, cfg, analyticsHandler, redis)

	// Order & Payment routes
	SetupOrderRoutes(api, cfg, orderHandler, redis)
	SetupVoucherRoutes(api, voucherHandler)

	// Gamification routes
	SetupAchievementRoutes(api, cfg, achievementHandler, redis)
	SetupLeaderboardRoutes(api, cfg, leaderboardHandler, redis)
	SetupUserStatsRoutes(api, userStatsHandler)
}
