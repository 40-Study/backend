package handler

import (
	"fmt"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"tiger.com/v2/src/dto"
	"tiger.com/v2/src/service"
)

type AuthHandlerInterface interface {
	Login(c *fiber.Ctx) error
	RequestRegister(c *fiber.Ctx) error
	Register(c *fiber.Ctx) error
	RefreshToken(c *fiber.Ctx) error
	RequestPasswordReset(c *fiber.Ctx) error
	ResetPassword(c *fiber.Ctx) error
	GetMe(c *fiber.Ctx) error
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

func (h *AuthHandler) Register(c *fiber.Ctx) error {
	var req dto.RegisterDto
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Invalid request body",
			"error":   err.Error(),
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
		"message": "Register successfully",
		"data":    response,
	})
}

func (h *AuthHandler) RequestRegister(c *fiber.Ctx) error {
	var req dto.RegisterRequestDto
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Invalid request body",
			"error":   err.Error(),
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
		"message": "OTP sent to your email",
	})
}

func (h *AuthHandler) Login(c *fiber.Ctx) error {
	var req dto.LoginDTO
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Invalid request body",
			"error":   err.Error(),
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
	})
	c.Cookie(&fiber.Cookie{
		Name:     "rfToken",
		Value:    response.RefreshToken,
		Expires:  time.Now().Add(24 * time.Hour),
		HTTPOnly: true,
	})
	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"message": "Login successful",
		"data":    response,
	})
}

func (h *AuthHandler) LogoutOneDevice(c *fiber.Ctx) error {
	user_id := c.Locals("user_id")
	device_id := c.Locals("device_id")
	fmt.Print(user_id, " ", device_id, "\n")
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
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Logout service error",
			"error":   err.Error(),
		})
	}
	c.Cookie(&fiber.Cookie{
		Name:     "accessToken",
		Value:    "",
		Expires:  time.Now().Add(-1 * time.Hour),
		HTTPOnly: true,
	})
	c.Cookie(&fiber.Cookie{
		Name:     "rfToken",
		Value:    "",
		Expires:  time.Now().Add(-1 * time.Hour),
		HTTPOnly: true,
	})
	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"message": "Logout successfully",
	})
}

func (h *AuthHandler) LogoutAll(c *fiber.Ctx) error {
	user_id := c.Locals("user_id")
	uid := user_id.(uuid.UUID)
	if uid == uuid.Nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "userid invalid",
		})
	}
	if err := h.authService.LogoutAllDevice(c.Context(), uid); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "logout all device false",
		})
	}

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"message": "Logout on all devices successfully",
	})
}

func (h *AuthHandler) RefreshToken(c *fiber.Ctx) error {
	old_rfToken := c.Cookies("rfToken")
	response, err := h.authService.RefreshToken(c.Context(), old_rfToken)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "refresh token service false",
		})
	}
	c.Cookie(&fiber.Cookie{
		Name:     "accessToken",
		Value:    response.AccessToken,
		Expires:  time.Now().Add(15 * time.Minute),
		HTTPOnly: true,
	})
	c.Cookie(&fiber.Cookie{
		Name:     "rfToken",
		Value:    response.RefreshToken,
		Expires:  time.Now().Add(24 * time.Hour),
		HTTPOnly: true,
	})
	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"message": "refresh token successfully",
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

func (h *AuthHandler) UpdatePasswordHash(c *fiber.Ctx) error {
	var req dto.ChangePasswordDto
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Invalid body",
		})
	}
	userId := c.Locals("user_id").(uuid.UUID)
	err := h.authService.ChangePassword(c.Context(), userId, req)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Change password service errors",
			"errors":  err.Error(),
		})
	}
	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"message": "Change password successfully",
	})
}

func (h *AuthHandler) RequestPasswordReset(c *fiber.Ctx) error {
	var req dto.ResetPasswordRequestDto
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Body invalid",
			"errors":  err.Error(),
		})
	}
	if err := h.authService.RequestPasswordReset(c.Context(), req.Email); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Request reset password errors",
			"errors":  err.Error(),
		})
	}
	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"message": "OTP have been send",
	})
}

func (h *AuthHandler) ResetPassword(c *fiber.Ctx) error {
	var req dto.ResetPasswordDto
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Body invalid",
			"errors":  err.Error(),
		})
	}
	if err := h.authService.ResetPassword(c.Context(), req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Reset password errors",
			"errors":  err.Error(),
		})
	}
	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"message": "Password reset successfully",
	})
}
