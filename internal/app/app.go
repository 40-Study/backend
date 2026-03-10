package app

import (
	"fmt"
	"log"

	"github.com/gofiber/fiber/v2"
	"study.com/v1/internal/database/seeds"
	"study.com/v1/internal/router"
)

// App is the main application structure
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

	// Seed system roles and permissions on startup (idempotent)
	seeder := seeds.NewSeeder(resources.DB)
	if err := seeder.SeedAll("./data"); err != nil {
		log.Printf("Warning: seeder failed: %v", err)
	}

	services := InitServices(resources, repos)

	// ⚠ dùng signature mới có MinioWrapper
	handlers := InitHandlers(services, resources.MinioWrapper)

	fiberApp := fiber.New()

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
