package handler

import (
	"strconv"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"study.com/v1/internal/service"
)

type ProfileHandlerInterface interface {
	GetChildren(c *fiber.Ctx) error
	GetOrganizations(c *fiber.Ctx) error
}

type ProfileHandler struct {
	profileService service.ProfileServiceInterface
}

func NewProfileHandler(profileService service.ProfileServiceInterface) *ProfileHandler {
	return &ProfileHandler{profileService: profileService}
}

// GetChildren lấy danh sách con của phụ huynh
// @Summary Lấy danh sách con của phụ huynh
// @Description Trả về danh sách học sinh là con của phụ huynh hiện tại. Cần có role PARENT.
// @Tags Profile
// @Security BearerAuth
// @Accept json
// @Produce json
// @Param page query int false "Page number" default(1)
// @Param pageSize query int false "Page size" default(20) maximum(100)
// @Success 200 {object} dto.PaginatedChildrenResponse "Danh sách con"
// @Failure 401 {object} map[string]interface{} "Unauthorized"
// @Failure 500 {object} map[string]interface{} "Internal Server Error"
// @Router /profile/children [get]
func (h *ProfileHandler) GetChildren(c *fiber.Ctx) error {
	userID, ok := c.Locals("user_id").(uuid.UUID)
	if !ok || userID == uuid.Nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"message": "Unauthorized",
		})
	}

	page, _ := strconv.Atoi(c.Query("page", "1"))
	pageSize, _ := strconv.Atoi(c.Query("pageSize", "20"))

	result, err := h.profileService.GetChildren(c.Context(), userID, page, pageSize)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"message": "Failed to get children",
			"error":   err.Error(),
		})
	}

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"message": "Success",
		"data":    result,
	})
}

// GetOrganizations lấy danh sách tổ chức của user
// @Summary Lấy danh sách tổ chức của user
// @Description Trả về danh sách các tổ chức mà user hiện tại là thành viên
// @Tags Profile
// @Security BearerAuth
// @Accept json
// @Produce json
// @Param page query int false "Page number" default(1)
// @Param pageSize query int false "Page size" default(20) maximum(100)
// @Success 200 {object} dto.PaginatedOrganizationsResponse "Danh sách tổ chức"
// @Failure 401 {object} map[string]interface{} "Unauthorized"
// @Failure 500 {object} map[string]interface{} "Internal Server Error"
// @Router /profile/organizations [get]
func (h *ProfileHandler) GetOrganizations(c *fiber.Ctx) error {
	userID, ok := c.Locals("user_id").(uuid.UUID)
	if !ok || userID == uuid.Nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"message": "Unauthorized",
		})
	}

	page, _ := strconv.Atoi(c.Query("page", "1"))
	pageSize, _ := strconv.Atoi(c.Query("pageSize", "20"))

	result, err := h.profileService.GetOrganizations(c.Context(), userID, page, pageSize)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"message": "Failed to get organizations",
			"error":   err.Error(),
		})
	}

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"message": "Success",
		"data":    result,
	})
}
