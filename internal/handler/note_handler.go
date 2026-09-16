package handler

import (
	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"study.com/v1/internal/dto"
	"study.com/v1/internal/service"
	"study.com/v1/internal/utils"
)

// NoteHandler — ghi chú theo mốc thời gian trong bài học (Phase 1 §3).
type NoteHandler struct {
	service service.NoteServiceInterface
}

func NewNoteHandler(service service.NoteServiceInterface) *NoteHandler {
	return &NoteHandler{service: service}
}

// noteErrorStatus ánh xạ lỗi service sang mã HTTP; trả 0 khi không khớp lỗi đã biết để caller
// tiếp tục nhánh 400 mặc định.
func noteErrorStatus(err error) int {
	switch err {
	case service.ErrNotNoteOwner:
		return fiber.StatusForbidden
	case service.ErrNoteNotEnrolled:
		return fiber.StatusForbidden
	}
	return 0
}

func (h *NoteHandler) userIDFromLocals(c *fiber.Ctx) (uuid.UUID, bool) {
	userID, ok := c.Locals("user_id").(uuid.UUID)
	return userID, ok
}

// CreateNote — POST /lessons/:lessonId/notes
func (h *NoteHandler) CreateNote(c *fiber.Ctx) error {
	lessonID, err := uuid.Parse(c.Params("lessonId"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Invalid lesson ID", "error": err.Error(),
		})
	}
	userID, ok := h.userIDFromLocals(c)
	if !ok {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"message": "Unauthorized"})
	}

	var req dto.CreateNoteDTO
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Invalid request body", "error": err.Error(),
		})
	}
	if errs := utils.ValidateStruct(req); len(errs) > 0 {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Validation failed", "errors": errs,
		})
	}

	note, err := h.service.CreateNote(c.Context(), userID, lessonID, req)
	if err != nil {
		if status := noteErrorStatus(err); status != 0 {
			return c.Status(status).JSON(fiber.Map{"message": err.Error()})
		}
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Failed to create note", "error": err.Error(),
		})
	}

	return c.Status(fiber.StatusCreated).JSON(fiber.Map{
		"message": "Note created successfully", "data": note,
	})
}

// ListByLesson — GET /lessons/:lessonId/notes?sort=
func (h *NoteHandler) ListByLesson(c *fiber.Ctx) error {
	lessonID, err := uuid.Parse(c.Params("lessonId"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Invalid lesson ID", "error": err.Error(),
		})
	}
	userID, ok := h.userIDFromLocals(c)
	if !ok {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"message": "Unauthorized"})
	}

	notes, err := h.service.ListByLesson(c.Context(), userID, lessonID, c.Query("sort"))
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"message": "Failed to retrieve notes", "error": err.Error(),
		})
	}

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"message": "Notes retrieved successfully", "data": notes,
	})
}

// ListByCourse — GET /courses/:courseId/notes?section_id=&sort=
func (h *NoteHandler) ListByCourse(c *fiber.Ctx) error {
	courseID, err := uuid.Parse(c.Params("courseId"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Invalid course ID", "error": err.Error(),
		})
	}
	userID, ok := h.userIDFromLocals(c)
	if !ok {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"message": "Unauthorized"})
	}

	var sectionID *uuid.UUID
	if raw := c.Query("section_id"); raw != "" {
		parsed, err := uuid.Parse(raw)
		if err != nil {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
				"message": "Invalid section_id", "error": err.Error(),
			})
		}
		sectionID = &parsed
	}

	notes, err := h.service.ListByCourse(c.Context(), userID, courseID, sectionID, c.Query("sort"))
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"message": "Failed to retrieve notes", "error": err.Error(),
		})
	}

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"message": "Notes retrieved successfully", "data": notes,
	})
}

// UpdateNote — PUT /notes/:id
func (h *NoteHandler) UpdateNote(c *fiber.Ctx) error {
	noteID, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Invalid note ID", "error": err.Error(),
		})
	}
	userID, ok := h.userIDFromLocals(c)
	if !ok {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"message": "Unauthorized"})
	}

	var req dto.UpdateNoteDTO
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Invalid request body", "error": err.Error(),
		})
	}
	if errs := utils.ValidateStruct(req); len(errs) > 0 {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Validation failed", "errors": errs,
		})
	}

	note, err := h.service.UpdateNote(c.Context(), userID, noteID, req)
	if err != nil {
		if status := noteErrorStatus(err); status != 0 {
			return c.Status(status).JSON(fiber.Map{"message": err.Error()})
		}
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Failed to update note", "error": err.Error(),
		})
	}

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"message": "Note updated successfully", "data": note,
	})
}

// DeleteNote — DELETE /notes/:id
func (h *NoteHandler) DeleteNote(c *fiber.Ctx) error {
	noteID, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Invalid note ID", "error": err.Error(),
		})
	}
	userID, ok := h.userIDFromLocals(c)
	if !ok {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"message": "Unauthorized"})
	}

	if err := h.service.DeleteNote(c.Context(), userID, noteID); err != nil {
		if status := noteErrorStatus(err); status != 0 {
			return c.Status(status).JSON(fiber.Map{"message": err.Error()})
		}
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Failed to delete note", "error": err.Error(),
		})
	}

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"message": "Note deleted successfully", "data": fiber.Map{},
	})
}
