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
	UserAdmin            *service.UserAdminService

	// ===== Organization & Profile =====
	Organization *service.OrganizationService
	Profile      *service.ProfileService

	// ===== Teacher =====
	Teacher        service.TeacherServiceInterface
	TeacherProfile *service.TeacherProfileService
	// Phase 3 duyệt khoá học + duyệt giáo viên
	CourseReview       *service.CourseReviewService
	TeacherApplication *service.TeacherApplicationService

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
	// AdminOrder/PlatformSetting (tính năng đơn hàng+hoàn tiền+doanh thu, quyết định 27/09/2026):
	// đơn hàng admin + hoàn tiền + báo cáo doanh thu thật + % phí nền tảng cấu hình được.
	AdminOrder      service.AdminOrderServiceInterface
	PlatformSetting service.PlatformSettingServiceInterface

	// ===== Gamification =====
	Achievement *service.AchievementService
	Leaderboard *service.LeaderboardService
	UserStats   *service.UserStatsService
	// ===== Wallet =====
	Wallet *service.WalletService
	// Phase 4: rút tiền giảng viên
	Withdrawal *service.WithdrawalService

	// ===== OAuth =====
	OAuth *service.OAuthService

	// ===== Parent Invitation =====
	ParentInvitation *service.ParentInvitationService
	// ===== Parent Dashboard =====
	ParentDashboard *service.ParentDashboardService
	// ===== Parent Link Request (QA vòng 2 lane E: phụ huynh gửi, con xác nhận) =====
	ParentLink *service.ParentLinkService
	// ===== Discussion Forum =====
	Discussion *service.DiscussionService
	// ===== Note (Phase 1 §3) =====
	Note service.NoteServiceInterface

	// ===== Notification =====
	Notification *service.NotificationService

	// ===== User Preference =====
	UserPreference *service.UserPreferenceService

	// ===== Schedule =====
	Schedule *service.ScheduleService

	// ===== Quiz =====
	Quiz *service.QuizService

	// ===== Grade =====
	Grade *service.GradeService

	// ===== Exercise =====
	Exercise *service.ExerciseService

	// ===== Review =====
	Review *service.ReviewService

	// ===== Certificate =====
	Certificate *service.CertificateService

	// ===== Report =====
	Report *service.ReportService

	// ===== Coin =====
	Coin *service.CoinService

	// ===== Group =====
	Group *service.GroupService

	// ===== Conversation =====
	Conversation *service.ConversationService

	// ===== Contest =====
	Contest *service.ContestService

	// ===== Personal Event =====
	PersonalEvent *service.PersonalEventService
}

func InitServices(resources *Resources, repos *Repositories, notifier *socket.Notifier) *Services {
	transactionSvc := initTransactionService(resources.Config)
	// voucherSvc khởi tạo SỚM (trước Order/Payment) vì item 24 (review web vòng 1) cần
	// OrderService/PaymentService dùng voucherSvc.ValidateAndApplyVoucher/IncrementUsedCount/
	// RecordUsageLog thay cho couponRepo — bảng coupons không còn route/handler nào tạo dữ
	// liệu (đã grep xác nhận), bảng vouchers mới là bảng web thực sự dùng.
	voucherSvc := service.NewVoucherService(repos.Voucher, repos.User)

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
			resources.Config,
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
		repos.Class,
		repos.Course,
		// V3-6 (issue #58): Join can kiem nguoi tham gia co enroll khoa cua phien khong.
		repos.Enrollment,
		resources.Redis,
		livekitSvc,
		resources.Queue,
		resources.Config,
	)

	assignmentSvc := service.NewAssignmentService(
		repos.Assignment,
		repos.TestCase,
		repos.Submission,
		// S3: đọc đề chỉ cho thành viên lớp/phiên — dùng đúng định nghĩa thành viên của chat/bảng trắng.
		repos.Class,
		livestreamSvc,
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
		// V3-6 (issue #58): dung livestreamSvc.EnsureSessionMember/EnsureSessionManage lam nguon
		// su that duy nhat cho quyen thanh vien/quan tri phien — khong lam lai phep kiem nay.
		livestreamSvc,
	)

	whiteboardSvc := service.NewWhiteboardService(
		repos.Whiteboard,
		repos.Livestream,
		resources.Redis,
		livestreamSvc,
	)

	// P1 QA 260927 teacher: repos.Livestream/Class/Course them vao de AnalyticsService kiem
	// duoc "actor co la host/GV lop/instructor khoa cua phien hay khong" truoc khi tra so lieu
	// (xem AnalyticsService.ensureSessionAnalyticsAccess) — truoc ban va nay 3 endpoint analytics
	// khong kiem gi ca, bat ky user dang nhap nao biet id la xem duoc so lieu cua lop/giao vien
	// khac.
	analyticsSvc := service.NewAnalyticsService(
		repos.Analytics,
		repos.Participant,
		repos.Submission,
		repos.Assignment,
		repos.Livestream,
		repos.Class,
		repos.Course,
	)

	// P1 QA 260927 teacher: TeacherService.GetMyStudents nay doc truc tiep tu enrollments (qua
	// EnrollmentRepository) thay vi di qua ClassService — khong con can mot instance ClassService
	// rieng chi de goi 1 method da xoa (GetTeacherStudents), Services.Class ben duoi da co instance
	// rieng cua no.
	teacherSvc := service.NewTeacherService(repos.Teacher, repos.Enrollment, repos.ParentStudent)

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
		repos.UserSystemRole,
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

	// ===== Order & Payment =====
	// M3-09 (review vòng 3b, bổ sung vòng 4): NewOrderService/NewPaymentService không còn nhận
	// couponRepo/enrollmentRepo/orderHistoryRepo (Order) hay courseRepo/orderItemRepo/couponRepo
	// (Payment) — cả 2 hàm dựng bản tx-bound của các repo này tại chỗ (từ txRepo.TxDB()) mỗi khi
	// cần. paymentEventRepo (Minor, review vòng 4b/5): xóa hẳn khỏi tham số NewPaymentService.
	orderSvc := service.NewOrderService(
		repos.Order,
		repos.OrderItem,
		repos.Course,
		repos.CartItem,
		repos.IdempotencyKey,
		voucherSvc,
	)
	// Review #76 vòng 3: transactionService đi qua coinTransactionServiceOrNil để gRPC khởi tạo lỗi
	// cho ra nil thật (không phải typed nil) — PaymentService coi nil là "chưa xác minh được" thay vì
	// panic khi đối chiếu.
	paymentSvc := service.NewPaymentService(
		repos.Order,
		repos.OrderStatusHistory,
		repos.Enrollment,
		voucherSvc,
		coinTransactionServiceOrNil(transactionSvc),
		repos.PlatformSetting,
	)
	// Review #76 vòng 3 MAJOR 3: huỷ đơn processing phải đối chiếu ngân hàng trước (OrderService
	// không có gRPC, nên dùng PaymentService làm bộ đối chiếu).
	orderSvc.SetPaymentReconciler(paymentSvc)

	// ================= Return Services =================
	s := &Services{
		// ===== Auth =====
		Auth:  authSvc,
		OAuth: oauthSvc,

		// ===== Parent Invitation =====
		ParentInvitation: parentInvitationSvc,

		// ===== Parent Dashboard =====
		ParentDashboard: service.NewParentDashboardService(
			repos.ParentStudent,
			repos.User,
			repos.Enrollment,
			repos.Grade,
			repos.Schedule,
			repos.Submission,
			repos.UserStats,
		),
		ParentLink: service.NewParentLinkService(resources.DB),

		// ===== Role =====
		Role:       service.NewRoleService(repos.Role, repos.Permission),
		SystemRole: service.NewSystemRoleService(repos.SystemRole, repos.Permission),

		UserSystemRole: service.NewUserSystemRoleService(
			repos.UserSystemRole,
			repos.User,
			repos.SystemRole,
		),

		// Phase 1 quản lý người dùng (2026-09-28) — tái dùng authSvc.RevokeAllSessions khi
		// khoá tài khoản, không viết lại logic INCR/DEL Redis lần 2.
		UserAdmin: service.NewUserAdminService(
			repos.User,
			repos.SystemRole,
			repos.UserSystemRole,
			authSvc,
			resources.Redis,
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
		Class: service.NewClassService(repos.Class, repos.Course, repos.Teacher, repos.Student, repos.ParentStudent),
		// V3-7 (issue #58): repos.Course duoc chen vao de kiem instructor cua khoa chua lop
		// (class_access.go) — xem NewClassLessonContentService.
		ClassLessonContent: service.NewClassLessonContentService(repos.ClassLessonContent, repos.Class, repos.Course, repos.Lesson, repos.Enrollment, livestreamSvc),
		Attendance:         service.NewAttendanceService(repos.Attendance, repos.Class, repos.Course),

		// ===== Teacher =====
		Teacher:        teacherSvc,
		TeacherProfile: service.NewTeacherProfileService(repos.TeacherProfile),
		// Phase 3: authSvc làm RoleChangeNotifier (bump user_version + marker đổi vai trò).
		CourseReview:       service.NewCourseReviewService(repos.CourseReview),
		TeacherApplication: service.NewTeacherApplicationService(repos.TeacherApplication, repos.TeacherProfile, authSvc),

		// ===== Course Management =====
		Category: service.NewCategoryService(repos.Category),
		Tag:      service.NewTagService(repos.Tag),
		Cart:     service.NewCartService(repos.CartItem, repos.Course, repos.Enrollment),
		// C-1 (review vòng 2): toán hạng repos.Enrollment thêm vào để GetCourseByID tính được
		// locked/lock_reason/progress theo người đang xem (xem service.NewCourseService).
		CourseService: service.NewCourseService(repos.Course, repos.Category, repos.Tag, repos.Enrollment),
		Section:       service.NewSectionService(repos.Section, repos.Course, repos.Enrollment),
		Lesson:        service.NewLessonService(repos.Lesson, repos.Section, repos.Course, repos.Enrollment).WithUploadOwnership(uploadSvc),
		LessonContent: service.NewLessonContentService(repos.Lesson, repos.Section, repos.Course, repos.Enrollment, uploadSvc),
		Enrollment:    service.NewEnrollmentService(repos.Enrollment, repos.Course, repos.Lesson, repos.VideoUpload),

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
		// Dựng ở trên (khối "Order & Payment") để nối bộ đối chiếu thanh toán vào OrderService.
		Order:   orderSvc,
		Payment: paymentSvc,
		TransactionService: transactionSvc,
		Voucher:            voucherSvc,
		AdminOrder: service.NewAdminOrderService(
			repos.Order,
			repos.OrderItem,
			repos.Enrollment,
			repos.Course,
		),
		PlatformSetting: service.NewPlatformSettingService(repos.PlatformSetting),

		// ===== Gamification =====
		Achievement: service.NewAchievementService(repos.Achievement),
		Leaderboard: service.NewLeaderboardService(repos.Leaderboard),
		UserStats:   service.NewUserStatsService(repos.UserStats, repos.UserPreference),
		// ===== Wallet =====
		Wallet:     service.NewWalletService(repos.Wallet, repos.TeacherProfile, resources.Config.WithdrawalMinAmount),
		Withdrawal: service.NewWithdrawalService(repos.Withdrawal, repos.Wallet, resources.Config.WithdrawalMinAmount),

		// ===== Discussion Forum =====
		// R7 (code-reviewer-260919-1557): repos.Enrollment them vao de kiem enroll/lesson_id
		// truoc khi doc/ghi hoi dap theo bai — xem DiscussionService.requireEnrolledInLessonCourse.
		Discussion: service.NewDiscussionService(repos.Discussion, repos.Enrollment),
		// ===== Note (Phase 1 §3) =====
		Note: service.NewNoteService(repos.Note, repos.Enrollment),

		// ===== Notification =====
		Notification: service.NewNotificationService(repos.Notification, notifier),

		// ===== User Preference =====
		UserPreference: service.NewUserPreferenceService(repos.UserPreference),

		// ===== Schedule (Asynq for reminders + Redis cache) =====
		Schedule: service.NewScheduleService(repos.Schedule, repos.Class, repos.Course, resources.Redis, resources.Queue),

		// ===== Quiz (Redis cache) =====
		// enrollmentRepo (SEC-1, vá lộ nội dung quiz): cần cho checkLessonQuizAccess dùng chung
		// gatherLessonLockInput với LessonContentService — xem quiz_service.go.
		Quiz: service.NewQuizService(repos.Quiz, resources.Redis, repos.Course, repos.Section, repos.Lesson, repos.Livestream, repos.Enrollment),

		// ===== Grade (Redis cache) =====
		Grade: service.NewGradeService(repos.Grade, repos.Class, resources.Redis),

		// ===== Exercise (RabbitMQ for code execution + Redis cache) =====
		Exercise: service.NewExerciseService(repos.Exercise, resources.Redis, resources.RabbitMQ),

		// ===== Review (Redis cache for ratings) =====
		Review: service.NewReviewService(repos.Review, repos.Course, repos.Enrollment, resources.Redis),

		// ===== Certificate (RabbitMQ for PDF generation) =====
		Certificate: service.NewCertificateService(
			repos.Certificate,
			repos.Enrollment,
			resources.Redis,
			resources.RabbitMQ,
		),

		// ===== Report =====
		Report: service.NewReportService(repos.Report),

		// ===== Coin =====
		// C4: CoinService cần transactionSvc để VerifyPurchase xác minh giao dịch
		// ngân hàng thật trước khi cộng xu (không tự cộng theo lời gọi của user).
		// coinTxSvc chỉ gán khi transactionSvc thực sự khác nil: gán trực tiếp con
		// trỏ *service.TransactionService (kể cả khi nil) vào interface sẽ tạo ra
		// "typed nil" khiến check `== nil` phía trong CoinService không còn đúng.
		Coin: service.NewCoinService(
			repos.CoinWallet,
			repos.CoinTransaction,
			repos.CoinPackage,
			repos.CoinPurchase,
			coinTransactionServiceOrNil(transactionSvc),
		),

		// ===== Group =====
		Group: service.NewGroupService(
			repos.Group,
			repos.GroupMember,
			repos.GroupJoinRequest,
			repos.Conversation,
			repos.ConversationParticipant,
		),

		// ===== Conversation =====
		Conversation: service.NewConversationService(
			repos.Conversation,
			repos.ConversationParticipant,
			repos.Message,
			repos.MessageReaction,
			notifier,
			// Lane G (QA 260927): giới hạn tạo cuộc trò chuyện trực tiếp mới theo quan hệ.
			repos.Enrollment,
			repos.ParentStudent,
			repos.UserSystemRole,
		),

		// ===== Personal Event =====
		PersonalEvent: service.NewPersonalEventService(repos.PersonalEvent, resources.Queue),
	}
	// Lane S2: moi vao nhom dung chung guard nhan tin cua Conversation (Lane G).
	s.Group.SetInviteGuard(s.Conversation)
	s.Group.SetChannelEvictor(notifier)
	wireContest(s, repos)
	return s
}

// wireContest — MVP "Cuộc thi" (contract §6): ContestService dùng QuizService làm engine chấm bài
// và ContestRewardService (voucher + thông báo) làm issuer; ngược lại QuizService gọi
// ContestService (gate) để khoá quiz đã gắn cuộc thi. Gate KHÔNG được để nil sau khi B1 merge —
// quiz gắn cuộc thi sẽ lộ đáp án qua /api/quizzes (test wiring ở services_contest_test.go).
func wireContest(s *Services, repos *Repositories) {
	s.Contest = service.NewContestService(
		repos.Contest,
		s.Quiz,
		service.NewContestRewardService(s.Voucher, s.Notification),
		repos.Enrollment,
	)
	s.Quiz.SetContestGate(s.Contest)
	s.Quiz.SetParentLinkChecker(repos.ParentStudent)
}

// initTransactionService creates the transaction gRPC service
func initTransactionService(cfg *config.Config) *service.TransactionService {
	transactionSvc, err := service.NewTransactionService(
		cfg.TransactionServiceHost,
		cfg.TransactionServicePort,
		cfg.TransactionServiceToken,
	)
	if err != nil {
		log.Printf("Warning: Failed to initialize transaction service: %v", err)
		return nil
	}

	log.Println("Transaction service (gRPC) initialized successfully")
	return transactionSvc
}

// coinTransactionServiceOrNil chuyển *service.TransactionService (con trỏ cụ thể,
// có thể nil khi gRPC service không khởi tạo được) sang interface một cách an
// toàn. Gán trực tiếp 1 con trỏ nil vào biến interface sẽ tạo ra "typed nil"
// (interface khác nil dù giá trị bên trong là nil) khiến `s.transactionService
// == nil` bên trong CoinService không còn phát hiện được — hàm này tránh bẫy đó
// bằng cách chỉ gán khi con trỏ thực sự khác nil.
func coinTransactionServiceOrNil(svc *service.TransactionService) service.TransactionServiceInterface {
	if svc == nil {
		return nil
	}
	return svc
}
