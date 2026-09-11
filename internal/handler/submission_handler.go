package handler

import (
	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"study.com/v1/internal/dto"
	"study.com/v1/internal/middleware"
	"study.com/v1/internal/service"
	"study.com/v1/internal/utils"
)

type SubmissionHandlerInterface interface {
	Submit(c *fiber.Ctx) error
	GetByID(c *fiber.Ctx) error
	GetByAssignment(c *fiber.Ctx) error
	GetByUser(c *fiber.Ctx) error
	RunCode(c *fiber.Ctx) error
	RunCustomCode(c *fiber.Ctx) error
	ExecuteCode(c *fiber.Ctx) error
	GetMySubmissions(c *fiber.Ctx) error
}

type SubmissionHandler struct {
	svc         service.SubmissionServiceInterface
	permChecker *middleware.PermissionChecker
}

func NewSubmissionHandler(svc service.SubmissionServiceInterface, permChecker *middleware.PermissionChecker) *SubmissionHandler {
	return &SubmissionHandler{svc: svc, permChecker: permChecker}
}

// submissionErrorStatus ánh xạ ErrSubmissionForbidden (C-10) sang 403; trả 0 khi không nhận
// diện được để caller giữ nguyên nhánh xử lý lỗi hiện có.
func submissionErrorStatus(err error) int {
	if err == service.ErrSubmissionForbidden {
		return fiber.StatusForbidden
	}
	return 0
}

func (h *SubmissionHandler) Submit(c *fiber.Ctx) error {
	var req dto.CreateSubmissionDTO
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}

	// C-11 (audit 260909): trước đây UserID lấy nguyên từ body -> học sinh A nộp bài dưới
	// tên B được. Ép UserID = user đang đăng nhập, không tin dữ liệu client gửi lên.
	userID, ok := c.Locals("user_id").(uuid.UUID)
	if !ok {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "unauthorized"})
	}
	req.UserID = userID.String()

	if errs := utils.ValidateStruct(req); len(errs) > 0 {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "validation failed", "errors": errs})
	}

	submission, err := h.svc.Submit(c.Context(), req)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}

	return c.Status(fiber.StatusCreated).JSON(fiber.Map{
		"message": "Submission created, processing...",
		"data":    submission,
	})
}

func (h *SubmissionHandler) GetByID(c *fiber.Ctx) error {
	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid id"})
	}

	// C-10: chỉ chủ bài nộp hoặc giáo viên buổi học liên quan mới xem được (kể cả điểm).
	requesterID, ok := c.Locals("user_id").(uuid.UUID)
	if !ok {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "unauthorized"})
	}

	submission, err := h.svc.GetByID(c.Context(), id, requesterID)
	if err != nil {
		if status := submissionErrorStatus(err); status != 0 {
			return c.Status(status).JSON(fiber.Map{"error": err.Error()})
		}
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
	if submission == nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "submission not found"})
	}

	return c.JSON(fiber.Map{"data": submission})
}

// GetByAssignment (H-03): chỉ giáo viên sở hữu assignment (session hoặc class) hoặc admin
// mới xem được toàn bộ bài nộp của một assignment — kiểm tra thực hiện ở service layer.
func (h *SubmissionHandler) GetByAssignment(c *fiber.Ctx) error {
	assignmentID, err := uuid.Parse(c.Params("assignmentId"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid assignment_id"})
	}

	requesterID, ok := c.Locals("user_id").(uuid.UUID)
	if !ok {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "unauthorized"})
	}
	isAdmin := isAdminActor(c, h.permChecker, requesterID)

	page := c.QueryInt("page", 1)
	pageSize := c.QueryInt("page_size", 20)

	result, err := h.svc.GetByAssignment(c.Context(), assignmentID, requesterID, isAdmin, page, pageSize)
	if err != nil {
		if status := submissionErrorStatus(err); status != 0 {
			return c.Status(status).JSON(fiber.Map{"error": err.Error()})
		}
		status := fiber.StatusInternalServerError
		if err.Error() == "assignment not found" {
			status = fiber.StatusNotFound
		}
		return c.Status(status).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(result)
}

func (h *SubmissionHandler) GetByUser(c *fiber.Ctx) error {
	userID, err := uuid.Parse(c.Params("userId"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid user_id"})
	}

	// C-10: endpoint chỉ liệt kê được bài nộp của chính người gọi.
	requesterID, ok := c.Locals("user_id").(uuid.UUID)
	if !ok {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "unauthorized"})
	}

	page := c.QueryInt("page", 1)
	pageSize := c.QueryInt("page_size", 20)

	result, err := h.svc.GetByUser(c.Context(), userID, requesterID, page, pageSize)
	if err != nil {
		if status := submissionErrorStatus(err); status != 0 {
			return c.Status(status).JSON(fiber.Map{"error": err.Error()})
		}
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(result)
}

func (h *SubmissionHandler) RunCode(c *fiber.Ctx) error {
	var req dto.RunCodeDTO
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}

	result, err := h.svc.RunCode(c.Context(), req)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(result)
}

func (h *SubmissionHandler) RunCustomCode(c *fiber.Ctx) error {
	var req dto.RunCustomCodeDTO
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}

	result, err := h.svc.RunCustomCode(c.Context(), req)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(result)
}

// ExecuteCode - Free sandbox execution without assignment
func (h *SubmissionHandler) ExecuteCode(c *fiber.Ctx) error {
	var req dto.ExecuteCodeDTO
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}

	result, err := h.svc.ExecuteCode(c.Context(), req)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(result)
}

func (h *SubmissionHandler) GetMySubmissions(c *fiber.Ctx) error {
	assignmentID, err := uuid.Parse(c.Params("assignmentId"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid assignment_id"})
	}

	// C-10 (audit 260909): tên hàm là "My" nhưng trước đây lấy user_id từ query string —
	// bất kỳ ai cũng liệt kê được bài nộp của người khác cho assignment này. Đổi sang lấy
	// đúng user đang đăng nhập.
	userID, ok := c.Locals("user_id").(uuid.UUID)
	if !ok {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "unauthorized"})
	}

	submissions, err := h.svc.GetUserSubmissionsForAssignment(c.Context(), assignmentID, userID)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(fiber.Map{"data": submissions})
}
