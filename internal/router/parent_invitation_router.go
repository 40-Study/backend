package router

import (
	"github.com/gofiber/fiber/v2"
	"github.com/redis/go-redis/v9"
	"study.com/v1/internal/config"
	"study.com/v1/internal/handler"
	"study.com/v1/internal/middleware"
)

func SetupParentInvitationRoutes(api fiber.Router, cfg *config.Config, invitationHandler *handler.ParentInvitationHandler, redis *redis.Client) {
	invitations := api.Group("/invitations")

	// Public: validate token từ email link (không cần auth)
	invitations.Get("/validate/:token", invitationHandler.ValidateInvitationToken)

	// Protected routes
	invitations.Use(middleware.AuthMiddleware(cfg, redis))

	invitations.Post("/invite", invitationHandler.InviteParent)
	invitations.Get("/pending", invitationHandler.GetPendingInvitations)
	invitations.Get("/sent", invitationHandler.GetSentInvitations)
	invitations.Post("/:id/respond", invitationHandler.RespondToInvitation)
	invitations.Post("/:id/revoke", invitationHandler.RevokeInvitation)
}
