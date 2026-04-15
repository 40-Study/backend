package router

import (
	"github.com/gofiber/fiber/v2"
	"github.com/redis/go-redis/v9"
	"study.com/v1/internal/config"
	"study.com/v1/internal/handler"
	"study.com/v1/internal/middleware"
)

func SetupReportRoutes(
	api fiber.Router,
	cfg *config.Config,
	reportHandler *handler.ReportHandler,
	redis *redis.Client,
) {
	auth := middleware.AuthMiddleware(cfg, redis)

	reports := api.Group("/reports", auth)
	{
		reports.Post("/", reportHandler.CreateReport)
		reports.Get("/", reportHandler.ListReports)
		reports.Get("/my", reportHandler.GetMyReports)
		reports.Get("/:id", reportHandler.GetReportByID)
		reports.Put("/:id/status", reportHandler.UpdateReportStatus)
		reports.Delete("/:id", reportHandler.DeleteReport)
	}
}
