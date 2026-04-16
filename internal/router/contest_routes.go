package router

import (
	"github.com/gofiber/fiber/v2"
	"github.com/redis/go-redis/v9"
	"study.com/v1/internal/config"
	"study.com/v1/internal/handler"
	"study.com/v1/internal/middleware"
)

func SetupContestRoutes(api fiber.Router, cfg *config.Config, h *handler.ContestHandler, redis *redis.Client) {
	contests := api.Group("/contests")

	auth := middleware.AuthMiddleware(cfg, redis)

	// Public
	contests.Get("/", h.ListContests)
	contests.Get("/:slug", h.GetContest)

	// Auth required
	authed := contests.Group("")
	authed.Use(auth)

	// My contests
	authed.Get("/me", h.GetMyContests)

	// Contest CRUD
	authed.Post("/", h.CreateContest)
	authed.Put("/:id", h.UpdateContest)
	authed.Delete("/:id", h.DeleteContest)
	authed.Post("/:id/publish", h.PublishContest)

	// Problems
	authed.Get("/:id/problems", h.GetProblems)
	authed.Post("/:id/problems", h.CreateProblem)
	authed.Put("/:id/problems/:problemId", h.UpdateProblem)
	authed.Delete("/:id/problems/:problemId", h.DeleteProblem)

	// Participation
	authed.Post("/:id/join", h.JoinContest)
	authed.Get("/:id/leaderboard", h.GetLeaderboard)

	// Submissions
	authed.Post("/:id/problems/:problemId/submit", h.SubmitAnswer)
	authed.Get("/:id/submissions/me", h.GetMySubmissions)
}
