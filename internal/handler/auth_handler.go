package handler

import (
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"study.com/v1/internal/dto"
	"study.com/v1/internal/service"
	"study.com/v1/internal/utils"
)

type AuthHandlerInterface interface {
	Login(c *fiber.Ctx) error
	RequestRegister(c *fiber.Ctx) error
	Register(c *fiber.Ctx) error
	RefreshToken(c *fiber.Ctx) error
	RequestPasswordReset(c *fiber.Ctx) error
	ResetPassword(c *fiber.Ctx) error
	GetMe(c *fiber.Ctx) error
	UpdateMe(c *fiber.Ctx) error
	GetAllDevices(c *fiber.Ctx) error
	LogoutOneDevice(c *fiber.Ctx) error
	LogoutAll(c *fiber.Ctx) error
	ChangePassword(c *fiber.Ctx) error
}

type AuthHandler struct {
	authService service.AuthServiceInterface
}

func NewAuthHandler(authService service.AuthServiceInterface) *AuthHandler {
	return &AuthHandler{
		authService: authService,
	}
}

// Register handles POST /auth/register
// Verifies OTP and creates user in database with assigned roles
// @Summary Register new user (verify OTP)
// @Tags Auth
// @Accept json
// @Produce json
// @Param request body dto.VerifyOtpRequestDto true "OTP verification"
// @Success 201 {object} dto.RegisterResponseDto
// @Failure 400 {object} map[string]interface{}
// @Router /auth/register [post]
func (h *AuthHandler) Register(c *fiber.Ctx) error {
	var req dto.VerifyOtpRequestDto
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Invalid request body",
			"error":   err.Error(),
		})
	}

	// Validate request
	if errors := utils.ValidateStruct(req); len(errors) > 0 {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Validation failed",
			"errors":  errors,
		})
	}

	response, err := h.authService.Register(c.Context(), req)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Register failed",
			"error":   err.Error(),
		})
	}
	return c.Status(fiber.StatusCreated).JSON(fiber.Map{
		"message": "User registered successfully",
		"data":    response,
	})
}

// RequestRegister handles POST /auth/register/request
// Validates registration data and sends OTP to email
// @Summary Request registration (send OTP)
// @Tags Auth
// @Accept json
// @Produce json
// @Param request body dto.RegisterRequestDto true "Registration data"
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} map[string]interface{}
// @Router /auth/register/request [post]
func (h *AuthHandler) RequestRegister(c *fiber.Ctx) error {
	var req dto.RegisterRequestDto
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Invalid request body",
			"error":   err.Error(),
		})
	}

	// Validate request
	if errors := utils.ValidateStruct(req); len(errors) > 0 {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Validation failed",
			"errors":  errors,
		})
	}

	err := h.authService.RequestRegister(c.Context(), req)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Request register failed",
			"error":   err.Error(),
		})
	}
	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"message": "OTP sent to your email. Please verify within 5 minutes.",
	})
}

func (h *AuthHandler) Login(c *fiber.Ctx) error {
	var req dto.LoginRequestDto
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Invalid request body",
			"error":   err.Error(),
		})
	}

	// Validate request
	if errors := utils.ValidateStruct(req); len(errors) > 0 {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Validation failed",
			"errors":  errors,
		})
	}

	response, err := h.authService.Login(c.Context(), req)
	if err != nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"message": "Login failed",
			"error":   err.Error(),
		})
	}
	c.Cookie(&fiber.Cookie{
		Name:     "accessToken",
		Value:    response.AccessToken,
		Expires:  time.Now().Add(15 * time.Minute),
		HTTPOnly: true,
		Secure:   true,
		SameSite: "Lax",
	})
	c.Cookie(&fiber.Cookie{
		Name:     "rfToken",
		Value:    response.RefreshToken,
		Expires:  time.Now().Add(24 * time.Hour),
		HTTPOnly: true,
		Secure:   true,
		SameSite: "Lax",
	})
	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"message": "Login successful",
		"data":    response,
	})
}

func (h *AuthHandler) LogoutOneDevice(c *fiber.Ctx) error {
	user_id := c.Locals("user_id")
	device_id := c.Locals("device_id")

	if user_id == nil || device_id == nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Empty userId or deviceID",
		})
	}
	userID, ok := user_id.(uuid.UUID)
	if !ok {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Can not logout",
		})
	}
	deviceID, ok := device_id.(uuid.UUID)
	if !ok {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Can not logout",
		})
	}

	if userID == uuid.Nil || deviceID == uuid.Nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Can not logout",
		})
	}
	err := h.authService.Logout(c.Context(), userID, deviceID)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"message": "Logout service error",
			"error":   err.Error(),
		})
	}
	
	// Clear cookies
	c.Cookie(&fiber.Cookie{
		Name:     "accessToken",
		Value:    "",
		Expires:  time.Now().Add(-1 * time.Hour),
		HTTPOnly: true,
		Secure:   true,
		SameSite: "Lax",
	})
	c.Cookie(&fiber.Cookie{
		Name:     "rfToken",
		Value:    "",
		Expires:  time.Now().Add(-1 * time.Hour),
		HTTPOnly: true,
		Secure:   true,
		SameSite: "Lax",
	})
	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"message": "Logout successfully",
	})
}

func (h *AuthHandler) LogoutAll(c *fiber.Ctx) error {
	user_id := c.Locals("user_id")
	if user_id == nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Empty userId",
		})
	}
	
	uid, ok := user_id.(uuid.UUID)
	if !ok || uid == uuid.Nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Invalid userId",
		})
	}
	
	if err := h.authService.LogoutAllDevice(c.Context(), uid); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"message": "Logout all devices failed",
			"error":   err.Error(),
		})
	}

	// Clear cookies on current device
	c.Cookie(&fiber.Cookie{
		Name:     "accessToken",
		Value:    "",
		Expires:  time.Now().Add(-1 * time.Hour),
		HTTPOnly: true,
		Secure:   true,
		SameSite: "Lax",
	})
	c.Cookie(&fiber.Cookie{
		Name:     "rfToken",
		Value:    "",
		Expires:  time.Now().Add(-1 * time.Hour),
		HTTPOnly: true,
		Secure:   true,
		SameSite: "Lax",
	})

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"message": "Logout on all devices successfully",
	})
}

func (h *AuthHandler) RefreshToken(c *fiber.Ctx) error {
	old_rfToken := c.Cookies("rfToken")
	if old_rfToken == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Missing refresh token",
		})
	}
	
	response, err := h.authService.RefreshToken(c.Context(), old_rfToken)
	if err != nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"message": "Refresh token failed",
			"error":   err.Error(),
		})
	}
	c.Cookie(&fiber.Cookie{
		Name:     "accessToken",
		Value:    response.AccessToken,
		Expires:  time.Now().Add(15 * time.Minute),
		HTTPOnly: true,
		Secure:   true,
		SameSite: "Lax",
	})
	c.Cookie(&fiber.Cookie{
		Name:     "rfToken",
		Value:    response.RefreshToken,
		Expires:  time.Now().Add(24 * time.Hour),
		HTTPOnly: true,
		Secure:   true,
		SameSite: "Lax",
	})
	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"message": "Refresh token successfully",
		"data":    response,
	})
}

func (h *AuthHandler) GetMe(c *fiber.Ctx) error {
	user_id := c.Locals("user_id")
	if user_id == nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Empty userId",
		})
	}
	uid, ok := user_id.(uuid.UUID)
	if !ok {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Can not get user info",
		})
	}
	user, err := h.authService.GetMe(c.Context(), uid)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Get user info failed",
			"error":   err.Error(),
		})
	}
	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"message": "Get user info successfully",
		"data":    user,
	})
}

func (h *AuthHandler) UpdateMe(c *fiber.Ctx) error {
	// ===== 1. Get user_id from token =====
	user_id := c.Locals("user_id")
	if user_id == nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Empty userId",
		})
	}

	uid, ok := user_id.(uuid.UUID)
	if !ok {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Invalid userId",
		})
	}

	// ===== 2. Parse request body =====
	var req dto.UpdateMeRequestDto
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Invalid request body",
			"error":   err.Error(),
		})
	}

	// ===== 3. Validate request =====
	if errors := utils.ValidateStruct(req); len(errors) > 0 {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Validation failed",
			"errors":  errors,
		})
	}

	// ===== 4. Call service =====
	user, err := h.authService.UpdateMe(c.Context(), uid, req)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Update user info failed",
			"error":   err.Error(),
		})
	}

	// ===== 5. Return response =====
	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"message": "Update user info successfully",
		"data":    user,
	})
}

func (h *AuthHandler) ChangePassword(c *fiber.Ctx) error {
	var req dto.ChangePasswordRequestDto
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Invalid body",
			"error":   err.Error(),
		})
	}

	// Validate request
	if errors := utils.ValidateStruct(req); len(errors) > 0 {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Validation failed",
			"errors":  errors,
		})
	}

	user_id := c.Locals("user_id")
	if user_id == nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Empty userId",
		})
	}

	userId, ok := user_id.(uuid.UUID)
	if !ok || userId == uuid.Nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Invalid userId",
		})
	}

	err := h.authService.ChangePassword(c.Context(), userId, req)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Change password failed",
			"error":   err.Error(),
		})
	}
	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"message": "Change password successfully",
	})
}

func (h *AuthHandler) RequestPasswordReset(c *fiber.Ctx) error {
	var req dto.RequestPasswordResetDto
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Invalid request body",
			"error":   err.Error(),
		})
	}

	// Validate request
	if errors := utils.ValidateStruct(req); len(errors) > 0 {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Validation failed",
			"errors":  errors,
		})
	}

	if err := h.authService.RequestPasswordReset(c.Context(), req.Email); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Request reset password failed",
			"error":   err.Error(),
		})
	}
	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"message": "OTP has been sent",
	})
}

func (h *AuthHandler) ResetPassword(c *fiber.Ctx) error {
	var req dto.ResetPasswordRequestDto
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Invalid request body",
			"error":   err.Error(),
		})
	}

	// Validate request
	if errors := utils.ValidateStruct(req); len(errors) > 0 {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Validation failed",
			"errors":  errors,
		})
	}

	if err := h.authService.ResetPassword(c.Context(), req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Reset password failed",
			"error":   err.Error(),
		})
	}
	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"message": "Password reset successfully",
	})
}
