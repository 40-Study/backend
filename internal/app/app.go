package app

import (
	"context"
	"fmt"
	"log"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/cors"
	"github.com/gofiber/fiber/v2/middleware/recover"
	"github.com/google/uuid"
	"study.com/v1/internal/database/seeds"
	"study.com/v1/internal/middleware"
	asynq_queue "study.com/v1/internal/queue/asynq"
	rabbitmq_queue "study.com/v1/internal/queue/rabbitmq"
	"study.com/v1/internal/router"
	"study.com/v1/internal/socket"
	"study.com/v1/internal/utils"
)

// defaultAuthorizer allows all authenticated users to subscribe to their own channels
type defaultAuthorizer struct{}

func (a *defaultAuthorizer) CanSubscribe(userID uuid.UUID, channel string) (bool, error) {
	// Allow users to subscribe to their personal notification channel
	// Format: "user:{userID}"
	if channel == fmt.Sprintf("user:%s", userID.String()) {
		return true, nil
	}
	// Allow subscribing to general notifications
	if channel == "notifications" {
		return true, nil
	}
	// Allow subscribing to conversation channels
	// Format: "conversation:{conversationID}"
	if len(channel) > 13 && channel[:13] == "conversation:" {
		return true, nil
	}
	// Allow subscribing to group channels
	// Format: "group:{groupID}"
	if len(channel) > 6 && channel[:6] == "group:" {
		return true, nil
	}
	return false, nil
}

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
	socketHandler := socket.NewHandler(hub, &defaultAuthorizer{})
	repos := InitRepositories(resources.DB)

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

	services := InitServices(resources, repos, notifier)

	// Register tasks sau khi có services để có thể inject livestream starter
	// V3-6 (issue #58): Start() nay doi actorID (nguoi goi). Task auto-start chay nen khong co
	// nguoi goi nen dung StartAsSystem — xem comment tai LivestreamService.StartAsSystem.
	livestreamStarter := func(ctx context.Context, sessionID uuid.UUID) error {
		_, err := services.Livestream.StartAsSystem(ctx, sessionID)
		return err
	}
	asynq_queue.RegisterTasks(resources.Queue, notifier, repos.Class, repos.Enrollment, resources.Redis, livestreamStarter)
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

	fiberApp := fiber.New()

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

		// ===== Organization & Profile =====
		handlers.Organization,
		handlers.Profile,

		// ===== Teacher =====
		handlers.Teacher,
		handlers.TeacherProfile,

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
	)

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
