package handler

import (
	"strconv"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"study.com/v1/internal/dto"
	"study.com/v1/internal/middleware"
	"study.com/v1/internal/service"
	"study.com/v1/internal/utils"
)

type CourseHandler struct {
	service     service.CourseServiceInterface
	permChecker *middleware.PermissionChecker
}

func NewCourseHandler(service service.CourseServiceInterface, permChecker *middleware.PermissionChecker) *CourseHandler {
	return &CourseHandler{service: service, permChecker: permChecker}
}

func (h *CourseHandler) CreateCourse(c *fiber.Ctx) error {
	var req dto.CreateCourseDTO
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Invalid request body",
			"error":   err.Error(),
		})
	}

	// Override instructor_id with authenticated user's ID (don't trust client)
	userID, ok := c.Locals("user_id").(uuid.UUID)
	if !ok {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"message": "Unauthorized",
		})
	}
	req.InstructorID = userID

	// Phase 3: trước đây route POST /courses chỉ có AuthMiddleware — BẤT KỲ ai đăng nhập (học
	// viên, ứng viên giảng viên chưa được duyệt) cũng tạo được khoá học. Giờ bắt buộc
	// COURSES_CREATE (TEACHER; SYSTEM_ADMIN qua "*"). Kiểm ở handler (đã giữ permChecker) thay vì
	// router để không phải đổi chữ ký SetupCourseRoutes dùng chung.
	// permChecker nil = lỗi wiring -> từ chối (fail-closed), không lặng lẽ cho qua.
	if h.permChecker == nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"message": "Permission checker not configured",
		})
	}
	var activeOrgID *uuid.UUID
	if orgID, ok := c.Locals("active_org_id").(uuid.UUID); ok {
		activeOrgID = &orgID
	}
	allowed, err := h.permChecker.HasPermission(c.Context(), userID, activeOrgID, "COURSES_CREATE")
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"message": "Failed to resolve permissions",
		})
	}
	if !allowed {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{
			"success": false,
			"message": "forbidden: missing permission COURSES_CREATE",
		})
	}

	if errors := utils.ValidateStruct(req); len(errors) > 0 {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Validation failed",
			"errors":  errors,
		})
	}

	course, err := h.service.CreateCourse(c.Context(), req)
	if err != nil {
		if writeDiscountPriceInvalid(c, err) {
			return nil
		}
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Failed to create course",
			"error":   err.Error(),
		})
	}

	return c.Status(fiber.StatusCreated).JSON(fiber.Map{
		"message": "Course created successfully",
		"data":    course,
	})
}

// GetAllCourses handles GET /courses — route CÔNG KHAI, không auth (course_router.go), nên
// KHÔNG có cách phân biệt người gọi là ai. Vì vậy status LUÔN bị ép "published", bỏ qua bất kỳ
// giá trị status nào client gửi lên — trước bản vá này (P1 QA 260927 teacher) client truyền
// thẳng status rỗng/tuỳ ý và repository không lọc gì, nên khoá draft của MỌI giảng viên lộ ra
// trang /courses công khai. Giáo viên xem khoá (kể cả draft) của chính mình dùng GetMyCourses.
func (h *CourseHandler) GetAllCourses(c *fiber.Ctx) error {
	// Review đối kháng PR #70 (MAJOR): trước khi có route `/courses/mine` (mới), web gọi
	// `GET /courses?mine=true` — tham số `mine` này route công khai KHÔNG BAO GIỜ đọc (route
	// không có auth middleware nên không có user_id để lọc theo). Nếu chỉ âm thầm bỏ qua như
	// trước, sau khi PR này ép status="published" thì "Khoá của tôi" phía giáo viên (nếu web
	// merge sau, còn gọi route cũ) sẽ mất luôn khả năng thấy draft CỦA CHÍNH MÌNH mà không có
	// bất kỳ thông báo nào (200 OK, danh sách rỗng/chỉ published) — mất dữ liệu im lặng. Trả lỗi
	// rõ ràng thay vì âm thầm hạ cấp kết quả; client thật (web) đã đổi sang gọi `/courses/mine`
	// (route có auth, lọc đúng instructor_id) trong PR web đi kèm.
	if c.Query("mine") == "true" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "GET /courses?mine=true khong con duoc ho tro — goi GET /courses/mine (co xac thuc) thay the",
			"error":   "deprecated_query_param",
		})
	}

	params := dto.CourseFilterParams{
		Level:    c.Query("level"),
		Status:   "published",
		Keyword:  c.Query("keyword"),
		Page:     c.QueryInt("page", 1),
		PageSize: c.QueryInt("page_size", 20),
	}

	if catID := c.Query("category_id"); catID != "" {
		id, err := uuid.Parse(catID)
		if err == nil {
			params.CategoryID = &id
		}
	}

	if c.Query("is_free") == "true" {
		isFree := true
		params.IsFree = &isFree
	} else if c.Query("is_free") == "false" {
		isFree := false
		params.IsFree = &isFree
	}

	if minStr := c.Query("min_price"); minStr != "" {
		if v, err := strconv.ParseFloat(minStr, 64); err == nil {
			params.MinPrice = &v
		}
	}
	if maxStr := c.Query("max_price"); maxStr != "" {
		if v, err := strconv.ParseFloat(maxStr, 64); err == nil {
			params.MaxPrice = &v
		}
	}

	courses, err := h.service.GetAllCourses(c.Context(), params)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"message": "Failed to retrieve courses",
			"error":   err.Error(),
		})
	}

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"message": "Courses retrieved successfully",
		"data":    courses,
	})
}

// GetMyCourses handles GET /courses/mine — route CÓ auth (course_router.go), trả khoá của
// CHÍNH giáo viên đang đăng nhập (mọi status: draft/published/archived), khác GetAllCourses
// (công khai, luôn ép published). P1 QA 260927 teacher: web trước đây gọi GET /courses?mine=true
// nhưng handler cũ không đọc "mine" hay "instructor_id" từ query nên trả TOÀN BỘ khoá của MỌI
// giảng viên — trang "Khóa học của tôi" trộn lẫn khoá người khác dù ghi (write) đã bị chặn đúng.
func (h *CourseHandler) GetMyCourses(c *fiber.Ctx) error {
	userID, ok := c.Locals("user_id").(uuid.UUID)
	if !ok {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"message": "Unauthorized",
		})
	}

	params := dto.CourseFilterParams{
		InstructorID: &userID,
		Status:       c.Query("status"),
		Keyword:      c.Query("keyword"),
		Page:         c.QueryInt("page", 1),
		PageSize:     c.QueryInt("page_size", 20),
	}

	courses, err := h.service.GetAllCourses(c.Context(), params)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"message": "Failed to retrieve courses",
			"error":   err.Error(),
		})
	}

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"message": "Courses retrieved successfully",
		"data":    courses,
	})
}

func (h *CourseHandler) GetCourseByID(c *fiber.Ctx) error {
	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		// QA T13: id không phải UUID thì không thể trỏ tới khoá nào => 404 như id hợp lệ nhưng không có.
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"message": "Course not found",
			"error":   err.Error(),
		})
	}

	// C-1 (review vòng 2): route nằm sau middleware.AuthMiddleware (course_router.go) nên user_id
	// luôn có mặt trên đường thật — cần để service tính locked/lock_reason/progress đúng người
	// đang xem, thay vì trả contents (video_url) cho bất kỳ ai đã đăng nhập.
	userID, ok := c.Locals("user_id").(uuid.UUID)
	if !ok {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"message": "Unauthorized",
		})
	}
	isAdmin := isAdminActor(c, h.permChecker, userID)

	course, err := h.service.GetCourseByID(c.Context(), id, userID, isAdmin)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"message": "Course not found",
			"error":   err.Error(),
		})
	}

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"message": "Course retrieved successfully",
		"data":    course,
	})
}

// GetCourseBySlug handles GET /courses/slug/:slug
func (h *CourseHandler) GetCourseBySlug(c *fiber.Ctx) error {
	slug := c.Params("slug")
	if slug == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Slug is required",
		})
	}

	course, err := h.service.GetCourseBySlug(c.Context(), slug)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"message": "Course not found",
			"error":   err.Error(),
		})
	}

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"message": "Course retrieved successfully",
		"data":    course,
	})
}

func (h *CourseHandler) UpdateCourse(c *fiber.Ctx) error {
	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Invalid course ID",
			"error":   err.Error(),
		})
	}

	// C-12: chỉ giảng viên sở hữu khóa học (hoặc sẽ được service từ chối) mới sửa được.
	userID, ok := c.Locals("user_id").(uuid.UUID)
	if !ok {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"message": "Unauthorized",
		})
	}

	var req dto.UpdateCourseDTO
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

	isAdmin := isAdminActor(c, h.permChecker, userID)
	course, err := h.service.UpdateCourse(c.Context(), id, userID, isAdmin, req)
	if err != nil {
		if writeCourseLocked(c, err) || writeDiscountPriceInvalid(c, err) {
			return nil
		}
		if err == service.ErrNotCourseOwner {
			return c.Status(fiber.StatusForbidden).JSON(fiber.Map{
				"message": "You are not the instructor of this course",
			})
		}
		if err == service.ErrCourseStatusChangeNotAllowed {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
				"message": "Course status cannot be changed directly — use submit-review / admin approval",
				"code":    "COURSE_STATUS_CHANGE_NOT_ALLOWED",
			})
		}
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Failed to update course",
			"error":   err.Error(),
		})
	}

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"message": "Course updated successfully",
		"data":    course,
	})
}

func (h *CourseHandler) DeleteCourse(c *fiber.Ctx) error {
	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Invalid course ID",
			"error":   err.Error(),
		})
	}

	// C-12: chỉ giảng viên sở hữu khóa học mới xóa được.
	userID, ok := c.Locals("user_id").(uuid.UUID)
	if !ok {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"message": "Unauthorized",
		})
	}

	isAdmin := isAdminActor(c, h.permChecker, userID)
	if err := h.service.DeleteCourse(c.Context(), id, userID, isAdmin); err != nil {
		if writeCourseLocked(c, err) {
			return nil
		}
		if err == service.ErrNotCourseOwner {
			return c.Status(fiber.StatusForbidden).JSON(fiber.Map{
				"message": "You are not the instructor of this course",
			})
		}
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Failed to delete course",
			"error":   err.Error(),
		})
	}

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"message": "Course deleted successfully",
	})
}
