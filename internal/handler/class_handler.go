package handler

import (
	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"study.com/v1/internal/dto"
	"study.com/v1/internal/service"
)

type ClassHandlerInterface interface {
	CreateClass(c *fiber.Ctx) error
	GetAllClasses(c *fiber.Ctx) error
	GetClassByID(c *fiber.Ctx) error
	UpdateClass(c *fiber.Ctx) error
	DeleteClass(c *fiber.Ctx) error
	AssignTeacherToClass(c *fiber.Ctx) error
	RemoveTeacherFromClass(c *fiber.Ctx) error
	GetTeachersByClass(c *fiber.Ctx) error
	EnrollStudentToClass(c *fiber.Ctx) error
	RemoveStudentFromClass(c *fiber.Ctx) error
	GetStudentsByClass(c *fiber.Ctx) error
}

type ClassHandler struct {
	service service.ClassServiceInterface
}

func NewClassHandler(service service.ClassServiceInterface) *ClassHandler {
	return &ClassHandler{service: service}
}

func (h *ClassHandler) CreateClass(c *fiber.Ctx) error {
	var req dto.CreateClassDTO
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Invalid request body",
			"error":   err.Error(),
		})
	}

	class, err := h.service.CreateClass(c.Context(), req)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Failed to create class",
			"error":   err.Error(),
		})
	}

	return c.Status(fiber.StatusCreated).JSON(fiber.Map{
		"message": "Class created successfully",
		"data":    class,
	})
}

func (h *ClassHandler) GetAllClasses(c *fiber.Ctx) error {
	page := c.QueryInt("page", 1)
	pageSize := c.QueryInt("page_size", 20)
	keyword := c.Query("keyword")
	status := c.Query("status")

	classes, err := h.service.GetAllClasses(c.Context(), page, pageSize, keyword, status)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"message": "Failed to retrieve classes",
			"error":   err.Error(),
		})
	}

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"message": "Classes retrieved successfully",
		"data":    classes,
	})
}

func (h *ClassHandler) GetClassByID(c *fiber.Ctx) error {
	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Invalid class ID",
			"error":   err.Error(),
		})
	}

	class, err := h.service.GetClassByID(c.Context(), id)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"message": "Class not found",
			"error":   err.Error(),
		})
	}

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"message": "Class retrieved successfully",
		"data":    class,
	})
}

func (h *ClassHandler) UpdateClass(c *fiber.Ctx) error {
	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Invalid class ID",
			"error":   err.Error(),
		})
	}

	var req dto.UpdateClassDTO
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Invalid request body",
			"error":   err.Error(),
		})
	}

	class, err := h.service.UpdateClass(c.Context(), id, req)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Failed to update class",
			"error":   err.Error(),
		})
	}

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"message": "Class updated successfully",
		"data":    class,
	})
}

func (h *ClassHandler) DeleteClass(c *fiber.Ctx) error {
	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Invalid class ID",
			"error":   err.Error(),
		})
	}

	hardDelete := c.QueryBool("hard_delete", false)

	if err := h.service.DeleteClass(c.Context(), id, hardDelete); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Failed to delete class",
			"error":   err.Error(),
		})
	}

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"message": "Class deleted successfully",
	})
}

// Teacher-Class

func (h *ClassHandler) AssignTeacherToClass(c *fiber.Ctx) error {
	classID, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Invalid class ID",
			"error":   err.Error(),
		})
	}

	var req dto.AssignTeacherDTO
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Invalid request body",
			"error":   err.Error(),
		})
	}

	tc, err := h.service.AssignTeacherToClass(c.Context(), classID, req)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Failed to assign teacher",
			"error":   err.Error(),
		})
	}

	return c.Status(fiber.StatusCreated).JSON(fiber.Map{
		"message": "Teacher assigned successfully",
		"data":    tc,
	})
}

func (h *ClassHandler) RemoveTeacherFromClass(c *fiber.Ctx) error {
	classID, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Invalid class ID",
			"error":   err.Error(),
		})
	}

	teacherID, err := uuid.Parse(c.Params("teacherId"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Invalid teacher ID",
			"error":   err.Error(),
		})
	}

	if err := h.service.RemoveTeacherFromClass(c.Context(), classID, teacherID); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Failed to remove teacher",
			"error":   err.Error(),
		})
	}

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"message": "Teacher removed successfully",
	})
}

func (h *ClassHandler) GetTeachersByClass(c *fiber.Ctx) error {
	classID, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Invalid class ID",
			"error":   err.Error(),
		})
	}

	page := c.QueryInt("page", 1)
	pageSize := c.QueryInt("page_size", 20)

	teachers, err := h.service.GetTeachersByClass(c.Context(), classID, page, pageSize)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"message": "Failed to retrieve teachers",
			"error":   err.Error(),
		})
	}

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"message": "Teachers retrieved successfully",
		"data":    teachers,
	})
}

// Student-Class

func (h *ClassHandler) EnrollStudentToClass(c *fiber.Ctx) error {
	classID, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Invalid class ID",
			"error":   err.Error(),
		})
	}

	var req dto.EnrollStudentDTO
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Invalid request body",
			"error":   err.Error(),
		})
	}

	sc, err := h.service.EnrollStudentToClass(c.Context(), classID, req)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Failed to enroll student",
			"error":   err.Error(),
		})
	}

	return c.Status(fiber.StatusCreated).JSON(fiber.Map{
		"message": "Student enrolled successfully",
		"data":    sc,
	})
}

func (h *ClassHandler) RemoveStudentFromClass(c *fiber.Ctx) error {
	classID, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Invalid class ID",
			"error":   err.Error(),
		})
	}

	studentID, err := uuid.Parse(c.Params("studentId"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Invalid student ID",
			"error":   err.Error(),
		})
	}

	if err := h.service.RemoveStudentFromClass(c.Context(), classID, studentID); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Failed to remove student",
			"error":   err.Error(),
		})
	}

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"message": "Student removed successfully",
	})
}

func (h *ClassHandler) GetStudentsByClass(c *fiber.Ctx) error {
	classID, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Invalid class ID",
			"error":   err.Error(),
		})
	}

	page := c.QueryInt("page", 1)
	pageSize := c.QueryInt("page_size", 20)

	students, err := h.service.GetStudentsByClass(c.Context(), classID, page, pageSize)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"message": "Failed to retrieve students",
			"error":   err.Error(),
		})
	}

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"message": "Students retrieved successfully",
		"data":    students,
	})
}
