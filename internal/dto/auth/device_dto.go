package auth

type DeviceInfoDTO struct {
	DeviceID   string `json:"device_id" validate:"required" example:"550e8400-e29b-41d4-a716-446655440000"`
	DeviceName string `json:"device_name" validate:"required,min=2,max=100" example:"iPhone 14 Pro"`
	OS         string `json:"os" validate:"required,max=50" example:"iOS 16.5"`
	AppVersion string `json:"app_version,omitempty" validate:"omitempty,max=20" example:"1.0.0"`
	UserAgent  string `json:"user_agent,omitempty" validate:"omitempty,max=255" example:"Mozilla/5.0..."`
}
