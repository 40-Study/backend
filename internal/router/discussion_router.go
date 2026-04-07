package router

import (
	"github.com/gofiber/fiber/v2"
	"github.com/redis/go-redis/v9"
	"study.com/v1/internal/config"
	"study.com/v1/internal/handler"
	"study.com/v1/internal/middleware"
)

func SetupDiscussionRoutes(api fiber.Router, cfg *config.Config, h *handler.DiscussionHandler, redis *redis.Client) {
	discussions := api.Group("/discussions")

	// Public routes
	discussions.Get("/", h.ListPosts)
	discussions.Get("/:slug", h.GetPostBySlug)

	// Auth-required routes
	auth := discussions.Group("")
	auth.Use(middleware.AuthMiddleware(cfg, redis))
	auth.Post("/", h.CreatePost)
	auth.Post("/:slug/comments", h.AddComment)
	auth.Post("/:id/vote", h.Vote)
	auth.Delete("/:id/vote", h.RemoveVote)
	auth.Delete("/:id", h.DeletePost)
}
