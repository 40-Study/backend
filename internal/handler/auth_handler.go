package handler

import (
	"os"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"study.com/v1/internal/dto"
	"study.com/v1/internal/service"
	"study.com/v1/internal/utils"
)

type AuthHandlerInterface interface {
	// Auth
	Login(c *fiber.Ctx) error
	SelectRole(c *fiber.Ctx) error
	RequestRegister(c *fiber.Ctx) error
	Register(c *fiber.Ctx) error
	RefreshToken(c *fiber.Ctx) error
	RequestPasswordReset(c *fiber.Ctx) error
	ResetPassword(c *fiber.Ctx) error
	// Role management
	GetMyRoles(c *fiber.Ctx) error
	SwitchRole(c *fiber.Ctx) error
	// Profile management
	GetSystemRoleOptions(c *fiber.Ctx) error
	GetMyProfiles(c *fiber.Ctx) error
	CreateProfile(c *fiber.Ctx) error
	DeleteProfile(c *fiber.Ctx) error
	// User info
	GetMe(c *fiber.Ctx) error
	UpdateMe(c *fiber.Ctx) error
	// Session
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

	if response.Completed && response.AccessToken != "" {
		h.setAuthCookies(c, response.AccessToken, response.RefreshToken)
	}

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"message": "Login successful",
		"data":    response,
	})
}

func (h *AuthHandler) setAuthCookies(c *fiber.Ctx, accessToken, refreshToken string) {
	secure := os.Getenv("ENVIRONMENT") == "production"
	c.Cookie(&fiber.Cookie{
		Name:     "accessToken",
		Value:    accessToken,
		Path:     "/",
		Expires:  time.Now().Add(15 * time.Minute),
		HTTPOnly: true,
		Secure:   secure,
		SameSite: "Lax",
	})
	c.Cookie(&fiber.Cookie{
		Name:     "rfToken",
		Value:    refreshToken,
		Path:     "/",
		Expires:  time.Now().Add(24 * time.Hour),
		HTTPOnly: true,
		Secure:   secure,
		SameSite: "Lax",
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
		// Secure:   true, // lên https
		SameSite: "Lax", // same site là khi user đăng nhập từ một trình duyệt, thì khi user
		// đăng nhập từ trình duyệt khác, thì cookie sẽ không được gửi đi
	})
	c.Cookie(&fiber.Cookie{
		Name:     "rfToken",
		Value:    "",
		Expires:  time.Now().Add(-1 * time.Hour),
		HTTPOnly: true,
		// Secure:   true,
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

	// If not in cookie, try request body
	if old_rfToken == "" {
		var body struct {
			RefreshToken string `json:"refresh_token"`
		}
		if err := c.BodyParser(&body); err == nil && body.RefreshToken != "" {
			old_rfToken = body.RefreshToken
		}
	}

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

// ========== UNIFIED ROLE SELECTION (NEW FLOW) ==========

// GetMyRoles handles GET /auth/my-roles
// Returns unified list of system roles and organization roles
func (h *AuthHandler) GetMyRoles(c *fiber.Ctx) error {
	userID, ok := c.Locals("user_id").(uuid.UUID)
	if !ok || userID == uuid.Nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"message": "Unauthorized",
		})
	}

	response, err := h.authService.GetMyRoles(c.Context(), userID)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"message": "Failed to get roles",
			"error":   err.Error(),
		})
	}

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"message": "Success",
		"data":    response,
	})
}

// SelectRole handles POST /auth/select-role
// Selects a role during login flow (using session_token)
func (h *AuthHandler) SelectRole(c *fiber.Ctx) error {
	var req dto.SelectRoleRequestDto
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

	response, err := h.authService.SelectRole(c.Context(), req)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Select role failed",
			"error":   err.Error(),
		})
	}

	h.setAuthCookies(c, response.AccessToken, response.RefreshToken)

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"message": "Role selected successfully",
		"data":    response,
	})
}

// SwitchRole handles POST /auth/switch-role
// Switches role while already logged in
func (h *AuthHandler) SwitchRole(c *fiber.Ctx) error {
	userID, ok := c.Locals("user_id").(uuid.UUID)
	if !ok || userID == uuid.Nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"message": "Unauthorized",
		})
	}
	deviceID, ok := c.Locals("device_id").(uuid.UUID)
	if !ok || deviceID == uuid.Nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"message": "Unauthorized",
		})
	}

	var req dto.SwitchRoleRequestDto
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

	response, err := h.authService.SwitchRole(c.Context(), userID, deviceID, req)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Switch role failed",
			"error":   err.Error(),
		})
	}

	h.setAuthCookies(c, response.AccessToken, response.RefreshToken)

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"message": "Role switched successfully",
		"data":    response,
	})
}

// ========== PROFILE MANAGEMENT ==========

// GetSystemRoleOptions handles GET /auth/system-roles
// Returns all available system roles for creating profiles (public)
func (h *AuthHandler) GetSystemRoleOptions(c *fiber.Ctx) error {
	roles, err := h.authService.GetSystemRoleOptions(c.Context())
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"message": "Failed to get system roles",
			"error":   err.Error(),
		})
	}

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"message": "Success",
		"data": fiber.Map{
			"system_roles": roles,
		},
	})
}

// GetMyProfiles handles GET /auth/me/profiles
// Returns all profiles of current user
func (h *AuthHandler) GetMyProfiles(c *fiber.Ctx) error {
	userID, ok := c.Locals("user_id").(uuid.UUID)
	if !ok || userID == uuid.Nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"message": "Unauthorized",
		})
	}

	profiles, err := h.authService.GetMyProfiles(c.Context(), userID)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"message": "Failed to get profiles",
			"error":   err.Error(),
		})
	}

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"message": "Success",
		"data": fiber.Map{
			"profiles": profiles,
		},
	})
}

// CreateProfile handles POST /auth/me/profiles
// Creates a new profile with a system role
func (h *AuthHandler) CreateProfile(c *fiber.Ctx) error {
	userID, ok := c.Locals("user_id").(uuid.UUID)
	if !ok || userID == uuid.Nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"message": "Unauthorized",
		})
	}

	var req dto.CreateProfileRequestDto
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

	profile, err := h.authService.CreateProfile(c.Context(), userID, req)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Failed to create profile",
			"error":   err.Error(),
		})
	}

	return c.Status(fiber.StatusCreated).JSON(fiber.Map{
		"message": "Profile created successfully",
		"data":    profile,
	})
}

// DeleteProfile handles DELETE /auth/me/profiles/:id
// Deletes a profile
func (h *AuthHandler) DeleteProfile(c *fiber.Ctx) error {
	userID, ok := c.Locals("user_id").(uuid.UUID)
	if !ok || userID == uuid.Nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"message": "Unauthorized",
		})
	}

	profileIDStr := c.Params("id")
	profileID, err := uuid.Parse(profileIDStr)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Invalid profile ID",
		})
	}

	if err := h.authService.DeleteProfile(c.Context(), userID, profileID); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Failed to delete profile",
			"error":   err.Error(),
		})
	}

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"message": "Profile deleted successfully",
	})
}
