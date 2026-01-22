package auth

import "time"

type SessionResponseDto struct {
	ID           string    `json:"id" example:"550e8400-e29b-41d4-a716-446655440000"`
	DeviceName   string    `json:"device_name" example:"iPhone 14 Pro"`
	DeviceType   string    `json:"device_type,omitempty" example:"mobile"`
	OS           string    `json:"os" example:"iOS 16.5"`
	Browser      string    `json:"browser,omitempty" example:"Safari"`
	IPAddress    string    `json:"ip_address" example:"192.168.1.1"`
	LastActiveAt time.Time `json:"last_active_at" example:"2023-10-27T10:00:00Z"`
	IsCurrent    bool      `json:"is_current" example:"true"`
}

type LogoutRequestDto struct {
	SessionID    string `json:"session_id,omitempty" validate:"omitempty,uuid"`
	RevokeOthers bool   `json:"revoke_others,omitempty"`
}
