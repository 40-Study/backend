package dto

type PrivacySettingsResponseDTO struct {
	ProfileVisibility  string `json:"profile_visibility"`
	ActivityStatus     string `json:"activity_status"`
	LeaderboardDisplay string `json:"leaderboard_display"`
}

type UpdatePrivacySettingsDTO struct {
	// S6: profile_visibility quyết định người khác thấy gì ở /users/:id/public-profile — chỉ nhận giá
	// trị đã định nghĩa (trước đây lưu chuỗi bất kỳ). "hidden" = ẩn hẳn (người khác nhận 404).
	ProfileVisibility  *string `json:"profile_visibility,omitempty" validate:"omitempty,oneof=public friends private hidden"`
	ActivityStatus     *string `json:"activity_status,omitempty"`
	LeaderboardDisplay *string `json:"leaderboard_display,omitempty" validate:"omitempty,oneof=name username anonymous"`
}
