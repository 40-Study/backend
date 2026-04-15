package router

import (
	"github.com/gofiber/fiber/v2"
	"github.com/redis/go-redis/v9"
	"study.com/v1/internal/config"
	"study.com/v1/internal/handler"
	"study.com/v1/internal/middleware"
)

func SetupScheduleRoutes(
	api fiber.Router,
	cfg *config.Config,
	scheduleHandler *handler.ScheduleHandler,
	redis *redis.Client,
) {
	auth := middleware.AuthMiddleware(cfg, redis)

	// ============================================================================
	// CLASS SCHEDULES - Lich hoc dinh ky
	// ============================================================================
	classSchedules := api.Group("/classes/:classId/schedules", auth)
	{
		classSchedules.Post("/", scheduleHandler.CreateSchedule)
		classSchedules.Get("/", scheduleHandler.GetSchedulesByClass)
		classSchedules.Get("/:id", scheduleHandler.GetScheduleByID)
		classSchedules.Put("/:id", scheduleHandler.UpdateSchedule)
		classSchedules.Delete("/:id", scheduleHandler.DeleteSchedule)
	}

	// Timetable
	api.Get("/classes/:classId/timetable", auth, scheduleHandler.GetClassTimetable)

	// ============================================================================
	// CLASS SESSIONS - Buoi hoc cu the
	// ============================================================================
	classSessions := api.Group("/classes/:classId/sessions", auth)
	{
		classSessions.Get("/", scheduleHandler.GetSessionsByClass)
		classSessions.Post("/", scheduleHandler.CreateSession)
		classSessions.Get("/:id", scheduleHandler.GetSessionByID)
		classSessions.Put("/:id", scheduleHandler.UpdateSession)
		classSessions.Delete("/:id", scheduleHandler.CancelSession)
		classSessions.Post("/generate", scheduleHandler.GenerateSessions)
	}

	// ============================================================================
	// SESSION ATTENDANCE - Diem danh theo buoi
	// ============================================================================
	sessionAttendances := api.Group("/sessions/:sessionId/attendances", auth)
	{
		sessionAttendances.Get("/", scheduleHandler.GetSessionAttendances)
		sessionAttendances.Post("/", scheduleHandler.MarkAttendance)
		sessionAttendances.Post("/bulk", scheduleHandler.BulkMarkAttendance)
		sessionAttendances.Put("/:id", scheduleHandler.UpdateAttendance)
	}

	api.Post("/sessions/:sessionId/check-in", auth, scheduleHandler.StudentCheckIn)
	api.Post("/sessions/:sessionId/check-out", auth, scheduleHandler.StudentCheckOut)

	// ============================================================================
	// MY TIMETABLE & ATTENDANCE
	// ============================================================================
	api.Get("/me/timetable", auth, scheduleHandler.GetMyTimetable)
	api.Get("/me/attendances", auth, scheduleHandler.GetMyAttendances)

	// ============================================================================
	// REMINDER SETTINGS
	// ============================================================================
	api.Get("/reminders/settings", auth, scheduleHandler.GetReminderSettings)
	api.Put("/reminders/settings", auth, scheduleHandler.UpdateReminderSetting)
}
