package app

import (
	"study.com/v1/internal/config"
	"study.com/v1/internal/handler"
	"study.com/v1/internal/storage"
)

type Handlers struct {
	// ===== Auth & Role =====
	Auth                 *handler.AuthHandler
	Role                 *handler.RoleHandler
	SystemRole           *handler.SystemRoleHandler
	UserSystemRole       *handler.UserSystemRoleHandler
	UserOrganizationRole *handler.UserOrganizationRoleHandler
	Permission           *handler.PermissionHandler

	// ===== Organization & Profile =====
	Organization *handler.OrganizationHandler
	Profile      *handler.ProfileHandler

	// ===== Teacher =====
	Teacher        *handler.TeacherHandler
	TeacherProfile *handler.TeacherProfileHandler

	// ===== Class =====
	Class              *handler.ClassHandler
	ClassLessonContent *handler.ClassLessonContentHandler
	Attendance         *handler.AttendanceHandler

	// ===== Course Management =====
	Category      *handler.CategoryHandler
	Tag           *handler.TagHandler
	Cart          *handler.CartHandler
	CourseHandler *handler.CourseHandler
	Section       *handler.SectionHandler
	Lesson        *handler.LessonHandler
	LessonContent *handler.LessonContentHandler
	Enrollment    *handler.EnrollmentHandler

	// ===== Upload & Video =====
	Upload      *handler.UploadHandler
	VideoUpload *handler.VideoUploadHandler
	HLS         *handler.HLSHandler

	// ===== Livestream Learning Platform =====
	Livestream *handler.LivestreamHandler
	Assignment *handler.AssignmentHandler
	Submission *handler.SubmissionHandler
	Chat       *handler.ChatHandler
	Whiteboard *handler.WhiteboardHandler
	Analytics  *handler.AnalyticsHandler

	// ===== Order & Payment =====
	Order   *handler.OrderHandler
	Voucher *handler.VoucherHandler

	// ===== Gamification =====
	Achievement *handler.AchievementHandler
	Leaderboard *handler.LeaderboardHandler
	UserStats   *handler.UserStatsHandler
	// ===== Wallet =====
	Wallet *handler.WalletHandler

	// ===== OAuth =====
	OAuth *handler.OAuthHandler

	// ===== Discussion Forum =====
	Discussion *handler.DiscussionHandler
}

func InitHandlers(services *Services, minioClient *storage.MinioClient, cfg *config.Config) *Handlers {
	return &Handlers{
		// ===== Auth & Role =====
		Auth:                 handler.NewAuthHandler(services.Auth),
		Role:                 handler.NewRoleHandler(services.Role),
		SystemRole:           handler.NewSystemRoleHandler(services.SystemRole),
		UserSystemRole:       handler.NewUserSystemRoleHandler(services.UserSystemRole),
		UserOrganizationRole: handler.NewUserOrganizationRoleHandler(services.UserOrganizationRole),
		Permission:           handler.NewPermissionHandler(services.Permission),

		// ===== Organization & Profile =====
		Organization: handler.NewOrganizationHandler(services.Organization),
		Profile:      handler.NewProfileHandler(services.Profile, services.UserOrganizationRole),

		// ===== Teacher =====
		Teacher:        handler.NewTeacherHandler(services.Teacher),
		TeacherProfile: handler.NewTeacherProfileHandler(services.TeacherProfile),

		// ===== Class =====
		Class:              handler.NewClassHandler(services.Class),
		ClassLessonContent: handler.NewClassLessonContentHandler(services.ClassLessonContent),
		Attendance:         handler.NewAttendanceHandler(services.Attendance),

		// ===== Course Management =====
		Category:      handler.NewCategoryHandler(services.Category),
		Tag:           handler.NewTagHandler(services.Tag),
		Cart:          handler.NewCartHandler(services.Cart),
		CourseHandler: handler.NewCourseHandler(services.CourseService),
		Section:       handler.NewSectionHandler(services.Section),
		Lesson:        handler.NewLessonHandler(services.Lesson),
		LessonContent: handler.NewLessonContentHandler(services.LessonContent),
		Enrollment:    handler.NewEnrollmentHandler(services.Enrollment),

		// ===== Upload & Video =====
		Upload:      handler.NewUploadHandler(services.Upload),
		VideoUpload: handler.NewVideoUploadHandler(services.VideoUpload),
		HLS:         handler.NewHLSHandler(minioClient),

		// ===== Livestream Learning Platform =====
		Livestream: handler.NewLivestreamHandler(services.Livestream),
		Assignment: handler.NewAssignmentHandler(services.Assignment, services.Livekit),
		Submission: handler.NewSubmissionHandler(services.Submission),
		Chat:       handler.NewChatHandler(services.Chat),
		Whiteboard: handler.NewWhiteboardHandler(services.Whiteboard, services.Livekit),
		Analytics:  handler.NewAnalyticsHandler(services.Analytics),

		// ===== Order & Payment =====
		Order:   handler.NewOrderHandler(services.Order, services.Payment),
		Voucher: handler.NewVoucherHandler(services.Voucher),

		// ===== Gamification =====
		Achievement: handler.NewAchievementHandler(services.Achievement),
		Leaderboard: handler.NewLeaderboardHandler(services.Leaderboard),
		UserStats:   handler.NewUserStatsHandler(services.UserStats),
		// ===== Wallet =====
		Wallet: handler.NewWalletHandler(services.Wallet),

		// ===== OAuth =====
		OAuth: handler.NewOAuthHandler(services.OAuth, cfg),

		// ===== Discussion Forum =====
		Discussion: handler.NewDiscussionHandler(services.Discussion),
	}
}
