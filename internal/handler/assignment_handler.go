package handler

import (
	"errors"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"study.com/v1/internal/dto"
	"study.com/v1/internal/middleware"
	"study.com/v1/internal/service"
)

type AssignmentHandlerInterface interface {
	Create(c *fiber.Ctx) error
	GetByID(c *fiber.Ctx) error
	GetBySession(c *fiber.Ctx) error
	GetByClass(c *fiber.Ctx) error
	Update(c *fiber.Ctx) error
	Delete(c *fiber.Ctx) error
	Publish(c *fiber.Ctx) error
	Unpublish(c *fiber.Ctx) error
	AddTestCase(c *fiber.Ctx) error
	DeleteTestCase(c *fiber.Ctx) error
	ImportTestCases(c *fiber.Ctx) error
	GetTestCases(c *fiber.Ctx) error
	GetSandbox(c *fiber.Ctx) error
}

type AssignmentHandler struct {
	svc         service.AssignmentServiceInterface
	livekitSvc  service.LivekitServiceInterface
	permChecker *middleware.PermissionChecker
}

func NewAssignmentHandler(svc service.AssignmentServiceInterface, livekitSvc service.LivekitServiceInterface, permChecker *middleware.PermissionChecker) *AssignmentHandler {
	return &AssignmentHandler{svc: svc, livekitSvc: livekitSvc, permChecker: permChecker}
}

// canSeeHiddenTests (S2): trả true khi người gọi là chủ assignment hoặc admin. Trước đây
// ?include_hidden=true do CLIENT tự bật nên học viên nào cũng đọc được input/expected_output của
// test case ẩn. Lỗi tra cứu quyền coi như KHÔNG có quyền (fail-closed).
func (h *AssignmentHandler) canSeeHiddenTests(c *fiber.Ctx, assignmentID uuid.UUID) bool {
	userID, ok := c.Locals("user_id").(uuid.UUID)
	if !ok {
		return false
	}
	allowed, err := h.svc.CanManage(c.Context(), assignmentID, userID, isAdminActor(c, h.permChecker, userID))
	return err == nil && allowed
}

// requireAssignmentManager chặn thao tác GHI (sửa/xoá/publish/test case) với người không phải chủ
// assignment/admin. Trả (true, nil) khi được phép; ngược lại đã ghi response và trả (false, err)
// để caller return. Người không xem được assignment (chưa vào lớp/phiên, hoặc bản nháp của người
// khác, hoặc id không tồn tại) nhận 404 giống hệt nhau để không dò được id; người xem được mà
// không phải chủ nhận 403 (cùng quy ước với quiz và exercise sau S2).
func (h *AssignmentHandler) requireAssignmentManager(c *fiber.Ctx, assignmentID uuid.UUID) (bool, error) {
	userID, ok := c.Locals("user_id").(uuid.UUID)
	if !ok {
		return false, c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "unauthorized"})
	}
	isAdmin := isAdminActor(c, h.permChecker, userID)
	manage, err := h.svc.CanManage(c.Context(), assignmentID, userID, isAdmin)
	if err != nil {
		return false, c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
	if manage {
		return true, nil
	}
	view, err := h.svc.CanView(c.Context(), assignmentID, userID, isAdmin)
	if err != nil {
		return false, c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
	if !view {
		return false, c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "assignment not found"})
	}
	return false, c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": service.ErrAssignmentForbidden.Error()})
}

// requireAssignmentViewer chặn ĐỌC assignment với người không xem được: trả 404 (xem ở trên).
func (h *AssignmentHandler) requireAssignmentViewer(c *fiber.Ctx, assignmentID uuid.UUID) (bool, error) {
	userID, ok := c.Locals("user_id").(uuid.UUID)
	if !ok {
		return false, c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "unauthorized"})
	}
	view, err := h.svc.CanView(c.Context(), assignmentID, userID, isAdminActor(c, h.permChecker, userID))
	if err != nil {
		return false, c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
	if !view {
		return false, c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "assignment not found"})
	}
	return true, nil
}

func (h *AssignmentHandler) Create(c *fiber.Ctx) error {
	var req dto.CreateAssignmentDTO
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}

	userID, ok := c.Locals("user_id").(uuid.UUID)
	if !ok {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "unauthorized"})
	}

	assignment, err := h.svc.Create(c.Context(), userID, isAdminActor(c, h.permChecker, userID), req)
	if err != nil {
		switch {
		case errors.Is(err, service.ErrAssignmentForbidden):
			return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": err.Error()})
		case errors.Is(err, service.ErrAssignmentTargetRequired):
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
		}
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}

	return c.Status(fiber.StatusCreated).JSON(fiber.Map{
		"message": "Assignment created",
		"data":    assignment,
	})
}

func (h *AssignmentHandler) GetByID(c *fiber.Ctx) error {
	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid id"})
	}

	// S3: người không xem được assignment (chưa vào lớp/phiên, bản nháp của người khác) nhận 404.
	if ok, err := h.requireAssignmentViewer(c, id); !ok {
		return err
	}

	// S2: include_hidden chỉ có hiệu lực với chủ assignment/admin; người khác bị hạ về false
	// (vẫn nhận bài, chỉ không có test case ẩn) thay vì báo lỗi để không lộ sự tồn tại của test ẩn.
	includeHidden := c.QueryBool("include_hidden", false) && h.canSeeHiddenTests(c, id)

	assignment, err := h.svc.GetByID(c.Context(), id, includeHidden)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
	if assignment == nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "assignment not found"})
	}

	return c.JSON(fiber.Map{"data": assignment})
}

func (h *AssignmentHandler) GetBySession(c *fiber.Ctx) error {
	sessionIDStr := c.Query("session_id")
	if sessionIDStr == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "session_id is required"})
	}
	sessionID, err := uuid.Parse(sessionIDStr)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid session_id"})
	}

	page := c.QueryInt("page", 1)
	pageSize := c.QueryInt("page_size", 20)

	userID, ok := c.Locals("user_id").(uuid.UUID)
	if !ok {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "unauthorized"})
	}

	result, err := h.svc.GetBySession(c.Context(), userID, isAdminActor(c, h.permChecker, userID), sessionID, page, pageSize)
	if err != nil {
		if errors.Is(err, service.ErrAssignmentNotFound) {
			return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "session not found"})
		}
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(result)
}

// GetByClass: GET /api/classes/:classId/assignments. Cùng envelope với GET /assignments?session_id=.
// Lớp không xem được (hoặc không tồn tại) là 404, không lộ lớp có tồn tại hay không.
func (h *AssignmentHandler) GetByClass(c *fiber.Ctx) error {
	classID, err := uuid.Parse(c.Params("classId"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid class id"})
	}
	userID, ok := c.Locals("user_id").(uuid.UUID)
	if !ok {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "unauthorized"})
	}

	result, err := h.svc.GetByClass(c.Context(), userID, isAdminActor(c, h.permChecker, userID), classID, c.QueryInt("page", 1), c.QueryInt("page_size", 20))
	if err != nil {
		if errors.Is(err, service.ErrClassNotFound) {
			return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "class not found"})
		}
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(result)
}

func (h *AssignmentHandler) Update(c *fiber.Ctx) error {
	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid id"})
	}

	if ok, err := h.requireAssignmentManager(c, id); !ok {
		return err
	}

	var req dto.UpdateAssignmentDTO
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}

	assignment, err := h.svc.Update(c.Context(), id, req)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(fiber.Map{"message": "Assignment updated", "data": assignment})
}

func (h *AssignmentHandler) Delete(c *fiber.Ctx) error {
	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid id"})
	}

	if ok, err := h.requireAssignmentManager(c, id); !ok {
		return err
	}

	if err := h.svc.Delete(c.Context(), id); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(fiber.Map{"message": "Assignment deleted"})
}

func (h *AssignmentHandler) Publish(c *fiber.Ctx) error {
	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid id"})
	}

	if ok, err := h.requireAssignmentManager(c, id); !ok {
		return err
	}

	assignment, err := h.svc.Publish(c.Context(), id, h.livekitSvc)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(fiber.Map{"message": "Assignment published", "data": assignment})
}

func (h *AssignmentHandler) Unpublish(c *fiber.Ctx) error {
	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid id"})
	}

	if ok, err := h.requireAssignmentManager(c, id); !ok {
		return err
	}

	assignment, err := h.svc.Unpublish(c.Context(), id)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(fiber.Map{"message": "Assignment unpublished", "data": assignment})
}

func (h *AssignmentHandler) AddTestCase(c *fiber.Ctx) error {
	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid id"})
	}

	if ok, err := h.requireAssignmentManager(c, id); !ok {
		return err
	}

	var req dto.CreateTestCaseDTO
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}

	testCase, err := h.svc.AddTestCase(c.Context(), id, req)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}

	return c.Status(fiber.StatusCreated).JSON(fiber.Map{
		"message": "Test case added",
		"data":    testCase,
	})
}

func (h *AssignmentHandler) DeleteTestCase(c *fiber.Ctx) error {
	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid id"})
	}
	tcID, err := uuid.Parse(c.Params("tcId"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid test case id"})
	}
	if ok, err := h.requireAssignmentManager(c, id); !ok {
		return err
	}

	if err := h.svc.DeleteTestCase(c.Context(), id, tcID); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(fiber.Map{"message": "Test case deleted"})
}

func (h *AssignmentHandler) ImportTestCases(c *fiber.Ctx) error {
	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid id"})
	}

	if ok, err := h.requireAssignmentManager(c, id); !ok {
		return err
	}

	var req dto.ImportTestCasesDTO
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}

	testCases, err := h.svc.ImportTestCases(c.Context(), id, req)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}

	return c.Status(fiber.StatusCreated).JSON(fiber.Map{
		"message": "Test cases imported",
		"count":   len(testCases),
		"data":    testCases,
	})
}

func (h *AssignmentHandler) GetTestCases(c *fiber.Ctx) error {
	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid id"})
	}

	if ok, err := h.requireAssignmentViewer(c, id); !ok {
		return err
	}

	includeHidden := c.QueryBool("include_hidden", false) && h.canSeeHiddenTests(c, id)

	testCases, err := h.svc.GetTestCases(c.Context(), id, includeHidden)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(fiber.Map{"data": testCases})
}

func (h *AssignmentHandler) GetSandbox(c *fiber.Ctx) error {
	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid id"})
	}

	userIDVal := c.Locals("user_id")
	if userIDVal == nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "unauthorized"})
	}
	userID := userIDVal.(uuid.UUID)

	if ok, err := h.requireAssignmentViewer(c, id); !ok {
		return err
	}

	result, err := h.svc.GetSandbox(c.Context(), id, userID)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(result)
}
