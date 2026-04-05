package dto

// OAuthState — dữ liệu lưu trong Redis khi bắt đầu OAuth flow
// Chứa thông tin provider + device để sau khi callback có thể tạo session
type OAuthState struct {
	Provider   string `json:"provider"`    // "github", "google", "facebook"
	DeviceID   string `json:"device_id"`
	DeviceName string `json:"device_name"`
	OS         string `json:"os"`
}
