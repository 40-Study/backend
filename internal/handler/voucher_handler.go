package handler

import (
	"strconv"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"study.com/v1/internal/dto"
	"study.com/v1/internal/service"
	"study.com/v1/internal/utils"
)

type VoucherHandler struct {
	voucherService service.VoucherServiceInterface
}

func NewVoucherHandler(voucherService service.VoucherServiceInterface) *VoucherHandler {
	return &VoucherHandler{
		voucherService: voucherService,
	}
}

// CreateVoucher - POST /api/v1/vouchers
func (h *VoucherHandler) CreateVoucher(c *fiber.Ctx) error {
	var req dto.CreateVoucherRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"code":    "ERR_INVALID_REQUEST",
			"message": "Invalid request body",
		})
	}

	// M-01 (audit 260909 vòng 2): DTO đã có validate tag đầy đủ (required/oneof/min/max)
	// nhưng handler trước đây không gọi ValidateStruct — mọi tag đều vô tác dụng, user có
	// thể tạo voucher discount_percent > 100, usage_limit âm, v.v.
	if errs := utils.ValidateStruct(req); len(errs) > 0 {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"code":    "ERR_VALIDATION",
			"message": "Validation failed",
			"errors":  errs,
		})
	}

	voucher, err := h.voucherService.CreateVoucher(c.Context(), &req)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"code":    "ERR_CREATE_VOUCHER",
			"message": err.Error(),
		})
	}

	return c.Status(fiber.StatusCreated).JSON(voucher)
}

// GetVoucher - GET /api/v1/vouchers/:id
func (h *VoucherHandler) GetVoucher(c *fiber.Ctx) error {
	voucherID, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"code":    "ERR_INVALID_ID",
			"message": "Invalid voucher ID",
		})
	}

	voucher, err := h.voucherService.GetVoucherByID(c.Context(), voucherID)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"code":    "ERR_NOT_FOUND",
			"message": "Voucher not found",
		})
	}

	return c.JSON(voucher)
}

func (h *VoucherHandler) GetVoucherByCode(c *fiber.Ctx) error {
	code := c.Params("code")
	if code == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"code":    "ERR_MISSING_CODE",
			"message": "Voucher code is required",
		})
	}

	voucher, err := h.voucherService.GetVoucherByCode(c.Context(), code)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"code":    "ERR_NOT_FOUND",
			"message": "Voucher not found",
		})
	}

	return c.JSON(voucher)
}

// GetAllVouchers - GET /api/v1/vouchers
func (h *VoucherHandler) GetAllVouchers(c *fiber.Ctx) error {
	limit, _ := strconv.Atoi(c.Query("limit", "20"))
	offset, _ := strconv.Atoi(c.Query("offset", "0"))
	keyword := c.Query("keyword", "")
	delMode, _ := strconv.Atoi(c.Query("del_mode", "0"))

	req := &dto.GetVouchersRequest{
		Limit:   limit,
		Offset:  offset,
		Keyword: keyword,
		DelMode: delMode,
	}

	vouchers, total, err := h.voucherService.GetAllVouchers(c.Context(), req)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"code":    "ERR_GET_VOUCHERS",
			"message": err.Error(),
		})
	}

	return c.JSON(fiber.Map{
		"vouchers":    vouchers,
		"total_count": total,
		"limit":       limit,
		"offset":      offset,
	})
}

// UpdateVoucher - PUT /api/v1/vouchers/:id
func (h *VoucherHandler) UpdateVoucher(c *fiber.Ctx) error {
	voucherID, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"code":    "ERR_INVALID_ID",
			"message": "Invalid voucher ID",
		})
	}

	var req dto.UpdateVoucherRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"code":    "ERR_INVALID_REQUEST",
			"message": "Invalid request body",
		})
	}

	// M-01 (audit 260909 vòng 2): xem ghi chú ở CreateVoucher.
	if errs := utils.ValidateStruct(req); len(errs) > 0 {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"code":    "ERR_VALIDATION",
			"message": "Validation failed",
			"errors":  errs,
		})
	}

	voucher, err := h.voucherService.UpdateVoucher(c.Context(), voucherID, &req)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"code":    "ERR_UPDATE_VOUCHER",
			"message": err.Error(),
		})
	}

	return c.JSON(voucher)
}

// DeleteVoucher - DELETE /api/v1/vouchers/:id
func (h *VoucherHandler) DeleteVoucher(c *fiber.Ctx) error {
	voucherID, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"code":    "ERR_INVALID_ID",
			"message": "Invalid voucher ID",
		})
	}

	err = h.voucherService.DeleteVoucher(c.Context(), voucherID)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"code":    "ERR_DELETE_VOUCHER",
			"message": err.Error(),
		})
	}

	return c.JSON(fiber.Map{
		"message": "Voucher deleted successfully",
	})
}

// RestoreVoucher - POST /api/v1/vouchers/:id/restore
func (h *VoucherHandler) RestoreVoucher(c *fiber.Ctx) error {
	voucherID, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"code":    "ERR_INVALID_ID",
			"message": "Invalid voucher ID",
		})
	}

	err = h.voucherService.RestoreVoucher(c.Context(), voucherID)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"code":    "ERR_RESTORE_VOUCHER",
			"message": err.Error(),
		})
	}

	return c.JSON(fiber.Map{
		"message": "Voucher restored successfully",
	})
}

// ActivateVoucher - POST /api/v1/vouchers/:id/activate
func (h *VoucherHandler) ActivateVoucher(c *fiber.Ctx) error {
	voucherID, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"code":    "ERR_INVALID_ID",
			"message": "Invalid voucher ID",
		})
	}

	err = h.voucherService.ActivateVoucher(c.Context(), voucherID)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"code":    "ERR_ACTIVATE_VOUCHER",
			"message": err.Error(),
		})
	}

	return c.JSON(fiber.Map{
		"message": "Voucher activated successfully",
	})
}

// DeactivateVoucher - POST /api/v1/vouchers/:id/deactivate
func (h *VoucherHandler) DeactivateVoucher(c *fiber.Ctx) error {
	voucherID, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"code":    "ERR_INVALID_ID",
			"message": "Invalid voucher ID",
		})
	}

	err = h.voucherService.DeactivateVoucher(c.Context(), voucherID)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"code":    "ERR_DEACTIVATE_VOUCHER",
			"message": err.Error(),
		})
	}

	return c.JSON(fiber.Map{
		"message": "Voucher deactivated successfully",
	})
}

// ============================================================
// USER VOUCHER
// ============================================================

// SaveVoucher - POST /api/v1/vouchers/:id/save
func (h *VoucherHandler) SaveVoucher(c *fiber.Ctx) error {
	// M-01 (audit 260909 vòng 2) — bug tìm thấy khi thêm validate: handler này trước đây đọc
	// Locals("user_id") rồi ép kiểu ".(string)", trong khi AuthMiddleware set thẳng uuid.UUID
	// (xem ghi chú getAuthUserID trong order_handler.go, cùng package handler) — type
	// assertion 1 giá trị sai kiểu PANIC, route này 500 với MỌI user đã đăng nhập.
	userID, ok := getAuthUserID(c)
	if !ok {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"code":    "ERR_UNAUTHORIZED",
			"message": "Unauthorized",
		})
	}

	voucherID, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"code":    "ERR_INVALID_ID",
			"message": "Invalid voucher ID",
		})
	}

	// Get voucher by ID to get code
	voucher, err := h.voucherService.GetVoucherByID(c.Context(), voucherID)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"code":    "ERR_NOT_FOUND",
			"message": "Voucher not found",
		})
	}

	var req dto.SaveVoucherRequest
	_ = c.BodyParser(&req)
	req.VoucherCode = voucher.Code

	// M-01 (audit 260909 vòng 2): DTO có validate tag (VoucherCode required) nhưng handler
	// chưa gọi ValidateStruct.
	if errs := utils.ValidateStruct(req); len(errs) > 0 {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"code":    "ERR_VALIDATION",
			"message": "Validation failed",
			"errors":  errs,
		})
	}

	savedVoucher, err := h.voucherService.SaveVoucher(c.Context(), userID, &req)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"code":    "ERR_SAVE_VOUCHER",
			"message": err.Error(),
		})
	}

	return c.Status(fiber.StatusCreated).JSON(savedVoucher)
}

// UnsaveVoucher - DELETE /api/v1/vouchers/:id/save
func (h *VoucherHandler) UnsaveVoucher(c *fiber.Ctx) error {
	// B2-01 (review vòng 2/2b): cùng lỗi panic `.(string)` như SaveVoucher trước khi sửa —
	// AuthMiddleware set thẳng uuid.UUID vào Locals("user_id"), không phải string. Route này
	// bị bỏ sót khi sửa SaveVoucher ở vòng 2a. Dùng lại getAuthUserID (đã dùng ở SaveVoucher).
	userID, ok := getAuthUserID(c)
	if !ok {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"code":    "ERR_UNAUTHORIZED",
			"message": "Unauthorized",
		})
	}

	voucherID, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"code":    "ERR_INVALID_ID",
			"message": "Invalid voucher ID",
		})
	}

	err = h.voucherService.UnsaveVoucher(c.Context(), userID, voucherID)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"code":    "ERR_UNSAVE_VOUCHER",
			"message": err.Error(),
		})
	}

	return c.JSON(fiber.Map{
		"message": "Voucher unsaved successfully",
	})
}

// GetUserSavedVouchers - GET /api/v1/vouchers/me
func (h *VoucherHandler) GetUserSavedVouchers(c *fiber.Ctx) error {
	// B2-01 (review vòng 2/2b): cùng lỗi panic `.(string)` như SaveVoucher, bỏ sót ở vòng 2a.
	// Endpoint này là đầu ra chính của mục 27 (vòng 2b) — panic khiến "/my-vouchers" 500 với
	// MỌI user đã đăng nhập, phủ nhận luôn tác dụng của Preload("Voucher") vừa thêm.
	userID, ok := getAuthUserID(c)
	if !ok {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"code":    "ERR_UNAUTHORIZED",
			"message": "Unauthorized",
		})
	}

	limit, _ := strconv.Atoi(c.Query("limit", "20"))
	offset, _ := strconv.Atoi(c.Query("offset", "0"))

	vouchers, total, err := h.voucherService.GetUserSavedVouchers(c.Context(), userID, limit, offset)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"code":    "ERR_GET_SAVED_VOUCHERS",
			"message": err.Error(),
		})
	}

	return c.JSON(fiber.Map{
		"vouchers":    vouchers,
		"total_count": total,
		"limit":       limit,
		"offset":      offset,
	})
}

// GetPublicVouchers - GET /api/v1/vouchers/public
func (h *VoucherHandler) GetPublicVouchers(c *fiber.Ctx) error {
	limit, _ := strconv.Atoi(c.Query("limit", "20"))
	offset, _ := strconv.Atoi(c.Query("offset", "0"))

	vouchers, total, err := h.voucherService.GetPublicVouchers(c.Context(), limit, offset)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"code":    "ERR_GET_PUBLIC_VOUCHERS",
			"message": err.Error(),
		})
	}

	return c.JSON(fiber.Map{
		"vouchers":    vouchers,
		"total_count": total,
		"limit":       limit,
		"offset":      offset,
	})
}

// GetVoucherStats - GET /api/v1/vouchers/:id/stats
func (h *VoucherHandler) GetVoucherStats(c *fiber.Ctx) error {
	voucherID, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"code":    "ERR_INVALID_ID",
			"message": "Invalid voucher ID",
		})
	}

	stats, err := h.voucherService.GetVoucherStats(c.Context(), voucherID)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"code":    "ERR_GET_STATS",
			"message": err.Error(),
		})
	}

	return c.JSON(stats)
}
