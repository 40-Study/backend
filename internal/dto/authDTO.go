package dto

import "github.com/google/uuid"

type LoginDTO struct {
	Email    string    `json:"email" binding:"required,email"`
	Password string    `json:"password" binding:"required,min=6"`
	DeviceId uuid.UUID `json:"device_id" binding:"required"`

	DeviceName string `json:"device_name" binding:"required"`
	UserAgent  string `json:"user_agent" binding:"required"` 
	IpAddress  string `json:"ip" binding:"required,ip"`
}

type RegisterRequestDto struct {
	Email       string `json:"email" binding:"required,email"`
	UserName    string `json:"user_name" binding:"required,min=6"`
	Password    string `json:"password" binding:"required,min=6"`
	Phone       string `json:"phone" binding:"required,min=9"`
	DateOfBirth string `json:"date_of_birth" binding:"required"`
	Gender      string `json:"gender" binding:"required,oneof=male female other"`
}

type RegisterDto struct {
	Email       string `json:"email" binding:"required,email"`
	UserName    string `json:"user_name" binding:"required,min=6"`
	Password    string `json:"password" binding:"required,min=6"`
	Phone       string `json:"phone" binding:"required,min=9"`
	DateOfBirth string `json:"date_of_birth" binding:"required"`
	Gender      string `json:"gender" binding:"required,oneof=male female other"`
	OTP         string `json:"otp" binding:"required,min=6"`
}

type ChangePasswordDto struct {
	OldPassword string `json:"old_password" binding:"required,min=6"`
	NewPassword string `json:"new_password" binding:"required,min=6"`
}

type ResetPasswordRequestDto struct {
	Email string `json:"email" binding:"required,email"`
}

type ResetPasswordDto struct {
	Email       string `json:"email" binding:"required,email"`
	NewPassword string `json:"new_password" binding:"required,min=6"`
	OTP         string `json:"otp" binding:"required,min=6"`
}

type UserResponseDto struct {
	ID          uuid.UUID `json:"id"`
	Username    string    `json:"username"`
	Email       string    `json:"email"`
	Phone       *string   `json:"phone,omitempty"`
	AvatarUrl   *string   `json:"avatar_url,omitempty"`
	DateOfBirth *string   `json:"date_of_birth,omitempty"`
	Gender      *string   `json:"gender,omitempty"`
	Status      *string   `json:"status,omitempty"`
	CreatedAt   string    `json:"created_at"`
}

type LoginResponseDto struct {
	AccessToken  string          `json:"access_token"`
	RefreshToken string          `json:"refresh_token"`
	User         UserResponseDto `json:"user"`
}

type RegisterResponseDto struct {
	User UserResponseDto `json:"user"`
}

type RefreshTokenResponseDto struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
}
