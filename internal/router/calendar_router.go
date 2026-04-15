package router

// import (
// 	"github.com/gofiber/fiber/v2"
// 	"github.com/redis/go-redis/v9"
// 	"study.com/v1/internal/config"
// 	"study.com/v1/internal/handler"
// 	"study.com/v1/internal/middleware"
// )

// func SetupCalendarRoutes(
// 	api fiber.Router,
// 	cfg *config.Config,
// 	calendarHandler *handler.CalendarHandler,
// 	reminderHandler *handler.ReminderHandler,
// 	redis *redis.Client,
// ) {
// 	auth := middleware.AuthMiddleware(cfg, redis)

// 	// ============================================================================
// 	// CALENDAR - Xem lich
// 	// ============================================================================
// 	// GET    /api/calendar                             - Lay events trong khoang thoi gian
// 	//        Query params:
// 	//        - start_date: RFC3339 (required)
// 	//        - end_date: RFC3339 (required)
// 	//        - types: livestream,assignment,class,quiz (optional, filter)
// 	//        - class_id: UUID (optional, filter theo class)
// 	// GET    /api/calendar/availability                - Kiem tra slot trong
// 	// GET    /api/calendar/conflicts                   - Kiem tra conflicts

// 	api.Get("/calendar", auth, calendarHandler.GetEvents)
// 	api.Get("/calendar/availability", auth, calendarHandler.CheckAvailability)
// 	api.Get("/calendar/conflicts", auth, calendarHandler.CheckConflicts)

// 	// ============================================================================
// 	// REMINDERS - Cai dat nhac nho
// 	// ============================================================================
// 	// GET    /api/reminders/settings                   - Lay cai dat reminder
// 	// PUT    /api/reminders/settings                   - Cap nhat cai dat
// 	// GET    /api/reminders/upcoming                   - Lay events sap toi (can nhac)

// 	api.Get("/reminders/settings", auth, reminderHandler.GetSettings)
// 	api.Put("/reminders/settings", auth, reminderHandler.UpdateSettings)
// 	api.Get("/reminders/upcoming", auth, reminderHandler.GetUpcomingEvents)

// 	// ============================================================================
// 	// TIMEZONE
// 	// ============================================================================
// 	// GET    /api/timezones                            - Lay danh sach timezones
// 	// PUT    /api/me/timezone                          - Cap nhat timezone ca nhan

// 	api.Get("/timezones", calendarHandler.GetTimezones) // Public
// 	api.Put("/me/timezone", auth, calendarHandler.UpdateMyTimezone)
// }
