package app

import (
	"context"
	"fmt"
	"log"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/cors"
	"github.com/google/uuid"
	"study.com/v1/internal/database/seeds"
	"study.com/v1/internal/middleware"
	asynq_queue "study.com/v1/internal/queue/asynq"
	rabbitmq_queue "study.com/v1/internal/queue/rabbitmq"
	"study.com/v1/internal/router"
	"study.com/v1/internal/socket"
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

	seeder := seeds.NewSeeder(resources.DB)
	if err := seeder.SeedAll("./data"); err != nil {
		log.Printf("Warning: seeder failed: %v", err)
	}

	services := InitServices(resources, repos, notifier)

	// Register tasks sau khi có services để có thể inject livestream starter
	livestreamStarter := func(ctx context.Context, sessionID uuid.UUID) error {
		_, err := services.Livestream.Start(ctx, sessionID)
		return err
	}
	asynq_queue.RegisterTasks(resources.Queue, notifier, repos.Class, repos.Enrollment, resources.Redis, livestreamStarter)
	go resources.Queue.Start()

	// Inject RabbitMQ vào ParentInvitationService + setup queue + start worker
	if resources.RabbitMQ != nil {
		services.ParentInvitation.SetRabbitMQ(resources.RabbitMQ)
		if err := rabbitmq_queue.SetupInvitationQueues(resources.RabbitMQ); err != nil {
			log.Printf("Warning: Failed to setup invitation queues: %v", err)
		} else {
			invitationWorker := rabbitmq_queue.NewInvitationWorker(resources.RabbitMQ, resources.Config, services.Notification)
			go func() {
				if err := invitationWorker.Start(context.Background()); err != nil {
					log.Printf("Warning: Failed to start invitation worker: %v", err)
				}
			}()
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
		go func() {
			if err := services.VideoProcessing.StartWorker(context.Background()); err != nil {
				log.Printf("Warning: Failed to start video processing worker: %v", err)
			}
		}()
		go func() {
			if err := services.VideoProcessing.StartCleanupScheduler(context.Background()); err != nil {
				log.Printf("Warning: Failed to start video cleanup scheduler: %v", err)
			}
		}()
	}

	handlers := InitHandlers(services, repos, resources.MinioWrapper, resources.Config)

	fiberApp := fiber.New()

	allowedOrigins := resources.Config.AllowedOrigins
	if allowedOrigins == "" {
		allowedOrigins = "http://localhost:3000"
	}
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
