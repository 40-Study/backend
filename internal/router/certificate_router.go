package router

import (
	"github.com/gofiber/fiber/v2"
	"github.com/redis/go-redis/v9"
	"study.com/v1/internal/config"
	"study.com/v1/internal/handler"
	"study.com/v1/internal/middleware"
)

func SetupCertificateRoutes(
	api fiber.Router,
	cfg *config.Config,
	certificateHandler *handler.CertificateHandler,
	redis *redis.Client,
) {
	auth := middleware.AuthMiddleware(cfg, redis)

	// Public: verify certificate
	api.Get("/certificates/verify/:number", certificateHandler.VerifyCertificate)

	// Auth required
	certs := api.Group("/certificates", auth)
	{
		certs.Post("/", certificateHandler.IssueCertificate)
		certs.Get("/", certificateHandler.GetMyCertificates)
		certs.Get("/:id", certificateHandler.GetCertificateByID)
	}
}
