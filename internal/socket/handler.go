package socket

import (
	"log"
	"net"

	"github.com/gobwas/ws"
	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"github.com/valyala/fasthttp"
)

type Handler struct {
	hub        *Hub
	authorizer ChannelAuthorizer
}

func NewHandler(hub *Hub, authorizer ChannelAuthorizer) *Handler {
	return &Handler{hub: hub, authorizer: authorizer}
}

// HandleWebSocket handles the WebSocket upgrade request
func (h *Handler) HandleWebSocket(c *fiber.Ctx) error {
	// Get user_id from auth middleware
	userIDVal := c.Locals("user_id")
	if userIDVal == nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"message": "Unauthorized: missing user_id",
		})
	}

	userID, ok := userIDVal.(uuid.UUID)
	if !ok {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Invalid user_id",
		})
	}

	// Hijack the connection using fasthttp
	ctx := c.Context()
	ctx.HijackSetNoResponse(true)
	ctx.Hijack(func(conn net.Conn) {
		h.handleConnection(conn, userID)
	})

	return nil
}

func (h *Handler) handleConnection(conn net.Conn, userID uuid.UUID) {
	_, err := ws.Upgrade(conn)
	if err != nil {
		log.Printf("WebSocket upgrade failed for user %s: %v", userID, err)
		conn.Close()
		return
	}

	// Create new client
	client := NewClient(userID, conn, h.hub, h.authorizer)

	// Register client with hub
	h.hub.Register <- client

	// Start read/write pumps
	go client.WritePump()
	client.ReadPump()
}

func (h *Handler) HandleWebSocketFastHTTP(ctx *fasthttp.RequestCtx, userID uuid.UUID) {
	ctx.HijackSetNoResponse(true)
	ctx.Hijack(func(conn net.Conn) {
		h.handleConnection(conn, userID)
	})
}

// GetHub returns the hub instance
func (h *Handler) GetHub() *Hub {
	return h.hub
}
