package router

import (
	"github.com/gofiber/fiber/v2"
	"github.com/redis/go-redis/v9"
	"study.com/v1/internal/config"
	"study.com/v1/internal/handler"
	"study.com/v1/internal/middleware"
)

func SetupLivestreamRoutes(api fiber.Router, cfg *config.Config, h *handler.LivestreamHandler, redis *redis.Client) {
	live := api.Group("/livestream")
	live.Use(middleware.AuthMiddleware(cfg, redis))
	live.Post("/", h.Create)
	live.Get("/", h.GetAll)
	live.Get("/:id", h.GetByID)
	live.Put("/:id", h.Update)
	live.Delete("/:id", h.Delete)
	live.Post("/:id/start", h.Start)
	live.Post("/:id/end", h.End)
	live.Post("/:id/join", h.Join)
	live.Post("/:id/leave", h.Leave)
	live.Get("/:id/participants", h.GetParticipants)
	live.Post("/:id/mute", h.MuteParticipant)
	live.Post("/:id/kick", h.KickParticipant)
	live.Post("/:id/lock-whiteboard", h.LockWhiteboard)
	live.Post("/:id/unlock-whiteboard", h.UnlockWhiteboard)
	live.Post("/:id/screenshare/start", h.StartScreenShare)
	live.Post("/:id/screenshare/stop", h.StopScreenShare)
}

func SetupChatRoutes(api fiber.Router, cfg *config.Config, h *handler.ChatHandler, redis *redis.Client) {
	chat := api.Group("/chat")
	chat.Use(middleware.AuthMiddleware(cfg, redis))
	chat.Post("/send", h.Send)
	chat.Get("/:sessionId/messages", h.GetMessages)
	chat.Delete("/:id", h.DeleteMessage)
	chat.Post("/:id/pin", h.PinMessage)
	chat.Post("/:id/unpin", h.UnPinMessage)
}

func SetupWhiteboardRoutes(api fiber.Router, cfg *config.Config, h *handler.WhiteboardHandler, redis *redis.Client) {
	whiteboard := api.Group("/whiteboard")
	whiteboard.Use(middleware.AuthMiddleware(cfg, redis))
	whiteboard.Get("/:sessionId/snapshot", h.GetSnapshot)
	whiteboard.Post("/:sessionId/snapshot", h.SaveSnapshot)
	whiteboard.Post("/:sessionId/event", h.BroadcastEvent)
}

func SetupAnalyticsRoutes(api fiber.Router, cfg *config.Config, h *handler.AnalyticsHandler, redis *redis.Client) {
	analytics := api.Group("/analytics")
	analytics.Use(middleware.AuthMiddleware(cfg, redis))
	analytics.Get("/livestream/:sessionId", h.GetLivestreamAnalytics)
	analytics.Get("/assignment/:assignmentId", h.GetAssignmentAnalytics)
	analytics.Get("/participants/:sessionId", h.GetParticipantAnalytics)
}

func SetupAssignmentRoutes(api fiber.Router, cfg *config.Config, h *handler.AssignmentHandler, redis *redis.Client) {
	assignments := api.Group("/assignments")
	assignments.Use(middleware.AuthMiddleware(cfg, redis))

	assignments.Post("/", h.Create)
	assignments.Get("/", h.GetBySession)
	assignments.Get("/:id", h.GetByID)
	assignments.Get("/:id/sandbox", h.GetSandbox)
	assignments.Put("/:id", h.Update)
	assignments.Delete("/:id", h.Delete)
	assignments.Post("/:id/publish", h.Publish)
	assignments.Post("/:id/unpublish", h.Unpublish)
	assignments.Get("/:id/testcases", h.GetTestCases)
	assignments.Post("/:id/testcases", h.AddTestCase)
	assignments.Post("/:id/testcases/import", h.ImportTestCases)
	assignments.Delete("/:id/testcases/:tcId", h.DeleteTestCase)
}

func SetupSubmissionRoutes(api fiber.Router, cfg *config.Config, h *handler.SubmissionHandler, redis *redis.Client) {
	submissions := api.Group("/submissions")
	submissions.Use(middleware.AuthMiddleware(cfg, redis))
	submissions.Post("/", h.Submit)
	submissions.Post("/run", h.RunCode)
	submissions.Post("/run-custom", h.RunCustomCode)
	submissions.Post("/execute", h.ExecuteCode) // Free sandbox - no assignment required
	submissions.Get("/:id", h.GetByID)
	submissions.Get("/assignment/:assignmentId", h.GetByAssignment)
	submissions.Get("/my/:assignmentId", h.GetMySubmissions)
	submissions.Get("/user/:userId", h.GetByUser)
}
