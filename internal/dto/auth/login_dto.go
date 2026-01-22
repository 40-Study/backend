package auth

type LoginRequestDto struct {
	Email      string        `json:"email" validate:"required,email" example:"student@example.com"`
	Password   string        `json:"password" validate:"required,min=8" example:"ResilientPass123!"`
	DeviceInfo DeviceInfoDTO `json:"device_info" validate:"required"`
}

type UserResponseDto struct {
	ID         string `json:"id" example:"550e8400-e29b-41d4-a716-446655440000"`
	Username   string `json:"username" example:"student123"`
	Email      string `json:"email" example:"student@example.com"`
	FullName   string `json:"full_name" example:"Nguyen Van A"`
	AvatarURL  string `json:"avatar_url,omitempty" example:"https://example.com/avatar.jpg"`
	IsVerified bool   `json:"is_verified" example:"true"`
	JoinedAt   string `json:"joined_at" example:"2023-01-01T00:00:00Z"`
}

type LoginResponseDto struct {
	AccessToken  string          `json:"access_token" example:"eyJhbGciOiJIUzI1Ni..."`
	RefreshToken string          `json:"refresh_token" example:"dcc417c8-..."`
	ExpiresIn    int             `json:"expires_in" example:"3600"`
	User         UserResponseDto `json:"user"`
	SessionID    string          `json:"session_id" example:"550e8400-e29b-41d4-a716-446655440000"`
}