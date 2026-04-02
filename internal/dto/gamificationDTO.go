package dto

import (
	"time"

	"github.com/google/uuid"
)

// ============================================================
// Achievement DTOs
// ============================================================

type AchievementDTO struct {
	ID          uuid.UUID `json:"id"`
	Name        string    `json:"name"`
	Slug        string    `json:"slug"`
	Description string    `json:"description"`
	IconURL     *string   `json:"icon_url,omitempty"`
	BadgeURL    *string   `json:"badge_url,omitempty"`
	Category    string    `json:"category"`
	Points      int       `json:"points"`
	Requirement string    `json:"requirement"`
	Threshold   int       `json:"threshold"`
}

// AchievementWithStatusDTO includes whether the current user has unlocked it
type AchievementWithStatusDTO struct {
	AchievementDTO
	Unlocked  bool       `json:"unlocked"`
	EarnedAt  *time.Time `json:"earned_at,omitempty"`
	Progress  int        `json:"progress"` // 0-100
}

type UnlockAchievementResponse struct {
	AchievementID uuid.UUID `json:"achievement_id"`
	UserID        uuid.UUID `json:"user_id"`
	EarnedAt      time.Time `json:"earned_at"`
	Message       string    `json:"message"`
}

// ============================================================
// Leaderboard DTOs
// ============================================================

type LeaderboardEntryDTO struct {
	Rank      int        `json:"rank"`
	UserID    uuid.UUID  `json:"user_id"`
	UserName  string     `json:"user_name"`
	FullName  *string    `json:"full_name,omitempty"`
	AvatarURL *string    `json:"avatar_url,omitempty"`
	Points    int        `json:"points"`
}

type LeaderboardResponse struct {
	PeriodType string                `json:"period_type"`
	Period     string                `json:"period"`
	Entries    []LeaderboardEntryDTO `json:"entries"`
	Total      int                   `json:"total"`
}

type MyRankResponse struct {
	PeriodType string               `json:"period_type"`
	Period     string               `json:"period"`
	Entry      *LeaderboardEntryDTO `json:"entry"`
}

// ============================================================
// User Stats / Public Profile DTOs
// ============================================================

type UserStatsDTO struct {
	TotalPoints      int `json:"total_points"`
	Level            int `json:"level"`
	LevelProgress    int `json:"level_progress"`
	CurrentStreak    int `json:"current_streak"`
	LongestStreak    int `json:"longest_streak"`
	TotalCheckins    int `json:"total_checkins"`
	AchievementCount int `json:"achievement_count"`
}

type PublicProfileResponse struct {
	UserID    uuid.UUID    `json:"user_id"`
	UserName  string       `json:"user_name"`
	FullName  *string      `json:"full_name,omitempty"`
	AvatarURL *string      `json:"avatar_url,omitempty"`
	Bio       *string      `json:"bio,omitempty"`
	Stats     UserStatsDTO `json:"stats"`
}
