package app

import (
	"gorm.io/gorm"
	"study.com/v1/internal/repository"
)

type Repositories struct {
	// ===== User & Role =====
	User                 *repository.UserRepository
	Role                 *repository.RoleRepository
	SystemRole           *repository.SystemRoleRepository
	UserSystemRole       *repository.UserSystemRoleRepository
	UserOrganizationRole *repository.UserOrganizationRoleRepository
	Permission           *repository.PermissionRepository

	// ===== Organization =====
	Organization  *repository.OrganizationRepository
	ParentStudent *repository.ParentStudentRepository

	// ===== Teacher =====
	Teacher        *repository.TeacherRepository
	TeacherProfile *repository.TeacherProfileRepository

	// ===== Class =====
	Class              *repository.ClassRepository
	ClassLessonContent *repository.ClassLessonContentRepository
	Attendance         *repository.AttendanceRepository

	// ===== Course & Student =====
	Course  *repository.CourseRepository
	Student *repository.StudentRepository

	// ===== Course Management =====
	Category   *repository.CategoryRepository
	Tag        *repository.TagRepository
	CartItem   *repository.CartItemRepository
	Section    *repository.SectionRepository
	Lesson     *repository.LessonRepository
	Enrollment *repository.EnrollmentRepository

	// ===== Video =====
	VideoUpload *repository.VideoUploadRepository

	// ===== Livestream Learning Platform =====
	Livestream  *repository.LivestreamRepository
	Participant *repository.ParticipantRepository
	Assignment  *repository.AssignmentRepository
	TestCase    *repository.TestCaseRepository
	Submission  *repository.SubmissionRepository
	ChatMessage *repository.ChatMessageRepository
	Whiteboard  *repository.WhiteboardRepository
	Analytics   *repository.AnalyticsRepository

	// ===== Order & Payment =====
	Order              *repository.OrderRepository
	OrderItem          *repository.OrderItemRepository
	Coupon             *repository.CouponRepository
	Voucher            *repository.VoucherRepository
	OrderStatusHistory *repository.OrderStatusHistoryRepository
	PaymentEvent       *repository.PaymentEventRepository
	IdempotencyKey     *repository.IdempotencyKeyRepository
	OrderLock          *repository.OrderLockRepository

	// ===== Gamification =====
	Achievement *repository.AchievementRepository
	Leaderboard *repository.LeaderboardRepository
	UserStats   *repository.UserStatsRepository
	// ===== Wallet =====
	Wallet *repository.WalletRepository

	// ===== OAuth =====
	OAuthProvider *repository.OAuthProviderRepository

	// ===== Parent Invitation =====
	ParentInvitation repository.ParentInvitationRepositoryInterface
	// ===== Discussion Forum =====
	Discussion *repository.DiscussionRepository

	// ===== Notification =====
	Notification *repository.NotificationRepository

	// ===== User Preference =====
	UserPreference *repository.UserPreferenceRepository
}

func InitRepositories(db *gorm.DB) *Repositories {
	return &Repositories{
		// ===== User & Role =====
		User:                 repository.NewUserRepository(db),
		Role:                 repository.NewRoleRepository(db),
		SystemRole:           repository.NewSystemRoleRepository(db),
		UserSystemRole:       repository.NewUserSystemRoleRepository(db),
		UserOrganizationRole: repository.NewUserOrganizationRoleRepository(db),
		Permission:           repository.NewPermissionRepository(db),

		// ===== Organization =====
		Organization:  repository.NewOrganizationRepository(db),
		ParentStudent: repository.NewParentStudentRepository(db),

		// ===== Teacher =====
		Teacher:        repository.NewTeacherRepository(db),
		TeacherProfile: repository.NewTeacherProfileRepository(db),

		// ===== Class =====
		Class:              repository.NewClassRepository(db),
		ClassLessonContent: repository.NewClassLessonContentRepository(db),
		Attendance:         repository.NewAttendanceRepository(db),

		// ===== Course & Student =====
		Course:  repository.NewCourseRepository(db),
		Student: repository.NewStudentRepository(db),

		// ===== Course Management =====
		Category:   repository.NewCategoryRepository(db),
		Tag:        repository.NewTagRepository(db),
		CartItem:   repository.NewCartItemRepository(db),
		Section:    repository.NewSectionRepository(db),
		Lesson:     repository.NewLessonRepository(db),
		Enrollment: repository.NewEnrollmentRepository(db),

		// ===== Video =====
		VideoUpload: repository.NewVideoUploadRepository(db),
		// ===== Livestream Learning Platform =====
		Livestream:  repository.NewLivestreamRepository(db),
		Participant: repository.NewParticipantRepository(db),
		Assignment:  repository.NewAssignmentRepository(db),
		TestCase:    repository.NewTestCaseRepository(db),
		Submission:  repository.NewSubmissionRepository(db),
		ChatMessage: repository.NewChatMessageRepository(db),
		Whiteboard:  repository.NewWhiteboardRepository(db),
		Analytics:   repository.NewAnalyticsRepository(db),

		// ===== Order & Payment =====
		Order:              repository.NewOrderRepository(db),
		OrderItem:          repository.NewOrderItemRepository(db),
		Coupon:             repository.NewCouponRepository(db),
		Voucher:            repository.NewVoucherRepository(db),
		OrderStatusHistory: repository.NewOrderStatusHistoryRepository(db),
		PaymentEvent:       repository.NewPaymentEventRepository(db),
		IdempotencyKey:     repository.NewIdempotencyKeyRepository(db),
		OrderLock:          repository.NewOrderLockRepository(db),

		// ===== Gamification =====
		Achievement: repository.NewAchievementRepository(db),
		Leaderboard: repository.NewLeaderboardRepository(db),
		UserStats:   repository.NewUserStatsRepository(db),
		// ===== Wallet =====
		Wallet: repository.NewWalletRepository(db),

		// ===== OAuth =====
		OAuthProvider: repository.NewOAuthProviderRepository(db),

		// ===== Parent Invitation =====
		ParentInvitation: repository.NewParentInvitationRepository(db),
		// ===== Discussion Forum =====
		Discussion: repository.NewDiscussionRepository(db),

		// ===== Notification =====
		Notification: repository.NewNotificationRepository(db),

		// ===== User Preference =====
		UserPreference: repository.NewUserPreferenceRepository(db),
	}
}
