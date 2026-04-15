package router

import (
	"github.com/gofiber/fiber/v2"
	"github.com/redis/go-redis/v9"
	"study.com/v1/internal/config"
	"study.com/v1/internal/handler"
	"study.com/v1/internal/middleware"
)

func SetupParentDashboardRoutes(api fiber.Router, cfg *config.Config, h *handler.ParentDashboardHandler, redis *redis.Client) {
	parent := api.Group("/parent")
	parent.Use(middleware.AuthMiddleware(cfg, redis))

	children := parent.Group("/children/:id")
	children.Get("/overview", h.GetChildOverview)
	children.Get("/courses", h.GetChildCourses)
	children.Get("/grades", h.GetChildGrades)
	children.Get("/schedule", h.GetChildSchedule)
	children.Get("/timetable", h.GetChildTimetable)
	children.Get("/attendance", h.GetChildAttendance)
	children.Get("/assignments", h.GetChildAssignments)
}
