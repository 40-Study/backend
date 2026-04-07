package router

import (
	"github.com/gofiber/fiber/v2"
	"github.com/minio/minio-go/v7"
	"github.com/redis/go-redis/v9"
	"study.com/v1/internal/config"
	"study.com/v1/internal/handler"
	asynq_queue "study.com/v1/internal/queue/asynq"
)

func SetupAllRoutes(
	app *fiber.App,
	cfg *config.Config,
	authHandler *handler.AuthHandler,
	oauthHandler *handler.OAuthHandler,
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
	classLessonContentHandler *handler.ClassLessonContentHandler,
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
	walletHandler *handler.WalletHandler,
	parentInvitationHandler *handler.ParentInvitationHandler,
	discussionHandler *handler.DiscussionHandler,
	notificationHandler *handler.NotificationHandler,
	userPreferenceHandler *handler.UserPreferenceHandler,
	redis *redis.Client,
	minio *minio.Client,
	aq *asynq_queue.Queue,
) {
	api := app.Group("/api")

	// Health check
	api.Get("/health", func(c *fiber.Ctx) error {
		return c.JSON(fiber.Map{
			"status":  "ok",
			"message": "Server is running",
		})
	})

	SetupAuthRoutes(api, cfg, authHandler, oauthHandler, redis)
	SetupOrgRoleRoutes(api, cfg, roleHandler, redis)
	SetupSystemRoleRoutes(api, cfg, systemRoleHandler, redis)
	SetupUserSystemRoleRoutes(api, cfg, userSystemRoleHandler, redis)
	SetupUserOrganizationRoleRoutes(api, cfg, userOrgRoleHandler, redis)
	SetupPermissionRoutes(api, cfg, permissionHandler, redis)
	SetupOrganizationRoutes(api, organizationHandler)
	SetupProfileRoutes(api, cfg, profileHandler, redis)
	SetupTeacherRoutes(api, cfg, teacherHandler, redis)
	SetupTeacherProfileRoutes(api, teacherProfileHandler)
	SetupClassRoutes(api, cfg, classHandler, attendanceHandler, redis)
	SetupCategoryRoutes(api, cfg, categoryHandler, tagHandler, redis)
	SetupCartRoutes(api, cfg, cartHandler, redis)
	SetupCourseRoutes(api, cfg, courseHandler, sectionHandler, lessonHandler, lessonContentHandler, classHandler, classLessonContentHandler, attendanceHandler, redis)
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
	// Wallet routes
	SetupWalletRoutes(api, cfg, walletHandler, redis)

	// Parent Invitation routes
	SetupParentInvitationRoutes(api, cfg, parentInvitationHandler, redis)
	// Discussion forum routes
	SetupDiscussionRoutes(api, cfg, discussionHandler, redis)

	// Notification routes
	SetupNotificationRoutes(api, cfg, notificationHandler, redis)

	// User Preference routes
	SetupUserPreferenceRoutes(api, cfg, userPreferenceHandler, redis)
}
