package app

import (
	"context"
	"fmt"
	"log"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/cors"
	"github.com/google/uuid"
	"study.com/v1/internal/database/seeds"
	asynq_queue "study.com/v1/internal/queue/asynq"
	"study.com/v1/internal/router"
	"study.com/v1/internal/socket"
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

	seeder := seeds.NewSeeder(resources.DB)
	if err := seeder.SeedAll("./data"); err != nil {
		log.Printf("Warning: seeder failed: %v", err)
	}

	services := InitServices(resources, repos)

	// Register tasks sau khi có services để có thể inject livestream starter
	livestreamStarter := func(ctx context.Context, sessionID uuid.UUID) error {
		_, err := services.Livestream.Start(ctx, sessionID)
		return err
	}
	asynq_queue.RegisterTasks(resources.Queue, notifier, repos.Class, repos.Enrollment, resources.Redis, livestreamStarter)
	go resources.Queue.Start()
	handlers := InitHandlers(services, resources.MinioWrapper, resources.Config)

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
