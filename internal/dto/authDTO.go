package dto

import "github.com/google/uuid"

// RoleDto - Role information in responses
type RoleDto struct {
	ID   string `json:"id" example:"550e8400-e29b-41d4-a716-446655440000"`
	Code string `json:"code" example:"student"`
	Name string `json:"name" example:"Học sinh"`
}

type DeviceInfoDTO struct {
	DeviceID   string `json:"device_id" validate:"required,uuid" example:"550e8400-e29b-41d4-a716-446655440000"`
	DeviceName string `json:"device_name" validate:"required,min=2,max=100" example:"iPhone 14 Pro"`
	OS         string `json:"os" validate:"required,max=50" example:"iOS 16.5"`
	AppVersion string `json:"app_version,omitempty" validate:"omitempty,max=50" example:"1.0.0"`
	UserAgent  string `json:"user_agent,omitempty" validate:"omitempty,max=512" example:"Mozilla/5.0..."`
}


type LoginRequestDto struct {
	Email      string        `json:"email" validate:"required,email" example:"student@example.com"`
	Password   string        `json:"password" validate:"required,min=8" example:"ResilientPass123!"`
	DeviceInfo DeviceInfoDTO `json:"device_info" validate:"required"`
}

type UserResponseDto struct {
	ID          uuid.UUID `json:"id" example:"550e8400-e29b-41d4-a716-446655440000"`
	Username    string    `json:"username" example:"student123"`
	Email       string    `json:"email" example:"student@example.com"`
	Phone       *string   `json:"phone,omitempty" example:"+84901234567"`
	AvatarUrl   *string   `json:"avatar_url,omitempty" example:"https://example.com/avatar.jpg"`
	DateOfBirth *string   `json:"date_of_birth,omitempty" example:"2005-01-01"`
	Status      *string   `json:"status,omitempty" example:"active"`
	CreatedAt   string    `json:"created_at" example:"2023-01-01T00:00:00Z"`
}

type DeviceSessionDto struct {
	DeviceID   string `json:"device_id" example:"550e8400-e29b-41d4-a716-446655440000"`
	DeviceName string `json:"device_name" example:"iPhone 14 Pro"`
	UserAgent  string `json:"user_agent,omitempty" example:"Mozilla/5.0..."`
	LoggedInAt string `json:"logged_in_at" example:"2024-01-01T00:00:00Z"`
	IsCurrent  bool   `json:"is_current,omitempty"`
}

type LoginResponseDto struct {
	AccessToken   string           `json:"access_token"`
	RefreshToken  string           `json:"refresh_token"`
	User          UserResponseDto  `json:"user"`
	CurrentDevice DeviceSessionDto `json:"current_device"`
}

type RefreshTokenResponseDto struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
}


type RequestPasswordResetDto struct {
	Email string `json:"email" validate:"required,email" example:"student@example.com"`
}

type ResetPasswordRequestDto struct {
	Email           string `json:"email" validate:"required,email"`
	Otp             string `json:"otp" validate:"required,len=6,numeric"`
	NewPassword     string `json:"new_password" validate:"required,min=8,max=72"`
	ConfirmPassword string `json:"confirm_password" validate:"required,eqfield=NewPassword"`
}


type ChangePasswordRequestDto struct {
	OldPassword     string        `json:"old_password" validate:"required,min=8,max=72" example:"OldPass123!"`
	NewPassword     string        `json:"new_password" validate:"required,min=8,max=72,nefield=OldPassword,containsany=!@#$%^&*()" example:"NewSecurePass123!"`
	ConfirmPassword string        `json:"confirm_password" validate:"required,eqfield=NewPassword" example:"NewSecurePass123!"`
	DeviceInfo      DeviceInfoDTO `json:"device_info" validate:"required"`
	RevokeOthers    bool          `json:"revoke_others" example:"true"`
}


// RegisterRequestDto - Request body for POST /auth/register/request
// Gửi thông tin đăng ký và nhận OTP qua email
type RegisterRequestDto struct {
	Email           string   `json:"email" validate:"required,email,max=255" example:"student@example.com"`
	Password        string   `json:"password" validate:"required,min=8,max=72" example:"SecurePass123!"`
	ConfirmPassword string   `json:"confirm_password" validate:"required,eqfield=Password" example:"SecurePass123!"`
	UserName        string   `json:"user_name" validate:"required,min=3,max=100" example:"student123"`
	FullName        string   `json:"full_name,omitempty" validate:"omitempty,min=2,max=255" example:"Nguyen Van A"`
	RoleCodes       []string `json:"role_codes" validate:"required,min=1,max=2,dive,oneof=student parent" example:"[\"student\"]"`
}

// RegisterResponseDto - Response for successful registration
type RegisterResponseDto struct {
	ID        string   `json:"id" example:"550e8400-e29b-41d4-a716-446655440000"`
	Email     string   `json:"email" example:"student@example.com"`
	UserName  string   `json:"user_name" example:"student123"`
	FullName  *string  `json:"full_name,omitempty" example:"Nguyen Van A"`
	RoleCodes []string `json:"role_codes" example:"[\"student\"]"`
}

// VerifyOtpRequestDto - Request body for POST /auth/register
// Xác thực OTP và tạo user thật trong database
type VerifyOtpRequestDto struct {
	Email string `json:"email" validate:"required,email" example:"student@example.com"`
	OTP   string `json:"otp" validate:"required,len=6,numeric" example:"123456"`
}

// VerifyOtpResponseDto - Response for OTP verification (kept for backward compatibility)
type VerifyOtpResponseDto struct {
	User UserResponseDto `json:"user"`
}

// UpdateMeRequestDto - Request body for updating user profile
// All fields are optional (partial update)
type UpdateMeRequestDto struct {
	Username    *string `json:"username,omitempty" validate:"omitempty,alphanum,min=3,max=30" example:"student123"`
	Phone       *string `json:"phone,omitempty" validate:"omitempty,e164" example:"+84901234567"`
	DateOfBirth *string `json:"date_of_birth,omitempty" validate:"omitempty,datetime=2006-01-02" example:"2005-01-01"`
}
