package router

import (
	"github.com/gofiber/fiber/v2"
	"github.com/redis/go-redis/v9"
	"study.com/v1/internal/config"
	"study.com/v1/internal/handler"
	"study.com/v1/internal/middleware"
)

func SetupGroupRoutes(api fiber.Router, cfg *config.Config, h *handler.GroupHandler, redis *redis.Client) {
	groups := api.Group("/groups")

	auth := middleware.AuthMiddleware(cfg, redis)

	// Public
	groups.Get("/", h.ListGroups)
	groups.Get("/:slug", h.GetGroupBySlug)

	// Auth required
	authed := groups.Group("")
	authed.Use(auth)

	// My groups
	authed.Get("/me/joined", h.GetMyGroups)
	authed.Get("/me/owned", h.GetMyOwnedGroups)

	// Group CRUD
	authed.Post("/", h.CreateGroup)
	authed.Put("/:id", h.UpdateGroup)
	authed.Delete("/:id", h.DeleteGroup)

	// Membership
	authed.Post("/:id/join", h.JoinGroup)
	authed.Post("/:id/leave", h.LeaveGroup)

	// Member management
	authed.Get("/:id/members", h.ListMembers)
	authed.Post("/:id/members/invite", h.InviteMembers)
	authed.Put("/:id/members/:userId/role", h.UpdateMemberRole)
	authed.Delete("/:id/members/:userId", h.RemoveMember)
	authed.Post("/:id/members/:userId/ban", h.BanMember)
	authed.Post("/:id/members/:userId/unban", h.UnbanMember)

	// Join requests (private groups)
	authed.Get("/:id/requests", h.ListJoinRequests)
	authed.Post("/:id/requests/:requestId/approve", h.ApproveRequest)
	authed.Post("/:id/requests/:requestId/reject", h.RejectRequest)
}
