package dto

type PrivacySettingsResponseDTO struct {
	ProfileVisibility  string `json:"profile_visibility"`
	ActivityStatus     string `json:"activity_status"`
	LeaderboardDisplay string `json:"leaderboard_display"`
}

type UpdatePrivacySettingsDTO struct {
	ProfileVisibility  *string `json:"profile_visibility,omitempty"`
	ActivityStatus     *string `json:"activity_status,omitempty"`
	LeaderboardDisplay *string `json:"leaderboard_display,omitempty"`
}
