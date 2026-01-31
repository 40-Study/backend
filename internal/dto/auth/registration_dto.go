package auth

type RegisterRequestDto struct {
	FullName    string `json:"full_name" validate:"required,min=2,max=100" example:"Nguyen Van A"`
	Username    string `json:"username" validate:"required,alphanum,min=3,max=30" example:"student123"`
	Password    string `json:"password" validate:"required,min=8,max=72,nefield=Username,nefield=Email,containsany=!@#$%^&*()_+-=[]{}|;:,.<>?~" example:"ResilientP@ss!23"`
	Phone       string `json:"phone" validate:"required,e164" example:"+84901234567"`
	DateOfBirth string `json:"date_of_birth" validate:"required,datetime=2006-01-02" example:"2005-01-01"`
	Gender      string `json:"gender" validate:"required,oneof=male female other" example:"male"`
	Email       string `json:"email" validate:"required,email,max=255" example:"student@example.com"`
}

type VerifyOtpRequestDto struct {
	OTPRegister        string        `json:"otp_register" validate:"required,len=6,numeric" example:"123456"`
	DeviceInfo DeviceInfoDTO `json:"device_info" validate:"required"`
}

type VerifyOtpResponseDto struct {
	User UserResponseDto `json:"user"`
}

