package dto

import "time"

// OAuthState — dữ liệu lưu trong Redis khi bắt đầu OAuth flow
// Chứa thông tin provider + device để sau khi callback có thể tạo session
type OAuthState struct {
	Provider   string `json:"provider"`    // "github", "google", "facebook"
	DeviceID   string `json:"device_id"`
	DeviceName string `json:"device_name"`
	OS         string `json:"os"`
}

// LinkedAccountDTO — thông tin một provider đã liên kết với user
type LinkedAccountDTO struct {
	Provider    string    `json:"provider"`
	Email       *string   `json:"email,omitempty"`
	ConnectedAt time.Time `json:"connected_at"`
}
