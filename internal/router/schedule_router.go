package router

// import (
// 	"github.com/gofiber/fiber/v2"
// 	"github.com/redis/go-redis/v9"
// 	"study.com/v1/internal/config"
// 	"study.com/v1/internal/handler"
// 	"study.com/v1/internal/middleware"
// )

// func SetupScheduleRoutes(
// 	api fiber.Router,
// 	cfg *config.Config,
// 	scheduleHandler *handler.ScheduleHandler,
// 	sessionHandler *handler.ClassSessionHandler,
// 	redis *redis.Client,
// ) {
// 	auth := middleware.AuthMiddleware(cfg, redis)

// 	// ============================================================================
// 	// CLASS SCHEDULES - Lich hoc dinh ky
// 	// ============================================================================
// 	// POST   /api/classes/:classId/schedules           - Tao lich hoc dinh ky
// 	// GET    /api/classes/:classId/schedules           - Lay lich hoc cua class
// 	// GET    /api/classes/:classId/schedules/:id       - Lay chi tiet lich hoc
// 	// PUT    /api/classes/:classId/schedules/:id       - Cap nhat lich hoc
// 	// DELETE /api/classes/:classId/schedules/:id       - Xoa lich hoc
// 	// GET    /api/classes/:classId/timetable           - Lay timetable tuan/thang

// 	classSchedules := api.Group("/classes/:classId/schedules", auth)
// 	{
// 		classSchedules.Post("/", scheduleHandler.CreateSchedule)
// 		classSchedules.Get("/", scheduleHandler.GetSchedulesByClass)
// 		classSchedules.Get("/:id", scheduleHandler.GetScheduleByID)
// 		classSchedules.Put("/:id", scheduleHandler.UpdateSchedule)
// 		classSchedules.Delete("/:id", scheduleHandler.DeleteSchedule)
// 	}

// 	// Timetable
// 	api.Get("/classes/:classId/timetable", auth, scheduleHandler.GetClassTimetable)

// 	// ============================================================================
// 	// CLASS SESSIONS - Buoi hoc cu the
// 	// ============================================================================
// 	// GET    /api/classes/:classId/sessions            - Lay danh sach buoi hoc
// 	// POST   /api/classes/:classId/sessions            - Tao buoi hoc (ngoai recurring)
// 	// GET    /api/classes/:classId/sessions/:id        - Chi tiet buoi hoc
// 	// PUT    /api/classes/:classId/sessions/:id        - Cap nhat buoi hoc
// 	// DELETE /api/classes/:classId/sessions/:id        - Huy buoi hoc
// 	// POST   /api/classes/:classId/sessions/generate   - Generate sessions tu recurring schedule

// 	classSessions := api.Group("/classes/:classId/sessions", auth)
// 	{
// 		classSessions.Get("/", sessionHandler.GetSessionsByClass)
// 		classSessions.Post("/", sessionHandler.CreateSession)
// 		classSessions.Get("/:id", sessionHandler.GetSessionByID)
// 		classSessions.Put("/:id", sessionHandler.UpdateSession)
// 		classSessions.Delete("/:id", sessionHandler.CancelSession)
// 		classSessions.Post("/generate", sessionHandler.GenerateSessions)
// 	}

// 	// ============================================================================
// 	// SESSION ATTENDANCE - Diem danh theo buoi
// 	// ============================================================================
// 	// GET    /api/sessions/:sessionId/attendances      - Lay diem danh buoi hoc
// 	// POST   /api/sessions/:sessionId/attendances      - Diem danh hoc sinh
// 	// POST   /api/sessions/:sessionId/attendances/bulk - Diem danh hang loat
// 	// PUT    /api/sessions/:sessionId/attendances/:id  - Cap nhat diem danh
// 	// POST   /api/sessions/:sessionId/check-in         - Student tu check-in
// 	// POST   /api/sessions/:sessionId/check-out        - Student tu check-out

// 	sessionAttendances := api.Group("/sessions/:sessionId/attendances", auth)
// 	{
// 		sessionAttendances.Get("/", sessionHandler.GetSessionAttendances)
// 		sessionAttendances.Post("/", sessionHandler.MarkAttendance)
// 		sessionAttendances.Post("/bulk", sessionHandler.BulkMarkAttendance)
// 		sessionAttendances.Put("/:id", sessionHandler.UpdateAttendance)
// 	}

// 	api.Post("/sessions/:sessionId/check-in", auth, sessionHandler.StudentCheckIn)
// 	api.Post("/sessions/:sessionId/check-out", auth, sessionHandler.StudentCheckOut)

// 	// ============================================================================
// 	// MY TIMETABLE - Lich ca nhan
// 	// ============================================================================
// 	// GET    /api/me/timetable                         - Lay timetable ca nhan
// 	// GET    /api/me/attendances                       - Lich su diem danh cua toi

// 	api.Get("/me/timetable", auth, scheduleHandler.GetMyTimetable)
// 	api.Get("/me/attendances", auth, sessionHandler.GetMyAttendances)
// }
