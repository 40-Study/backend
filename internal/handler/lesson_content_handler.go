package handler

import (
	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"study.com/v1/internal/dto"
	"study.com/v1/internal/service"
)

type LessonContentHandler struct {
	service service.LessonContentServiceInterface
}

func NewLessonContentHandler(service service.LessonContentServiceInterface) *LessonContentHandler {
	return &LessonContentHandler{service: service}
}

// Video handlers

func (h *LessonContentHandler) CreateVideo(c *fiber.Ctx) error {
	lessonID, err := uuid.Parse(c.Params("lessonId"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Invalid lesson ID", "error": err.Error(),
		})
	}

	var req dto.CreateLessonVideoDTO
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Invalid request body", "error": err.Error(),
		})
	}

	video, err := h.service.CreateVideo(c.Context(), lessonID, req)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Failed to create video", "error": err.Error(),
		})
	}

	return c.Status(fiber.StatusCreated).JSON(fiber.Map{
		"message": "Video created successfully", "data": video,
	})
}

func (h *LessonContentHandler) GetVideo(c *fiber.Ctx) error {
	lessonID, err := uuid.Parse(c.Params("lessonId"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Invalid lesson ID", "error": err.Error(),
		})
	}

	video, err := h.service.GetVideo(c.Context(), lessonID)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"message": "Video not found", "error": err.Error(),
		})
	}

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"message": "Video retrieved successfully", "data": video,
	})
}

func (h *LessonContentHandler) UpdateVideo(c *fiber.Ctx) error {
	lessonID, err := uuid.Parse(c.Params("lessonId"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Invalid lesson ID", "error": err.Error(),
		})
	}

	var req dto.UpdateLessonVideoDTO
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Invalid request body", "error": err.Error(),
		})
	}

	video, err := h.service.UpdateVideo(c.Context(), lessonID, req)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Failed to update video", "error": err.Error(),
		})
	}

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"message": "Video updated successfully", "data": video,
	})
}

func (h *LessonContentHandler) DeleteVideo(c *fiber.Ctx) error {
	lessonID, err := uuid.Parse(c.Params("lessonId"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Invalid lesson ID", "error": err.Error(),
		})
	}

	if err := h.service.DeleteVideo(c.Context(), lessonID); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Failed to delete video", "error": err.Error(),
		})
	}

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"message": "Video deleted successfully",
	})
}

// Article handlers

func (h *LessonContentHandler) CreateArticle(c *fiber.Ctx) error {
	lessonID, err := uuid.Parse(c.Params("lessonId"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Invalid lesson ID", "error": err.Error(),
		})
	}

	var req dto.CreateLessonArticleDTO
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Invalid request body", "error": err.Error(),
		})
	}

	article, err := h.service.CreateArticle(c.Context(), lessonID, req)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Failed to create article", "error": err.Error(),
		})
	}

	return c.Status(fiber.StatusCreated).JSON(fiber.Map{
		"message": "Article created successfully", "data": article,
	})
}

func (h *LessonContentHandler) GetArticle(c *fiber.Ctx) error {
	lessonID, err := uuid.Parse(c.Params("lessonId"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Invalid lesson ID", "error": err.Error(),
		})
	}

	article, err := h.service.GetArticle(c.Context(), lessonID)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"message": "Article not found", "error": err.Error(),
		})
	}

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"message": "Article retrieved successfully", "data": article,
	})
}

func (h *LessonContentHandler) UpdateArticle(c *fiber.Ctx) error {
	lessonID, err := uuid.Parse(c.Params("lessonId"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Invalid lesson ID", "error": err.Error(),
		})
	}

	var req dto.UpdateLessonArticleDTO
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Invalid request body", "error": err.Error(),
		})
	}

	article, err := h.service.UpdateArticle(c.Context(), lessonID, req)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Failed to update article", "error": err.Error(),
		})
	}

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"message": "Article updated successfully", "data": article,
	})
}

func (h *LessonContentHandler) DeleteArticle(c *fiber.Ctx) error {
	lessonID, err := uuid.Parse(c.Params("lessonId"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Invalid lesson ID", "error": err.Error(),
		})
	}

	if err := h.service.DeleteArticle(c.Context(), lessonID); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Failed to delete article", "error": err.Error(),
		})
	}

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"message": "Article deleted successfully",
	})
}

// Attachment handlers

func (h *LessonContentHandler) CreateAttachment(c *fiber.Ctx) error {
	lessonID, err := uuid.Parse(c.Params("lessonId"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Invalid lesson ID", "error": err.Error(),
		})
	}

	var req dto.CreateLessonAttachmentDTO
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Invalid request body", "error": err.Error(),
		})
	}

	attachment, err := h.service.CreateAttachment(c.Context(), lessonID, req)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Failed to create attachment", "error": err.Error(),
		})
	}

	return c.Status(fiber.StatusCreated).JSON(fiber.Map{
		"message": "Attachment created successfully", "data": attachment,
	})
}

func (h *LessonContentHandler) GetAttachments(c *fiber.Ctx) error {
	lessonID, err := uuid.Parse(c.Params("lessonId"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Invalid lesson ID", "error": err.Error(),
		})
	}

	attachments, err := h.service.GetAttachments(c.Context(), lessonID)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"message": "Failed to retrieve attachments", "error": err.Error(),
		})
	}

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"message": "Attachments retrieved successfully", "data": attachments,
	})
}

func (h *LessonContentHandler) DeleteAttachment(c *fiber.Ctx) error {
	lessonID, err := uuid.Parse(c.Params("lessonId"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Invalid lesson ID", "error": err.Error(),
		})
	}

	attachmentID, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Invalid attachment ID", "error": err.Error(),
		})
	}

	if err := h.service.DeleteAttachment(c.Context(), lessonID, attachmentID); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Failed to delete attachment", "error": err.Error(),
		})
	}

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"message": "Attachment deleted successfully",
	})
}
