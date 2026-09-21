package handler

import (
	"errors"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"study.com/v1/internal/dto"
	"study.com/v1/internal/service"
	"study.com/v1/internal/utils"
)

// mapDiscussionLessonErr (R7, code-reviewer-260919-1557): dung CHUNG cho CreatePost va
// ListByLesson — cung mot cap loi (lesson khong ton tai / user chua enroll) nen phai anh xa
// giong nhau o ca hai endpoint, dung mau voi note_handler.go.
func mapDiscussionLessonErr(c *fiber.Ctx, err error) error {
	switch {
	case errors.Is(err, service.ErrDiscussionLessonNotFound):
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "lesson not found"})
	case errors.Is(err, service.ErrDiscussionNotEnrolled):
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "not enrolled in the course containing this lesson"})
	default:
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
}

type DiscussionHandler struct {
	svc service.DiscussionServiceInterface
}

func NewDiscussionHandler(svc service.DiscussionServiceInterface) *DiscussionHandler {
	return &DiscussionHandler{svc: svc}
}

// CreatePost POST /api/discussions
func (h *DiscussionHandler) CreatePost(c *fiber.Ctx) error {
	userID, ok := c.Locals("user_id").(uuid.UUID)
	if !ok || userID == uuid.Nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "unauthorized"})
	}

	var req dto.CreateForumPostDTO
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}

	// R7 (code-reviewer-260919-1557): truoc ban va nay handler CHI BodyParser, khong bao gio
	// goi ValidateStruct — moi tag validate tren CreateForumPostDTO (required/min/oneof/uuid) la
	// tag CHET. Cung mau voi category_handler.go/cart_handler.go.
	if errs := utils.ValidateStruct(req); len(errs) > 0 {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Validation failed",
			"errors":  errs,
		})
	}

	post, err := h.svc.CreatePost(c.Context(), userID, req)
	if err != nil {
		if errors.Is(err, service.ErrDiscussionLessonNotFound) || errors.Is(err, service.ErrDiscussionNotEnrolled) {
			return mapDiscussionLessonErr(c, err)
		}
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}

	return c.Status(fiber.StatusCreated).JSON(fiber.Map{
		"message": "Post created",
		"data":    post,
	})
}

// ListPosts GET /api/discussions
func (h *DiscussionHandler) ListPosts(c *fiber.Ctx) error {
	category := c.Query("category", "")
	page := c.QueryInt("page", 1)
	pageSize := c.QueryInt("page_size", 10)

	var userID *uuid.UUID
	if uid, ok := c.Locals("user_id").(uuid.UUID); ok && uid != uuid.Nil {
		userID = &uid
	}

	result, err := h.svc.ListPosts(c.Context(), category, page, pageSize, userID)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(fiber.Map{"data": result})
}

// ListByLesson — GET /lessons/:lessonId/discussions?page=&limit= (Phase 1 §5). Tham số phân
// trang la "limit" (khong phai "page_size" nhu ListPosts) — dung theo dung ten web goi
// (web/src/services/discussion.service.ts, listByLesson).
func (h *DiscussionHandler) ListByLesson(c *fiber.Ctx) error {
	lessonID, err := uuid.Parse(c.Params("lessonId"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Invalid lesson ID", "error": err.Error(),
		})
	}
	page := c.QueryInt("page", 1)
	limit := c.QueryInt("limit", 10)

	var userID *uuid.UUID
	if uid, ok := c.Locals("user_id").(uuid.UUID); ok && uid != uuid.Nil {
		userID = &uid
	}

	result, err := h.svc.ListPostsByLesson(c.Context(), lessonID, page, limit, userID)
	if err != nil {
		return mapDiscussionLessonErr(c, err)
	}

	return c.JSON(fiber.Map{"message": "Discussions retrieved successfully", "data": result})
}

// GetPostBySlug GET /api/discussions/:slug
func (h *DiscussionHandler) GetPostBySlug(c *fiber.Ctx) error {
	slug := c.Params("slug")
	if slug == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "slug required"})
	}

	var userID *uuid.UUID
	if uid, ok := c.Locals("user_id").(uuid.UUID); ok && uid != uuid.Nil {
		userID = &uid
	}

	post, err := h.svc.GetPostBySlug(c.Context(), slug, userID)
	if err != nil {
		if err.Error() == "post not found" {
			return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "post not found"})
		}
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(fiber.Map{"data": post})
}

// AddComment POST /api/discussions/:slug/comments
func (h *DiscussionHandler) AddComment(c *fiber.Ctx) error {
	userID, ok := c.Locals("user_id").(uuid.UUID)
	if !ok || userID == uuid.Nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "unauthorized"})
	}

	slug := c.Params("slug")
	if slug == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "slug required"})
	}

	var req dto.CreateForumCommentDTO
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}

	comment, err := h.svc.AddComment(c.Context(), slug, userID, req)
	if err != nil {
		if err.Error() == "post not found" {
			return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "post not found"})
		}
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}

	return c.Status(fiber.StatusCreated).JSON(fiber.Map{
		"message": "Comment added",
		"data":    comment,
	})
}

// Vote POST /api/discussions/:id/vote
func (h *DiscussionHandler) Vote(c *fiber.Ctx) error {
	userID, ok := c.Locals("user_id").(uuid.UUID)
	if !ok || userID == uuid.Nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "unauthorized"})
	}

	discussionID, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid id"})
	}

	var req dto.VoteForumDTO
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}

	if err := h.svc.VoteDiscussion(c.Context(), discussionID, userID, req.VoteType); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(fiber.Map{"message": "Vote recorded"})
}

// RemoveVote DELETE /api/discussions/:id/vote
func (h *DiscussionHandler) RemoveVote(c *fiber.Ctx) error {
	userID, ok := c.Locals("user_id").(uuid.UUID)
	if !ok || userID == uuid.Nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "unauthorized"})
	}

	discussionID, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid id"})
	}

	if err := h.svc.RemoveVote(c.Context(), discussionID, userID); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(fiber.Map{"message": "Vote removed"})
}

// DeletePost DELETE /api/discussions/:id
func (h *DiscussionHandler) DeletePost(c *fiber.Ctx) error {
	userID, ok := c.Locals("user_id").(uuid.UUID)
	if !ok || userID == uuid.Nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "unauthorized"})
	}

	postID, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid id"})
	}

	if err := h.svc.DeletePost(c.Context(), postID, userID); err != nil {
		if err.Error() == "post not found" {
			return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "post not found"})
		}
		if err.Error() == "forbidden: not the owner" {
			return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "forbidden"})
		}
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(fiber.Map{"message": "Deleted"})
}
