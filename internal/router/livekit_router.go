package router

import (
	"github.com/gofiber/fiber/v2"
	"github.com/redis/go-redis/v9"
	"study.com/v1/internal/config"
	"study.com/v1/internal/handler"
	"study.com/v1/internal/middleware"
)

func SetupLiveRouter(api fiber.Router, cfg *config.Config, liveHandler *handler.LivekitHandler, redis *redis.Client) {
	live := api.Group("/live")
	live.Use(middleware.AuthMiddleware(cfg, redis))

	// Room management
	live.Post("/rooms", liveHandler.CreateRoom)
	live.Get("/rooms", liveHandler.ListRooms)
	live.Get("/rooms/:roomName", liveHandler.GetRoom)
	live.Delete("/rooms/:roomName", liveHandler.DeleteRoom)
	live.Put("/rooms/:roomName/metadata", liveHandler.UpdateRoomMetadata)

	// Token (join a room)
	live.Post("/rooms/:roomName/token", liveHandler.JoinToken)

	// Participant management
	live.Get("/rooms/:roomName/participants", liveHandler.ListParticipants)
	live.Get("/rooms/:roomName/participants/:identity", liveHandler.GetParticipant)
	live.Delete("/rooms/:roomName/participants/:identity", liveHandler.RemoveParticipant)
	live.Put("/rooms/:roomName/participants/:identity", liveHandler.UpdateParticipant)

	// Data messaging
	live.Post("/rooms/:roomName/data", liveHandler.SendData)
}
