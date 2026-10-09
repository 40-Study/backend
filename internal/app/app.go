package app

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/cors"
	"github.com/gofiber/fiber/v2/middleware/recover"
	"github.com/google/uuid"
	"study.com/v1/internal/database/seeds"
	"study.com/v1/internal/middleware"
	asynq_queue "study.com/v1/internal/queue/asynq"
	rabbitmq_queue "study.com/v1/internal/queue/rabbitmq"
	"study.com/v1/internal/router"
	"study.com/v1/internal/service"
	"study.com/v1/internal/socket"
	"study.com/v1/internal/utils"
)

type App struct {
	Resources *Resources
	Repos     *Repositories
	Services  *Services
	Handlers  *Handlers
	Fiber     *fiber.App
}

func New() (*App, error) {
	resources, err := InitResources()
	if err != nil {
		return nil, fmt.Errorf("failed to initialize resources: %w", err)
	}
	hub := socket.NewHub()
	go hub.Run()
	notifier := socket.NewNotifier(hub)
	repos := InitRepositories(resources.DB)
	// S6: authorizer thật (kiểm participant/thành viên nhóm), thay defaultAuthorizer cho phép mọi kênh.
	wsAuthorizer := newWSChannelAuthorizer(repos.ConversationParticipant, repos.GroupMember)
	wsAuthorizer.SetUserNameLookup(repos.User)
	socketHandler := socket.NewHandler(hub, wsAuthorizer)

	// C-02 (audit 260909): PermissionChecker triển khai thật cho RequirePermissions (trước
	// đây là no-op không dùng ở đâu). Dùng chung các repository RBAC đã có sẵn trong repos.
	// Tạo ở đây (trước InitHandlers) để C-12/H-11 (vòng 2) có thể tiêm vào CourseHandler/
	// SectionHandler/LessonHandler/ClassHandler — các handler này cần permChecker để tính
	// "actor có phải SYSTEM_ADMIN không" (override quyền owner/teacher).
	permChecker := middleware.NewPermissionChecker(repos.UserSystemRole, repos.SystemRole, repos.UserOrganizationRole, repos.Role)

	seeder := seeds.NewSeeder(resources.DB)
	if err := seeder.SeedAll("./data"); err != nil {
		log.Printf("Warning: seeder failed: %v", err)
	}

	services := InitServices(resources, repos, notifier, permChecker)
	// "Đang gõ" trong DM bị chặn không được phát cho phía bên kia (cùng quy tắc với khoá gửi tin).
	wireDirectBlockRealtime(services, wsAuthorizer)

	// Register tasks sau khi có services để có thể inject livestream starter
	// V3-6 (issue #58): Start() nay doi actorID (nguoi goi). Task auto-start chay nen khong co
	// nguoi goi nen dung StartAsSystem — xem comment tai LivestreamService.StartAsSystem.
	livestreamStarter := func(ctx context.Context, sessionID uuid.UUID) error {
		_, err := services.Livestream.StartAsSystem(ctx, sessionID)
		return err
	}
	asynq_queue.RegisterTasks(resources.Queue, notifier, repos.Class, repos.Enrollment, resources.Redis, livestreamStarter)
	// L6 mục 5: job nền đối chiếu ngân hàng (đơn chờ + đơn vừa hoàn tất), chu kỳ lấy từ config.
	if services.Payment != nil {
		if resources.Redis != nil {
			services.Payment.SetSweepLocker(service.NewRedisSweepLocker(resources.Redis))
		}
		services.Payment.SetSweepInterval(time.Duration(resources.Config.PaymentReconcileIntervalMinutes) * time.Minute) // cùng chu kỳ với lịch ngay dưới: khoá phân tán theo khung chu kỳ
		if err := asynq_queue.RegisterPaymentReconcile(resources.Queue, resources.Config.PaymentReconcileIntervalMinutes, services.Payment.RunReconcileSweep); err != nil {
			log.Printf("Warning: Failed to register payment reconcile job: %v", err)
		}
	}
	// M-05 (audit 260909 vòng 2): bọc SafeGo — goroutine chạy suốt vòng đời app, panic bên
	// trong (vd lỗi kết nối Redis/asynq giữa chừng) trước đây sập cả process.
	utils.SafeGo(func() { _ = resources.Queue.Start() })

	// Inject RabbitMQ vào ParentInvitationService + setup queue + start worker
	if resources.RabbitMQ != nil {
		services.ParentInvitation.SetRabbitMQ(resources.RabbitMQ)
		if err := rabbitmq_queue.SetupInvitationQueues(resources.RabbitMQ); err != nil {
			log.Printf("Warning: Failed to setup invitation queues: %v", err)
		} else {
			invitationWorker := rabbitmq_queue.NewInvitationWorker(resources.RabbitMQ, resources.Config, services.Notification)
			utils.SafeGo(func() {
				if err := invitationWorker.Start(context.Background()); err != nil {
					log.Printf("Warning: Failed to start invitation worker: %v", err)
				}
			})
		}
	}

	// Setup exercise submission queue (RabbitMQ)
	if services.Exercise != nil {
		if err := services.Exercise.SetupExerciseQueues(); err != nil {
			log.Printf("Warning: Failed to setup exercise queues: %v", err)
		}
	}

	// Setup certificate generation queue (RabbitMQ)
	if services.Certificate != nil {
		if err := services.Certificate.SetupCertificateQueues(); err != nil {
			log.Printf("Warning: Failed to setup certificate queues: %v", err)
		}
	}

	// Start video processing worker if available
	if services.VideoProcessing != nil {
		utils.SafeGo(func() {
			if err := services.VideoProcessing.StartWorker(context.Background()); err != nil {
				log.Printf("Warning: Failed to start video processing worker: %v", err)
			}
		})
		utils.SafeGo(func() {
			if err := services.VideoProcessing.StartCleanupScheduler(context.Background()); err != nil {
				log.Printf("Warning: Failed to start video cleanup scheduler: %v", err)
			}
		})
	}

	handlers := InitHandlers(services, repos, resources.MinioWrapper, resources.Config, permChecker)

	// Review PR #69 BLOCKER (QA 260927): fiber.New() KHÔNG có config nghĩa là c.IP() luôn trả
	// TCP peer trực tiếp — sau proxy Next.js, TẤT CẢ người dùng đứng chung 1 peer, nên mọi rate
	// limiter theo IP chia sẻ 1 bucket cho toàn bộ website. Xem BuildFiberConfig (fiber_config.go)
	// để biết chi tiết đầy đủ.
	fiberApp := fiber.New(BuildFiberConfig(resources.Config))

	// H-02 (audit 260909): trước đây không có recover middleware nên bất kỳ panic nào
	// (vd nil pointer dereference ở H-01) đều làm sập kết nối thay vì trả 500 có log.
	// Phải đứng TRƯỚC mọi route khác.
	fiberApp.Use(recover.New(recover.Config{EnableStackTrace: true}))

	// N4 (review vong 2, 260915): default va canh bao "*" gio nam mot noi duy nhat
	// (config.Config.ResolvedAllowedOrigins) — dung chung voi middleware.SameOriginRequired o
	// enrollment_router.go, tranh hai noi troi default lang le khoi nhau.
	allowedOrigins := resources.Config.ResolvedAllowedOrigins()
	fiberApp.Use(cors.New(cors.Config{
		AllowOrigins:     allowedOrigins,
		AllowMethods:     "GET, POST, PUT, DELETE, OPTIONS, PATCH",
		AllowHeaders:     "Origin, Content-Type, Accept, Authorization",
		AllowCredentials: true,
	}))

	// Setup WebSocket route with auth middleware
	auth := middleware.AuthMiddleware(resources.Config, resources.Redis)
	fiberApp.Get("/api/ws", auth, socketHandler.HandleWebSocket)

	router.SetupAllRoutes(
		fiberApp,
		resources.Config,
		permChecker,

		// ===== Auth & Role =====
		handlers.Auth,
		handlers.OAuth,
		handlers.Role,
		handlers.SystemRole,
		handlers.UserSystemRole,
		handlers.UserOrganizationRole,
		handlers.Permission,
		handlers.UserAdmin,

		// ===== Organization & Profile =====
		handlers.Organization,
		handlers.Profile,

		// ===== Teacher =====
		handlers.Teacher,
		handlers.TeacherProfile,
		handlers.Approval,

		// ===== Class =====
		handlers.Class,
		handlers.ClassLessonContent,
		handlers.Attendance,

		// ===== Course Management =====
		handlers.Category,
		handlers.Tag,
		handlers.Cart,
		handlers.CourseHandler,
		handlers.Section,
		handlers.Lesson,
		handlers.LessonContent,
		handlers.LessonPreview,
		handlers.Enrollment,

		// ===== Upload & Video =====
		handlers.Upload,
		handlers.VideoUpload,
		handlers.HLS,

		// ===== Livestream Learning Platform =====
		handlers.Livestream,
		handlers.Assignment,
		handlers.Submission,
		handlers.Chat,
		handlers.Whiteboard,
		handlers.Analytics,

		// ===== Order & Payment =====
		handlers.Order,
		handlers.AdminOrder,
		handlers.Voucher,

		// ===== Gamification =====
		handlers.Achievement,
		handlers.Leaderboard,
		handlers.UserStats,
		// ===== Wallet =====
		handlers.Wallet,

		// ===== Parent Invitation =====
		handlers.ParentInvitation,
		// ===== Parent Dashboard =====
		handlers.ParentDashboard,
		// ===== Discussion Forum =====
		handlers.Discussion,
		// ===== Note (Phase 1 §3) =====
		handlers.Note,

		// ===== Notification =====
		handlers.Notification,

		// ===== User Preference =====
		handlers.UserPreference,

		// ===== Schedule (Asynq) =====
		handlers.Schedule,

		// ===== Quiz =====
		handlers.Quiz,

		// ===== Grade =====
		handlers.Grade,

		// ===== Exercise (RabbitMQ) =====
		handlers.Exercise,

		// ===== Review =====
		handlers.Review,

		// ===== Certificate (RabbitMQ) =====
		handlers.Certificate,

		// ===== Report =====
		handlers.Report,

		// ===== Coin =====
		handlers.Coin,

		// ===== Group =====
		handlers.Group,

		// ===== Message =====
		handlers.Message,

		// ===== Contest =====
		handlers.Contest,

		// ===== Personal Event =====
		handlers.PersonalEvent,

		resources.Redis,
		resources.MinioClient,
		resources.Queue,
		// Recorder của middleware.Audit cho mọi route quản trị được ghi nhật ký (plan 261008 phase 8).
		services.AuditLog,
	)

	// Phase 4 rút tiền giảng viên: đăng ký riêng thay vì thêm tham số vào SetupAllRoutes (router
	// dùng chung nhiều lane đang sửa song song). Group "/api" thứ 2 chỉ là tiền tố, không kèm
	// middleware nào nên không ảnh hưởng route đã đăng ký trong SetupAllRoutes.
	router.SetupWithdrawalRoutes(fiberApp.Group("/api"), resources.Config, handlers.Withdrawal, resources.Redis, permChecker, services.AuditLog)
	// QA vòng 2 lane E: liên kết phụ huynh-học sinh do phụ huynh khởi xướng — cùng lý do đăng ký riêng.
	router.SetupParentLinkRoutes(fiberApp.Group("/api"), resources.Config, handlers.ParentLink, resources.Redis)
	// Bạn bè (phase 01 plan 260930): đăng ký riêng, cùng lý do.
	router.SetupFriendRoutes(fiberApp.Group("/api"), resources.Config, handlers.Friendship, resources.Redis)
	// Plan 261008: nhật ký hoạt động quản trị, broadcast thông báo hệ thống, cấu hình hệ thống — cùng lý do đăng ký riêng.
	router.SetupAuditLogRoutes(fiberApp.Group("/api"), resources.Config, handlers.AuditLog, resources.Redis, permChecker)
	router.SetupAdminBroadcastRoutes(fiberApp.Group("/api"), resources.Config, handlers.AdminBroadcast, resources.Redis, permChecker, services.AuditLog)
	router.SetupAdminSettingsRoutes(fiberApp.Group("/api"), resources.Config, handlers.AdminSettings, resources.Redis, permChecker)

	return &App{
		Resources: resources,
		Repos:     repos,
		Services:  services,
		Handlers:  handlers,
		Fiber:     fiberApp,
	}, nil
}

func (a *App) Run() error {
	defer func() {
		if err := a.Resources.Close(); err != nil {
			log.Printf("Error closing resources: %v", err)
		}
	}()

	addr := fmt.Sprintf("%s:%s", a.Resources.Config.Host, a.Resources.Config.Port)
	log.Printf("Server starting on %s", addr)

	if err := a.Fiber.Listen(addr); err != nil {
		return fmt.Errorf("server failed to start: %w", err)
	}

	return nil
}
