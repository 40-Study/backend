package handler

import (
	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"study.com/v1/internal/dto"
	"study.com/v1/internal/service"
	"study.com/v1/internal/utils"
)

type ParentInvitationHandler struct {
	invitationService service.ParentInvitationServiceInterface
}

func NewParentInvitationHandler(invitationService service.ParentInvitationServiceInterface) *ParentInvitationHandler {
	return &ParentInvitationHandler{
		invitationService: invitationService,
	}
}

// InviteParent handles POST /invitations/invite
func (h *ParentInvitationHandler) InviteParent(c *fiber.Ctx) error {
	userID, ok := c.Locals("user_id").(uuid.UUID)
	if !ok || userID == uuid.Nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"message": "Unauthorized",
		})
	}

	var req dto.InviteParentRequestDto
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Invalid request body",
			"error":   err.Error(),
		})
	}

	if errors := utils.ValidateStruct(req); len(errors) > 0 {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Validation failed",
			"errors":  errors,
		})
	}

	response, err := h.invitationService.InviteParent(c.Context(), userID, req)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"message": "Failed to send invitation",
			"error":   err.Error(),
		})
	}

	if response.Status == "error" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": response.Message,
			"data":    response,
		})
	}

	return c.Status(fiber.StatusCreated).JSON(fiber.Map{
		"message": response.Message,
		"data":    response,
	})
}

// ValidateInvitationToken handles GET /invitations/validate/:token (public)
func (h *ParentInvitationHandler) ValidateInvitationToken(c *fiber.Ctx) error {
	token := c.Params("token")
	if token == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Token is required",
		})
	}

	invitation, err := h.invitationService.ValidateInvitationToken(c.Context(), token)
	if err != nil || invitation == nil {
		return c.Status(fiber.StatusOK).JSON(fiber.Map{
			"message": "Invalid or expired token",
			"data": dto.ValidateInvitationTokenDto{
				Valid: false,
			},
		})
	}

	// Check expiration
	if invitation.Status != "invited" && invitation.Status != "pending" {
		return c.Status(fiber.StatusOK).JSON(fiber.Map{
			"message": "Invitation is no longer valid",
			"data": dto.ValidateInvitationTokenDto{
				Valid:        false,
				InvitationID: invitation.ID,
			},
		})
	}

	hasAccount := invitation.InviteeUserID != nil

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"message": "Token is valid",
		"data": dto.ValidateInvitationTokenDto{
			Valid:        true,
			InvitationID: invitation.ID,
			StudentName:  invitation.Student.UserName,
			Relationship: invitation.Relationship,
			HasAccount:   hasAccount,
			Email:        invitation.InviteeEmail,
		},
	})
}

// RespondToInvitation handles POST /invitations/:id/respond
func (h *ParentInvitationHandler) RespondToInvitation(c *fiber.Ctx) error {
	userID, ok := c.Locals("user_id").(uuid.UUID)
	if !ok || userID == uuid.Nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"message": "Unauthorized",
		})
	}

	invitationID, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Invalid invitation ID",
		})
	}

	var req dto.RespondInvitationRequestDto
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Invalid request body",
			"error":   err.Error(),
		})
	}

	if errors := utils.ValidateStruct(req); len(errors) > 0 {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Validation failed",
			"errors":  errors,
		})
	}

	if err := h.invitationService.RespondToInvitation(c.Context(), invitationID, userID, req.Action); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Failed to respond to invitation",
			"error":   err.Error(),
		})
	}

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"message": "Invitation responded successfully",
	})
}

// GetPendingInvitations handles GET /invitations/pending
func (h *ParentInvitationHandler) GetPendingInvitations(c *fiber.Ctx) error {
	userID, ok := c.Locals("user_id").(uuid.UUID)
	if !ok || userID == uuid.Nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"message": "Unauthorized",
		})
	}

	invitations, err := h.invitationService.GetPendingInvitations(c.Context(), userID)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"message": "Failed to get pending invitations",
			"error":   err.Error(),
		})
	}

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"message": "Success",
		"data":    invitations,
	})
}

// GetSentInvitations handles GET /invitations/sent
func (h *ParentInvitationHandler) GetSentInvitations(c *fiber.Ctx) error {
	userID, ok := c.Locals("user_id").(uuid.UUID)
	if !ok || userID == uuid.Nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"message": "Unauthorized",
		})
	}

	invitations, err := h.invitationService.GetSentInvitations(c.Context(), userID)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"message": "Failed to get sent invitations",
			"error":   err.Error(),
		})
	}

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"message": "Success",
		"data":    invitations,
	})
}

// RevokeInvitation handles POST /invitations/:id/revoke
func (h *ParentInvitationHandler) RevokeInvitation(c *fiber.Ctx) error {
	userID, ok := c.Locals("user_id").(uuid.UUID)
	if !ok || userID == uuid.Nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"message": "Unauthorized",
		})
	}

	invitationID, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Invalid invitation ID",
		})
	}

	if err := h.invitationService.RevokeInvitation(c.Context(), invitationID, userID); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Failed to revoke invitation",
			"error":   err.Error(),
		})
	}

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"message": "Invitation revoked successfully",
	})
}
