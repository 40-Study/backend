package app

import (
	"log"
	"net/http"
	"time"

	"study.com/v1/internal/config"
	rabbitmq_queue "study.com/v1/internal/queue/rabbitmq"
	"study.com/v1/internal/service"
	"study.com/v1/internal/socket"
	"study.com/v1/internal/thirdparty/oauth"
)

type Services struct {
	// ===== Auth & Role =====
	Auth                 *service.AuthService
	Role                 *service.RoleService
	SystemRole           *service.SystemRoleService
	UserSystemRole       *service.UserSystemRoleService
	UserOrganizationRole *service.UserOrganizationRoleService
	Permission           *service.PermissionService

	// ===== Organization & Profile =====
	Organization *service.OrganizationService
	Profile      *service.ProfileService

	// ===== Teacher =====
	Teacher        service.TeacherServiceInterface
	TeacherProfile *service.TeacherProfileService

	// ===== Class =====
	Class              *service.ClassService
	ClassLessonContent *service.ClassLessonContentService
	Attendance         *service.AttendanceService

	// ===== Course Management =====
	Category      *service.CategoryService
	Tag           *service.TagService
	Cart          *service.CartService
	CourseService *service.CourseService
	Section       *service.SectionService
	Lesson        *service.LessonService
	LessonContent *service.LessonContentService
	Enrollment    *service.EnrollmentService

	// ===== Upload & Video =====
	Upload          *service.UploadService
	VideoUpload     *service.VideoUploadService
	VideoProcessing *service.VideoProcessingService

	// ===== LiveKit =====
	Livekit *service.LivekitService

	// ===== Livestream Learning Platform =====
	Livestream *service.LivestreamService
	Assignment *service.AssignmentService
	Submission *service.SubmissionService
	Chat       *service.ChatService
	Whiteboard *service.WhiteboardService
	Analytics  *service.AnalyticsService

	// ===== Order & Payment =====
	Order              *service.OrderService
	Payment            *service.PaymentService
	TransactionService *service.TransactionService
	Voucher            *service.VoucherService

	// ===== Gamification =====
	Achievement *service.AchievementService
	Leaderboard *service.LeaderboardService
	UserStats   *service.UserStatsService
	// ===== Wallet =====
	Wallet *service.WalletService

	// ===== OAuth =====
	OAuth *service.OAuthService

	// ===== Parent Invitation =====
	ParentInvitation *service.ParentInvitationService
	// ===== Discussion Forum =====
	Discussion *service.DiscussionService

	// ===== Notification =====
	Notification *service.NotificationService

	// ===== User Preference =====
	UserPreference *service.UserPreferenceService
}

func InitServices(resources *Resources, repos *Repositories, notifier *socket.Notifier) *Services {
	transactionSvc := initTransactionService(resources.Config)

	var videoQueue *rabbitmq_queue.VideoQueueSetup
	if resources.RabbitMQ != nil {
		videoQueue = rabbitmq_queue.NewVideoQueueSetup(resources.RabbitMQ)
		if err := videoQueue.SetupVideoQueues(); err != nil {
			log.Printf("Warning: Failed to setup video queues: %v", err)
			videoQueue = nil
		}
	}

	uploadSvc := service.NewVideoUploadService(
		repos.VideoUpload,
		resources.MinioWrapper,
		resources.RabbitMQ,
		videoQueue,
		resources.Redis,
	)

	var videoProcessingSvc *service.VideoProcessingService
	if resources.RabbitMQ != nil && resources.MinioWrapper != nil {
		var err error
		videoProcessingSvc, err = service.NewVideoProcessingService(
			repos.VideoUpload,
			resources.MinioWrapper,
			uploadSvc,
			resources.RabbitMQ,
		)
		if err != nil {
			log.Printf("Warning: Failed to create video processing service: %v", err)
		}
	}
	httpClient := &http.Client{Timeout: 10 * time.Second}

	// ================= Initialize LiveKit Service =================
	livekitSvc := service.NewLivekitService(resources.Config)

	// ================= Initialize Livestream Learning Platform Services =================
	livestreamSvc := service.NewLivestreamService(
		repos.Livestream,
		repos.Participant,
		repos.Analytics,
		resources.Redis,
		livekitSvc,
		resources.Queue,
		resources.Config,
	)

	assignmentSvc := service.NewAssignmentService(
		repos.Assignment,
		repos.TestCase,
		repos.Submission,
	)

	submissionSvc := service.NewSubmissionService(
		repos.Submission,
		assignmentSvc,
		repos.TestCase,
		resources.Redis,
		resources.Config,
	)

	chatSvc := service.NewChatService(
		repos.ChatMessage,
		repos.Analytics,
		repos.Livestream,
		livekitSvc,
	)

	whiteboardSvc := service.NewWhiteboardService(
		repos.Whiteboard,
		repos.Livestream,
		resources.Redis,
	)

	analyticsSvc := service.NewAnalyticsService(
		repos.Analytics,
		repos.Participant,
		repos.Submission,
		repos.Assignment,
	)

	classSvc := service.NewClassService(repos.Class, repos.Course, repos.Teacher, repos.Student, repos.ParentStudent)
	teacherSvc := service.NewTeacherService(repos.Teacher, classSvc)

	// ================= Auth (created early because OAuth depends on it) =================
	authSvc := service.NewAuthService(
		resources.Config,
		repos.User,
		repos.Role,
		repos.UserOrganizationRole,
		repos.UserSystemRole,
		repos.SystemRole,
		repos.UserSystemRole, // userRoleRepo - dùng cho OAuth flow (tạo role sau khi chọn)
		resources.Redis,
	)

	// ================= OAuth Providers =================
	// Chỉ đăng ký provider nào có config (client ID không rỗng)
	// Thêm provider mới: chỉ cần tạo file trong thirdparty/oauth/ và đăng ký ở đây
	oauthProviders := make(map[string]oauth.OAuthProvider)
	if resources.Config.GitHub.ClientID != "" {
		oauthProviders["github"] = oauth.NewGitHubProvider(&resources.Config.GitHub, httpClient)
	}
	if resources.Config.Google.ClientID != "" {
		oauthProviders["google"] = oauth.NewGoogleProvider(&oauth.GoogleOAuthConfig{
			ClientID:     resources.Config.Google.ClientID,
			ClientSecret: resources.Config.Google.ClientSecret,
			RedirectURL:  resources.Config.Google.RedirectURL,
		}, httpClient)
	}
	if resources.Config.Facebook.ClientID != "" {
		oauthProviders["facebook"] = oauth.NewFacebookProvider(&oauth.FacebookOAuthConfig{
			ClientID:     resources.Config.Facebook.ClientID,
			ClientSecret: resources.Config.Facebook.ClientSecret,
			RedirectURL:  resources.Config.Facebook.RedirectURL,
		}, httpClient)
	}

	// ================= Parent Invitation Service =================
	parentInvitationSvc := service.NewParentInvitationService(
		resources.Config,
		resources.Redis,
		repos.ParentInvitation,
		repos.ParentStudent,
		repos.User,
	)

	// Inject vào AuthService và OAuthService để gọi LinkInvitationToNewUser khi register
	authSvc.SetParentInvitationService(parentInvitationSvc)

	oauthSvc := service.NewOAuthService(
		resources.Config,
		resources.Redis,
		repos.User,
		repos.OAuthProvider,
		repos.UserSystemRole,
		repos.SystemRole,
		authSvc,
		oauthProviders,
	)
	oauthSvc.SetParentInvitationService(parentInvitationSvc)

	// ================= Return Services =================
	return &Services{
		// ===== Auth =====
		Auth:  authSvc,
		OAuth: oauthSvc,

		// ===== Parent Invitation =====
		ParentInvitation: parentInvitationSvc,

		// ===== Role =====
		Role:       service.NewRoleService(repos.Role, repos.Permission),
		SystemRole: service.NewSystemRoleService(repos.SystemRole, repos.Permission),

		UserSystemRole: service.NewUserSystemRoleService(
			repos.UserSystemRole,
			repos.User,
			repos.SystemRole,
		),

		UserOrganizationRole: service.NewUserOrganizationRoleService(
			repos.UserOrganizationRole,
			repos.User,
			repos.Role,
			repos.Organization,
			resources.Redis,
		),

		Permission: service.NewPermissionService(repos.Permission),

		// ===== Organization & Profile =====
		Organization: service.NewOrganizationService(repos.Organization),

		Profile: service.NewProfileService(
			repos.ParentStudent,
			repos.UserOrganizationRole,
		),

		// ===== Class =====
		Class:              service.NewClassService(repos.Class, repos.Course, repos.Teacher, repos.Student, repos.ParentStudent),
		ClassLessonContent: service.NewClassLessonContentService(repos.ClassLessonContent, repos.Class, repos.Lesson, repos.Enrollment, livestreamSvc),
		Attendance:         service.NewAttendanceService(repos.Attendance),

		// ===== Teacher =====
		Teacher:        teacherSvc,
		TeacherProfile: service.NewTeacherProfileService(repos.TeacherProfile),

		// ===== Course Management =====
		Category:      service.NewCategoryService(repos.Category),
		Tag:           service.NewTagService(repos.Tag),
		Cart:          service.NewCartService(repos.CartItem, repos.Course, repos.Enrollment),
		CourseService: service.NewCourseService(repos.Course, repos.Category, repos.Tag),
		Section:       service.NewSectionService(repos.Section, repos.Course),
		Lesson:        service.NewLessonService(repos.Lesson, repos.Section, service.NewUploadService(resources.MinioClient, resources.Config)),
		LessonContent: service.NewLessonContentService(repos.Lesson, service.NewUploadService(resources.MinioClient, resources.Config)),
		Enrollment:    service.NewEnrollmentService(repos.Enrollment, repos.Course, repos.Lesson),

		// ===== Upload & Video =====
		Upload:          service.NewUploadService(resources.MinioClient, resources.Config),
		VideoUpload:     uploadSvc,
		VideoProcessing: videoProcessingSvc,

		// ===== LiveKit =====
		Livekit: livekitSvc,

		// ===== Livestream Learning Platform =====
		Livestream: livestreamSvc,
		Assignment: assignmentSvc,
		Submission: submissionSvc,
		Chat:       chatSvc,
		Whiteboard: whiteboardSvc,
		Analytics:  analyticsSvc,

		// ===== Order & Payment =====
		Order: service.NewOrderService(
			repos.Order,
			repos.OrderItem,
			repos.Coupon,
			repos.Course,
			repos.Enrollment,

			repos.CartItem,
			repos.OrderStatusHistory,
			repos.IdempotencyKey,
		),
		Payment: service.NewPaymentService(
			repos.Order,
			repos.OrderItem,
			repos.PaymentEvent,
			repos.OrderStatusHistory,
			repos.Enrollment,
			repos.Coupon,
			transactionSvc,
		),
		TransactionService: transactionSvc,
		Voucher:            service.NewVoucherService(repos.Voucher, repos.User),

		// ===== Gamification =====
		Achievement: service.NewAchievementService(repos.Achievement),
		Leaderboard: service.NewLeaderboardService(repos.Leaderboard),
		UserStats:   service.NewUserStatsService(repos.UserStats),
		// ===== Wallet =====
		Wallet: service.NewWalletService(repos.Wallet, repos.TeacherProfile),

		// ===== Discussion Forum =====
		Discussion: service.NewDiscussionService(repos.Discussion),

		// ===== Notification =====
		Notification: service.NewNotificationService(repos.Notification, notifier),

		// ===== User Preference =====
		UserPreference: service.NewUserPreferenceService(repos.UserPreference),
	}
}

// initTransactionService creates the transaction gRPC service
func initTransactionService(cfg *config.Config) *service.TransactionService {
	transactionSvc, err := service.NewTransactionService(
		cfg.TransactionServiceHost,
		cfg.TransactionServicePort,
	)
	if err != nil {
		log.Printf("Warning: Failed to initialize transaction service: %v", err)
		return nil
	}

	log.Println("Transaction service (gRPC) initialized successfully")
	return transactionSvc
}
