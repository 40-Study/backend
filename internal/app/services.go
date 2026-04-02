package app

import (
	"log"

	"study.com/v1/internal/config"
	"study.com/v1/internal/queue"
	"study.com/v1/internal/service"
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
	Teacher        *service.TeacherService
	TeacherProfile *service.TeacherProfileService

	// ===== Class =====
	Class         *service.ClassService
	ClassSchedule *service.ClassScheduleService
	Attendance    *service.AttendanceService

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
}

func InitServices(resources *Resources, repos *Repositories) *Services {
	// Initialize transaction service (gRPC)
	transactionSvc := initTransactionService(resources.Config)

	// ================= Video Queue Setup =================
	var videoQueue *queue.VideoQueueSetup
	if resources.RabbitMQ != nil {
		videoQueue = queue.NewVideoQueueSetup(resources.RabbitMQ)
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

	// ================= Initialize LiveKit Service =================
	livekitSvc := service.NewLivekitService(resources.Config)

	// ================= Initialize Livestream Learning Platform Services =================
	livestreamSvc := service.NewLivestreamService(
		repos.Livestream,
		repos.Participant,
		repos.Analytics,
		resources.Redis,
		livekitSvc,
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

	// ================= Return Services =================
	return &Services{
		// ===== Auth =====
		Auth: service.NewAuthService(
			resources.Config,
			repos.User,
			repos.Role,
			repos.UserOrganizationRole,
			repos.UserSystemRole,
			repos.SystemRole,
			resources.Redis,
		),

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
		),

		Permission: service.NewPermissionService(repos.Permission),

		// ===== Organization & Profile =====
		Organization: service.NewOrganizationService(repos.Organization),

		Profile: service.NewProfileService(
			repos.ParentStudent,
			repos.UserOrganizationRole,
		),

		// ===== Class =====
		Class:         classSvc,
		ClassSchedule: service.NewClassScheduleService(repos.ClassSchedule, repos.Class),
		Attendance:    service.NewAttendanceService(repos.Attendance),

		// ===== Teacher =====
		Teacher:        teacherSvc,
		TeacherProfile: service.NewTeacherProfileService(repos.TeacherProfile),

		// ===== Course Management =====
		Category:      service.NewCategoryService(repos.Category),
		Tag:           service.NewTagService(repos.Tag),
		Cart:          service.NewCartService(repos.CartItem, repos.Course, repos.Enrollment),
		CourseService: service.NewCourseService(repos.Course, repos.Category, repos.Tag),
		Section:       service.NewSectionService(repos.Section, repos.Course),
		Lesson:        service.NewLessonService(repos.Lesson, repos.Section, repos.Course),
		LessonContent: service.NewLessonContentService(repos.LessonContent, repos.Lesson),
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
