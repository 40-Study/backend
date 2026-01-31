package app

import (
	"fmt"
	"log"

	"github.com/gofiber/fiber/v2"
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

	services := InitServices(resources, repos)

	handlers := InitHandlers(services)

	fiberApp := fiber.New()

	router.SetupAllRoutes(
		fiberApp,
		resources.Config,
		handlers.Auth,
		handlers.Role,
		handlers.Permission,
		handlers.Organization,
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

	// Start server
	addr := fmt.Sprintf("%s:%s", a.Resources.Config.Host, a.Resources.Config.Port)
	log.Printf("Server starting on %s", addr)

	if err := a.Fiber.Listen(addr); err != nil {
		return fmt.Errorf("server failed to start: %w", err)
	}

	return nil
}
