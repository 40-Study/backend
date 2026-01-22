package auth

type ForgotPasswordRequestDto struct {
	Email string `json:"email" validate:"required,email" example:"student@example.com"`
}

type ResetPasswordRequestDto struct {
	Token           string `json:"token" validate:"required,uuid" example:"550e8400-e29b-41d4-a716-446655440000"`
	NewPassword     string `json:"new_password" validate:"required,min=12,max=72,containsany=!@#$%^&*()_+-=[]{}|;:,.<>?~" example:"SecureP@ssw0rd2024!"`
	ConfirmPassword string `json:"confirm_password" validate:"required,eqfield=NewPassword" example:"SecureP@ssw0rd2024!"`
}

type ChangePasswordRequestDto struct {
	OldPassword     string        `json:"old_password" validate:"required" example:"OldPass123!"`
	NewPassword     string        `json:"new_password" validate:"required,min=12,max=72,nefield=OldPassword,containsany=!@#$%^&*()_+-=[]{}|;:,.<>?~" example:"NewSecurePass123!"`
	ConfirmPassword string        `json:"confirm_password" validate:"required,eqfield=NewPassword" example:"NewSecurePass123!"`
	DeviceInfo      DeviceInfoDTO `json:"device_info" validate:"required"`
	RevokeOthers    bool          `json:"revoke_others" example:"true"`
}
