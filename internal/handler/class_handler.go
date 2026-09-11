package handler

import (
	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"study.com/v1/internal/dto"
	"study.com/v1/internal/middleware"
	"study.com/v1/internal/service"
)

type ClassHandlerInterface interface {
	CreateClass(c *fiber.Ctx) error
	GetAllClasses(c *fiber.Ctx) error
	GetMyClasses(c *fiber.Ctx) error
	GetClassByID(c *fiber.Ctx) error
	UpdateClass(c *fiber.Ctx) error
	DeleteClass(c *fiber.Ctx) error
	GetClassesByCourseID(c *fiber.Ctx) error
	CreateClassForCourse(c *fiber.Ctx) error
	AssignTeacherToClass(c *fiber.Ctx) error
	RemoveTeacherFromClass(c *fiber.Ctx) error
	GetTeachersByClass(c *fiber.Ctx) error
	EnrollStudentToClass(c *fiber.Ctx) error
	RemoveStudentFromClass(c *fiber.Ctx) error
	GetStudentsByClass(c *fiber.Ctx) error
}

type ClassHandler struct {
	service     service.ClassServiceInterface
	permChecker *middleware.PermissionChecker
}

func NewClassHandler(service service.ClassServiceInterface, permChecker *middleware.PermissionChecker) *ClassHandler {
	return &ClassHandler{service: service, permChecker: permChecker}
}

// classErrorStatus ánh xạ lỗi phân quyền (H-11) sang HTTP status phù hợp; trả 0 khi không
// nhận diện được (để caller giữ nguyên xử lý 400/500 hiện có) — cùng pattern gradeErrorStatus.
func classErrorStatus(err error) int {
	switch err {
	case service.ErrNotClassTeacher:
		return fiber.StatusForbidden
	default:
		return 0
	}
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

func (h *ClassHandler) CreateClassForCourse(c *fiber.Ctx) error {
	courseID, err := uuid.Parse(c.Params("course_id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Invalid course ID",
			"error":   err.Error(),
		})
	}

	var req dto.CreateClassDTO
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Invalid request body",
			"error":   err.Error(),
		})
	}
	req.CourseID = &courseID

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

func (h *ClassHandler) GetClassesByCourseID(c *fiber.Ctx) error {
	courseID, err := uuid.Parse(c.Params("course_id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Invalid course ID",
			"error":   err.Error(),
		})
	}

	classes, err := h.service.GetClassesByCourseID(c.Context(), courseID)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"message": "Failed to get classes",
			"error":   err.Error(),
		})
	}

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"message": "Classes retrieved successfully",
		"data":    classes,
	})
}

func (h *ClassHandler) GetMyClasses(c *fiber.Ctx) error {
	teacherID, ok := c.Locals("user_id").(uuid.UUID)
	if !ok || teacherID == uuid.Nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"message": "Unauthorized",
		})
	}

	classes, err := h.service.GetMyClasses(c.Context(), teacherID)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"message": "Failed to retrieve classes",
			"error":   err.Error(),
		})
	}

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"message": "Classes retrieved successfully",
		"data": fiber.Map{
			"classes": classes,
		},
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

	// H-11: chỉ giáo viên của lớp hoặc admin mới sửa được.
	actorUserID, ok := c.Locals("user_id").(uuid.UUID)
	if !ok {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"message": "Unauthorized",
		})
	}

	var req dto.UpdateClassDTO
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Invalid request body",
			"error":   err.Error(),
		})
	}

	isAdmin := isAdminActor(c, h.permChecker, actorUserID)
	class, err := h.service.UpdateClass(c.Context(), id, actorUserID, isAdmin, req)
	if err != nil {
		if status := classErrorStatus(err); status != 0 {
			return c.Status(status).JSON(fiber.Map{"message": err.Error()})
		}
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

	// H-11: chỉ giáo viên của lớp hoặc admin mới xóa được.
	actorUserID, ok := c.Locals("user_id").(uuid.UUID)
	if !ok {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"message": "Unauthorized",
		})
	}

	hardDelete := c.QueryBool("hard_delete", false)

	isAdmin := isAdminActor(c, h.permChecker, actorUserID)
	if err := h.service.DeleteClass(c.Context(), id, actorUserID, isAdmin, hardDelete); err != nil {
		if status := classErrorStatus(err); status != 0 {
			return c.Status(status).JSON(fiber.Map{"message": err.Error()})
		}
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

	// H-11: chỉ giáo viên của lớp hoặc admin mới thêm học sinh được.
	actorUserID, ok := c.Locals("user_id").(uuid.UUID)
	if !ok {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"message": "Unauthorized",
		})
	}

	var req dto.EnrollStudentDTO
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Invalid request body",
			"error":   err.Error(),
		})
	}

	isAdmin := isAdminActor(c, h.permChecker, actorUserID)
	sc, err := h.service.EnrollStudentToClass(c.Context(), classID, actorUserID, isAdmin, req)
	if err != nil {
		if status := classErrorStatus(err); status != 0 {
			return c.Status(status).JSON(fiber.Map{"message": err.Error()})
		}
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

	// H-11: chỉ giáo viên của lớp hoặc admin mới xóa học sinh được.
	actorUserID, ok := c.Locals("user_id").(uuid.UUID)
	if !ok {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"message": "Unauthorized",
		})
	}

	isAdmin := isAdminActor(c, h.permChecker, actorUserID)
	if err := h.service.RemoveStudentFromClass(c.Context(), classID, studentID, actorUserID, isAdmin); err != nil {
		if status := classErrorStatus(err); status != 0 {
			return c.Status(status).JSON(fiber.Map{"message": err.Error()})
		}
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

	// H-11: danh sách học sinh (email/tên/avatar) chỉ cho giáo viên của lớp hoặc admin xem.
	actorUserID, ok := c.Locals("user_id").(uuid.UUID)
	if !ok {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"message": "Unauthorized",
		})
	}

	page := c.QueryInt("page", 1)
	pageSize := c.QueryInt("page_size", 20)

	isAdmin := isAdminActor(c, h.permChecker, actorUserID)
	students, err := h.service.GetStudentsByClass(c.Context(), classID, actorUserID, isAdmin, page, pageSize)
	if err != nil {
		if status := classErrorStatus(err); status != 0 {
			return c.Status(status).JSON(fiber.Map{"message": err.Error()})
		}
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
