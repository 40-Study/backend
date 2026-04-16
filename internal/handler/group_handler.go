package handler

import (
	"strconv"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"study.com/v1/internal/dto"
	"study.com/v1/internal/service"
	"study.com/v1/internal/utils"
)

type GroupHandler struct {
	groupService service.GroupServiceInterface
}

func NewGroupHandler(groupService service.GroupServiceInterface) *GroupHandler {
	return &GroupHandler{groupService: groupService}
}

// ─── Public endpoints ───────────────────────────────────────────────────────

// ListGroups - GET /api/groups
func (h *GroupHandler) ListGroups(c *fiber.Ctx) error {
	page, _ := strconv.Atoi(c.Query("page", "1"))
	limit, _ := strconv.Atoi(c.Query("limit", "20"))
	keyword := c.Query("keyword")
	privacy := c.Query("privacy")

	result, err := h.groupService.ListGroups(c.Context(), keyword, privacy, page, limit)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"message": err.Error(),
		})
	}

	return c.JSON(fiber.Map{
		"message": "Groups retrieved successfully",
		"data":    result,
	})
}

// GetGroupBySlug - GET /api/groups/:slug
func (h *GroupHandler) GetGroupBySlug(c *fiber.Ctx) error {
	slug := c.Params("slug")

	var userID *uuid.UUID
	if uid := c.Locals("user_id"); uid != nil {
		parsed, ok := uid.(uuid.UUID)
		if ok {
			userID = &parsed
		}
	}

	result, err := h.groupService.GetGroupBySlug(c.Context(), slug, userID)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"message": err.Error(),
		})
	}

	return c.JSON(fiber.Map{
		"message": "Group retrieved successfully",
		"data":    result,
	})
}

// ─── Group CRUD ─────────────────────────────────────────────────────────────

// CreateGroup - POST /api/groups
func (h *GroupHandler) CreateGroup(c *fiber.Ctx) error {
	userID, err := parseUserID(c)
	if err != nil {
		return err
	}

	var req dto.CreateGroupRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Invalid request body",
		})
	}

	if errs := utils.ValidateStruct(req); len(errs) > 0 {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Validation failed",
			"errors":  errs,
		})
	}

	result, err := h.groupService.CreateGroup(c.Context(), userID, req)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": err.Error(),
		})
	}

	return c.Status(fiber.StatusCreated).JSON(fiber.Map{
		"message": "Group created successfully",
		"data":    result,
	})
}

// UpdateGroup - PUT /api/groups/:id
func (h *GroupHandler) UpdateGroup(c *fiber.Ctx) error {
	userID, err := parseUserID(c)
	if err != nil {
		return err
	}

	groupID, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Invalid group ID",
		})
	}

	var req dto.UpdateGroupRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Invalid request body",
		})
	}

	result, err := h.groupService.UpdateGroup(c.Context(), userID, groupID, req)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": err.Error(),
		})
	}

	return c.JSON(fiber.Map{
		"message": "Group updated successfully",
		"data":    result,
	})
}

// DeleteGroup - DELETE /api/groups/:id
func (h *GroupHandler) DeleteGroup(c *fiber.Ctx) error {
	userID, err := parseUserID(c)
	if err != nil {
		return err
	}

	groupID, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Invalid group ID",
		})
	}

	if err := h.groupService.DeleteGroup(c.Context(), userID, groupID); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": err.Error(),
		})
	}

	return c.JSON(fiber.Map{
		"message": "Group deleted successfully",
	})
}

// ─── My groups ──────────────────────────────────────────────────────────────

// GetMyGroups - GET /api/groups/me/joined
func (h *GroupHandler) GetMyGroups(c *fiber.Ctx) error {
	userID, err := parseUserID(c)
	if err != nil {
		return err
	}

	page, _ := strconv.Atoi(c.Query("page", "1"))
	limit, _ := strconv.Atoi(c.Query("limit", "20"))

	result, err := h.groupService.GetMyGroups(c.Context(), userID, page, limit)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"message": err.Error(),
		})
	}

	return c.JSON(fiber.Map{
		"message": "My groups retrieved successfully",
		"data":    result,
	})
}

// GetMyOwnedGroups - GET /api/groups/me/owned
func (h *GroupHandler) GetMyOwnedGroups(c *fiber.Ctx) error {
	userID, err := parseUserID(c)
	if err != nil {
		return err
	}

	page, _ := strconv.Atoi(c.Query("page", "1"))
	limit, _ := strconv.Atoi(c.Query("limit", "20"))

	result, err := h.groupService.GetMyOwnedGroups(c.Context(), userID, page, limit)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"message": err.Error(),
		})
	}

	return c.JSON(fiber.Map{
		"message": "My owned groups retrieved successfully",
		"data":    result,
	})
}

// ─── Membership ─────────────────────────────────────────────────────────────

// JoinGroup - POST /api/groups/:id/join
func (h *GroupHandler) JoinGroup(c *fiber.Ctx) error {
	userID, err := parseUserID(c)
	if err != nil {
		return err
	}

	groupID, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Invalid group ID",
		})
	}

	var req dto.JoinGroupRequest
	_ = c.BodyParser(&req)

	result, err := h.groupService.JoinGroup(c.Context(), userID, groupID, req.Message)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": err.Error(),
		})
	}

	return c.JSON(fiber.Map{
		"message": "Join request processed",
		"data":    result,
	})
}

// LeaveGroup - POST /api/groups/:id/leave
func (h *GroupHandler) LeaveGroup(c *fiber.Ctx) error {
	userID, err := parseUserID(c)
	if err != nil {
		return err
	}

	groupID, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Invalid group ID",
		})
	}

	if err := h.groupService.LeaveGroup(c.Context(), userID, groupID); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": err.Error(),
		})
	}

	return c.JSON(fiber.Map{
		"message": "Left group successfully",
	})
}

// ─── Member management ──────────────────────────────────────────────────────

// ListMembers - GET /api/groups/:id/members
func (h *GroupHandler) ListMembers(c *fiber.Ctx) error {
	groupID, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Invalid group ID",
		})
	}

	page, _ := strconv.Atoi(c.Query("page", "1"))
	limit, _ := strconv.Atoi(c.Query("limit", "20"))

	result, err := h.groupService.ListMembers(c.Context(), groupID, page, limit)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"message": err.Error(),
		})
	}

	return c.JSON(fiber.Map{
		"message": "Members retrieved successfully",
		"data":    result,
	})
}

// InviteMembers - POST /api/groups/:id/members/invite
func (h *GroupHandler) InviteMembers(c *fiber.Ctx) error {
	userID, err := parseUserID(c)
	if err != nil {
		return err
	}

	groupID, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Invalid group ID",
		})
	}

	var req dto.InviteMembersRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Invalid request body",
		})
	}

	if errs := utils.ValidateStruct(req); len(errs) > 0 {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Validation failed",
			"errors":  errs,
		})
	}

	if err := h.groupService.InviteMembers(c.Context(), userID, groupID, req.UserIDs); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": err.Error(),
		})
	}

	return c.JSON(fiber.Map{
		"message": "Members invited successfully",
	})
}

// UpdateMemberRole - PUT /api/groups/:id/members/:userId/role
func (h *GroupHandler) UpdateMemberRole(c *fiber.Ctx) error {
	userID, err := parseUserID(c)
	if err != nil {
		return err
	}

	groupID, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Invalid group ID",
		})
	}

	targetUserID, err := uuid.Parse(c.Params("userId"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Invalid user ID",
		})
	}

	var req dto.UpdateMemberRoleRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Invalid request body",
		})
	}

	if errs := utils.ValidateStruct(req); len(errs) > 0 {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Validation failed",
			"errors":  errs,
		})
	}

	if err := h.groupService.UpdateMemberRole(c.Context(), userID, groupID, targetUserID, req.Role); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": err.Error(),
		})
	}

	return c.JSON(fiber.Map{
		"message": "Member role updated successfully",
	})
}

// RemoveMember - DELETE /api/groups/:id/members/:userId
func (h *GroupHandler) RemoveMember(c *fiber.Ctx) error {
	userID, err := parseUserID(c)
	if err != nil {
		return err
	}

	groupID, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Invalid group ID",
		})
	}

	targetUserID, err := uuid.Parse(c.Params("userId"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Invalid user ID",
		})
	}

	if err := h.groupService.RemoveMember(c.Context(), userID, groupID, targetUserID); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": err.Error(),
		})
	}

	return c.JSON(fiber.Map{
		"message": "Member removed successfully",
	})
}

// BanMember - POST /api/groups/:id/members/:userId/ban
func (h *GroupHandler) BanMember(c *fiber.Ctx) error {
	userID, err := parseUserID(c)
	if err != nil {
		return err
	}

	groupID, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Invalid group ID",
		})
	}

	targetUserID, err := uuid.Parse(c.Params("userId"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Invalid user ID",
		})
	}

	if err := h.groupService.BanMember(c.Context(), userID, groupID, targetUserID); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": err.Error(),
		})
	}

	return c.JSON(fiber.Map{
		"message": "Member banned successfully",
	})
}

// UnbanMember - POST /api/groups/:id/members/:userId/unban
func (h *GroupHandler) UnbanMember(c *fiber.Ctx) error {
	userID, err := parseUserID(c)
	if err != nil {
		return err
	}

	groupID, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Invalid group ID",
		})
	}

	targetUserID, err := uuid.Parse(c.Params("userId"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Invalid user ID",
		})
	}

	if err := h.groupService.UnbanMember(c.Context(), userID, groupID, targetUserID); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": err.Error(),
		})
	}

	return c.JSON(fiber.Map{
		"message": "Member unbanned successfully",
	})
}

// ─── Join requests ──────────────────────────────────────────────────────────

// ListJoinRequests - GET /api/groups/:id/requests
func (h *GroupHandler) ListJoinRequests(c *fiber.Ctx) error {
	userID, err := parseUserID(c)
	if err != nil {
		return err
	}

	groupID, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Invalid group ID",
		})
	}

	page, _ := strconv.Atoi(c.Query("page", "1"))
	limit, _ := strconv.Atoi(c.Query("limit", "20"))

	result, err := h.groupService.ListJoinRequests(c.Context(), userID, groupID, page, limit)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": err.Error(),
		})
	}

	return c.JSON(fiber.Map{
		"message": "Join requests retrieved successfully",
		"data":    result,
	})
}

// ApproveRequest - POST /api/groups/:id/requests/:requestId/approve
func (h *GroupHandler) ApproveRequest(c *fiber.Ctx) error {
	userID, err := parseUserID(c)
	if err != nil {
		return err
	}

	groupID, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Invalid group ID",
		})
	}

	requestID, err := uuid.Parse(c.Params("requestId"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Invalid request ID",
		})
	}

	if err := h.groupService.ApproveRequest(c.Context(), userID, groupID, requestID); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": err.Error(),
		})
	}

	return c.JSON(fiber.Map{
		"message": "Join request approved",
	})
}

// RejectRequest - POST /api/groups/:id/requests/:requestId/reject
func (h *GroupHandler) RejectRequest(c *fiber.Ctx) error {
	userID, err := parseUserID(c)
	if err != nil {
		return err
	}

	groupID, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Invalid group ID",
		})
	}

	requestID, err := uuid.Parse(c.Params("requestId"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Invalid request ID",
		})
	}

	var body dto.RejectRequestBody
	_ = c.BodyParser(&body)

	if err := h.groupService.RejectRequest(c.Context(), userID, groupID, requestID, body.Reason); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": err.Error(),
		})
	}

	return c.JSON(fiber.Map{
		"message": "Join request rejected",
	})
}

// parseUserID is defined in wallet_handler.go
