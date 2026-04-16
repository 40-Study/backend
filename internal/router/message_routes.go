package router

import (
	"github.com/gofiber/fiber/v2"
	"github.com/redis/go-redis/v9"
	"study.com/v1/internal/config"
	"study.com/v1/internal/handler"
	"study.com/v1/internal/middleware"
)

func SetupMessageRoutes(api fiber.Router, cfg *config.Config, h *handler.MessageHandler, redis *redis.Client) {
	auth := middleware.AuthMiddleware(cfg, redis)

	// ─── Conversations ──────────────────────────────────────────────────
	conversations := api.Group("/conversations")
	conversations.Use(auth)

	conversations.Get("/", h.ListConversations)
	conversations.Post("/direct", h.CreateDirectConversation)
	conversations.Get("/:id", h.GetConversation)

	// Conversation actions
	conversations.Post("/:id/read", h.MarkAsRead)
	conversations.Post("/:id/mute", h.MuteConversation)
	conversations.Post("/:id/unmute", h.UnmuteConversation)
	conversations.Post("/:id/pin", h.PinConversation)
	conversations.Post("/:id/unpin", h.UnpinConversation)

	// Messages within a conversation
	conversations.Get("/:id/messages", h.ListMessages)
	conversations.Post("/:id/messages", h.SendMessage)
	conversations.Put("/:id/messages/:messageId", h.EditMessage)
	conversations.Delete("/:id/messages/:messageId", h.DeleteMessage)

	// Message actions
	conversations.Post("/:id/messages/:messageId/pin", h.PinMessage)
	conversations.Post("/:id/messages/:messageId/unpin", h.UnpinMessage)

	// Reactions
	conversations.Post("/:id/messages/:messageId/reactions", h.AddReaction)
	conversations.Delete("/:id/messages/:messageId/reactions/:emoji", h.RemoveReaction)

	// ─── Global message endpoints ───────────────────────────────────────
	messages := api.Group("/messages")
	messages.Use(auth)

	messages.Get("/search", h.SearchMessages)
	messages.Get("/unread", h.GetUnreadCount)
}
