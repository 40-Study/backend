package handler

import (
	"errors"

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
	switch {
	case errors.Is(err, service.ErrClassNotFound):
		// S4: lớp không tồn tại hoặc người gọi không xem được lớp — không phân biệt để không dò được id.
		return fiber.StatusNotFound
	case errors.Is(err, service.ErrNotClassTeacher),
		errors.Is(err, service.ErrNotClassOwner),
		errors.Is(err, service.ErrNotCourseInstructor),
		errors.Is(err, service.ErrNotOrgMember),
		errors.Is(err, service.ErrNotTeacher),
		errors.Is(err, service.ErrClassDeleteAdminOnly):
		return fiber.StatusForbidden
	case errors.Is(err, service.ErrTeacherNotOrgMember):
		// 400 (không 422): web (api-client) bỏ code/message của 422 và chỉ hiện "dữ liệu không hợp lệ" chung.
		return fiber.StatusBadRequest
	default:
		return 0
	}
}

// classErrorBody: thân JSON của lỗi đã ánh xạ. Hai lỗi nghiệp vụ mới mang mã ổn định để web hiện câu tiếng Việt
// mà không so khớp chuỗi tiếng Anh (cùng kiểu ROLE_IN_USE ở role_handler).
func classErrorBody(err error) fiber.Map {
	body := fiber.Map{"message": err.Error()}
	switch {
	case errors.Is(err, service.ErrTeacherNotOrgMember):
		body["code"] = "TEACHER_NOT_ORG_MEMBER"
	case errors.Is(err, service.ErrClassDeleteAdminOnly):
		body["code"] = "CLASS_DELETE_ADMIN_ONLY"
	}
	return body
}

func (h *ClassHandler) CreateClass(c *fiber.Ctx) error {
	var req dto.CreateClassDTO
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Invalid request body",
			"error":   err.Error(),
		})
	}

	actorUserID, ok := c.Locals("user_id").(uuid.UUID)
	if !ok {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"message": "Unauthorized"})
	}

	class, err := h.service.CreateClass(c.Context(), actorUserID, isAdminActor(c, h.permChecker, actorUserID), req)
	if err != nil {
		if status := classErrorStatus(err); status != 0 {
			return c.Status(status).JSON(classErrorBody(err))
		}
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

	actorUserID, ok := c.Locals("user_id").(uuid.UUID)
	if !ok {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"message": "Unauthorized"})
	}

	class, err := h.service.CreateClass(c.Context(), actorUserID, isAdminActor(c, h.permChecker, actorUserID), req)
	if err != nil {
		if status := classErrorStatus(err); status != 0 {
			return c.Status(status).JSON(classErrorBody(err))
		}
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

	actorUserID, ok := c.Locals("user_id").(uuid.UUID)
	if !ok {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"message": "Unauthorized"})
	}
	classes, err := h.service.GetAllClasses(c.Context(), actorUserID, isAdminActor(c, h.permChecker, actorUserID), page, pageSize, keyword, status)
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

// GetOrganizationClasses: GET /organizations/:organization_id/classes. Router đã đòi ORG_MEMBERS_MANAGE trên đúng tổ chức
// (RequireOrgPermission), nên ở đây chỉ phân tích tham số.
func (h *ClassHandler) GetOrganizationClasses(c *fiber.Ctx) error {
	orgID, err := uuid.Parse(c.Params("organization_id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"message": "Invalid organization ID"})
	}
	classes, err := h.service.GetOrganizationClasses(c.Context(), orgID, c.QueryInt("page", 1), c.QueryInt("page_size", 20), c.Query("keyword"), c.Query("status"))
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"message": "Failed to retrieve organization classes",
			"error":   err.Error(),
		})
	}
	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"message": "Classes retrieved successfully",
		"data":    classes,
	})
}

// SearchEnrollableStudents: GET /classes/:id/enrollable-students?keyword= — ô chọn học viên để ghi danh (B-12).
func (h *ClassHandler) SearchEnrollableStudents(c *fiber.Ctx) error {
	classID, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"message": "Invalid class ID"})
	}
	actorUserID, ok := c.Locals("user_id").(uuid.UUID)
	if !ok {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"message": "Unauthorized"})
	}
	students, err := h.service.SearchEnrollableStudents(c.Context(), classID, actorUserID, isAdminActor(c, h.permChecker, actorUserID), c.Query("keyword"))
	if err != nil {
		status := fiber.StatusInternalServerError
		switch {
		case errors.Is(err, service.ErrClassNotFound):
			status = fiber.StatusNotFound
		case errors.Is(err, service.ErrNotClassTeacher):
			status = fiber.StatusForbidden
		}
		return c.Status(status).JSON(fiber.Map{"message": "Failed to search students", "error": err.Error()})
	}
	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"message": "Students retrieved successfully",
		"data":    fiber.Map{"students": students},
	})
}

// SearchAssignableTeachers: GET /classes/:id/assignable-teachers?keyword= — ô chọn giảng viên để gán vào lớp (W2-A).
func (h *ClassHandler) SearchAssignableTeachers(c *fiber.Ctx) error {
	classID, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"message": "Invalid class ID"})
	}
	actorUserID, ok := c.Locals("user_id").(uuid.UUID)
	if !ok {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"message": "Unauthorized"})
	}
	teachers, err := h.service.SearchAssignableTeachers(c.Context(), classID, actorUserID, isAdminActor(c, h.permChecker, actorUserID), c.Query("keyword"))
	if err != nil {
		if status := classErrorStatus(err); status != 0 {
			return c.Status(status).JSON(classErrorBody(err))
		}
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"message": "Failed to search teachers", "error": err.Error()})
	}
	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"message": "Teachers retrieved successfully",
		"data":    fiber.Map{"teachers": teachers},
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

	actorUserID, ok := c.Locals("user_id").(uuid.UUID)
	if !ok {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"message": "Unauthorized"})
	}

	class, err := h.service.GetClassByID(c.Context(), id, actorUserID, isAdminActor(c, h.permChecker, actorUserID))
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
			return c.Status(status).JSON(classErrorBody(err))
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
			return c.Status(status).JSON(classErrorBody(err))
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

	actorUserID, ok := c.Locals("user_id").(uuid.UUID)
	if !ok {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"message": "Unauthorized"})
	}

	tc, err := h.service.AssignTeacherToClass(c.Context(), classID, actorUserID, isAdminActor(c, h.permChecker, actorUserID), req)
	if err != nil {
		if status := classErrorStatus(err); status != 0 {
			return c.Status(status).JSON(classErrorBody(err))
		}
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

	actorUserID, ok := c.Locals("user_id").(uuid.UUID)
	if !ok {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"message": "Unauthorized"})
	}

	if err := h.service.RemoveTeacherFromClass(c.Context(), classID, teacherID, actorUserID, isAdminActor(c, h.permChecker, actorUserID)); err != nil {
		if status := classErrorStatus(err); status != 0 {
			return c.Status(status).JSON(classErrorBody(err))
		}
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

	actorUserID, ok := c.Locals("user_id").(uuid.UUID)
	if !ok {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"message": "Unauthorized"})
	}
	teachers, err := h.service.GetTeachersByClass(c.Context(), classID, actorUserID, isAdminActor(c, h.permChecker, actorUserID), page, pageSize)
	if err != nil {
		if status := classErrorStatus(err); status != 0 {
			return c.Status(status).JSON(classErrorBody(err))
		}
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
			return c.Status(status).JSON(classErrorBody(err))
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
			return c.Status(status).JSON(classErrorBody(err))
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
			return c.Status(status).JSON(classErrorBody(err))
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
