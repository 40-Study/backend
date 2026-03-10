package router

import (
	"github.com/gofiber/fiber/v2"
	"study.com/v1/internal/handler"
)

func SetupLivestreamRoutes(api fiber.Router, h *handler.LivestreamHandler) {
	live := api.Group("/livestream")

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

func SetupChatRoutes(api fiber.Router, h *handler.ChatHandler) {
	chat := api.Group("/chat")

	chat.Post("/send", h.Send)
	chat.Get("/:sessionId/messages", h.GetMessages)
	chat.Delete("/:id", h.DeleteMessage)
	chat.Post("/:id/pin", h.PinMessage)
}

func SetupWhiteboardRoutes(api fiber.Router, h *handler.WhiteboardHandler) {
	whiteboard := api.Group("/whiteboard")

	whiteboard.Get("/:sessionId/snapshot", h.GetSnapshot)
	whiteboard.Post("/:sessionId/snapshot", h.SaveSnapshot)
	whiteboard.Post("/:sessionId/event", h.BroadcastEvent)
}

func SetupAnalyticsRoutes(api fiber.Router, h *handler.AnalyticsHandler) {
	analytics := api.Group("/analytics")

	analytics.Get("/livestream/:sessionId", h.GetLivestreamAnalytics)
	analytics.Get("/assignment/:assignmentId", h.GetAssignmentAnalytics)
	analytics.Get("/participants/:sessionId", h.GetParticipantAnalytics)
}

func SetupAssignmentRoutes(api fiber.Router, h *handler.AssignmentHandler) {
	assignments := api.Group("/assignments")

	assignments.Post("/", h.Create)
	assignments.Get("/:id", h.GetByID)
	assignments.Put("/:id", h.Update)
	assignments.Delete("/:id", h.Delete)
	assignments.Post("/:id/publish", h.Publish)
	assignments.Post("/:id/unpublish", h.Unpublish)
	assignments.Get("/:id/testcases", h.GetTestCases)
	assignments.Post("/:id/testcases", h.AddTestCase)
}

func SetupSubmissionRoutes(api fiber.Router, h *handler.SubmissionHandler) {
	submissions := api.Group("/submissions")

	submissions.Post("/", h.Submit)
	submissions.Post("/run", h.RunCode)
	submissions.Get("/:id", h.GetByID)
	submissions.Get("/assignment/:assignmentId", h.GetByAssignment)
	submissions.Get("/user/:userId", h.GetByUser)
}
