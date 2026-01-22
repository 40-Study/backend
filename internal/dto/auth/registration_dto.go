package auth

type RegisterRequestDto struct {
	FullName    string `json:"full_name" validate:"required,min=2,max=100" example:"Nguyen Van A"`
	Username    string `json:"username" validate:"required,alphanum,min=3,max=30" example:"student123"`
	Password    string `json:"password" validate:"required,min=6,max=72,nefield=Username,nefield=Email,containsany=!@#$%^&*()_+-=[]{}|;:,.<>?~" example:"ResilientP@ss!23"`
	Phone       string `json:"phone" validate:"required,min=9,max=15" example:"+84901234567"`
	DateOfBirth string `json:"date_of_birth" validate:"required" example:"2005-01-01"`
	Gender      string `json:"gender" validate:"required,oneof=male female other" example:"male"`
	Email       string `json:"email" validate:"required,email,max=255" example:"student@example.com"`
}

type VerifyMagicLinkRequestDto struct {
	Token      string        `json:"token" validate:"required,uuid" example:"550e8400-e29b-41d4-a716-446655440000"`
	DeviceInfo DeviceInfoDTO `json:"device_info" validate:"required"`
}

type VerifyMagicLinkResponseDto struct {
	AccessToken  string          `json:"access_token" example:"eyJhbGciOiJIUzI1Ni..."`
	RefreshToken string          `json:"refresh_token" example:"dcc417c8-..."`
	ExpiresIn    int             `json:"expires_in" example:"3600"`
	User         UserResponseDto `json:"user"`
	SessionID    string          `json:"session_id" example:"550e8400-e29b-41d4-a716-446655440000"`
}
