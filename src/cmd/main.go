package main

import (
	"fmt"
	"log"

	"github.com/gofiber/fiber/v2"
	"tiger.com/v2/src/config"
)

func main() {
	// 1. Load Configuration
	cfg, err := config.LoadConfig()
	if err != nil {
		log.Fatalf("Failed to load configuration: %v", err)
	}

	app := fiber.New()

	addr := fmt.Sprintf("%s:%s", cfg.Host, cfg.Port)
	log.Printf("Server starting on %s", addr)
	if err := app.Listen(addr); err != nil {
		log.Fatalf("Server failed to start: %v", err)
	}

}
