package app

import (
	"fmt"
	"log"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/cors"
	"study.com/v1/internal/database/seeds"
	"study.com/v1/internal/router"
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

	repos := InitRepositories(resources.DB)

	seeder := seeds.NewSeeder(resources.DB)
	if err := seeder.SeedAll("./data"); err != nil {
		log.Printf("Warning: seeder failed: %v", err)
	}

	services := InitServices(resources, repos)
	handlers := InitHandlers(services, resources.MinioWrapper)

	fiberApp := fiber.New()

	fiberApp.Use(cors.New(cors.Config{
		AllowOrigins:     "http://localhost:3000, http://localhost:3001",
		AllowMethods:     "GET, POST, PUT, DELETE, OPTIONS, PATCH",
		AllowHeaders:     "Origin, Content-Type, Accept, Authorization",
		AllowCredentials: true,
	}))

	router.SetupAllRoutes(
		fiberApp,
		resources.Config,

		// ===== Auth & Role =====
		handlers.Auth,
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
		handlers.ClassSchedule,
		handlers.Attendance,

		// ===== Course Management =====
		handlers.Category,
		handlers.Tag,
		handlers.CourseHandler,
		handlers.Section,
		handlers.Lesson,
		handlers.LessonContent,
		handlers.Enrollment,

		// ===== Video =====
		handlers.VideoUpload,
		handlers.HLS,

		// ===== LiveKit =====
		handlers.Livekit,

		// ===== Livestream Learning Platform =====
		handlers.Livestream,
		handlers.Assignment,
		handlers.Submission,
		handlers.Chat,
		handlers.Whiteboard,
		handlers.Analytics,

		resources.Redis,
		resources.MinioClient,
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
