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

// LeaderboardEntryDTO: một dòng bảng xếp hạng điểm thưởng. Người đặt `leaderboard_display = anonymous` hiện cho
// người khác (trừ chính họ và admin) với DisplayName "Học viên ẩn danh", KHÔNG có user_id, user_name, full_name,
// avatar_url: các field đó bị bỏ khỏi JSON (omitempty) chứ không để rỗng, vì id dẫn tới hồ sơ công khai.
// DisplayName luôn có và là nhãn web nên hiển thị.
type LeaderboardEntryDTO struct {
	Rank        int        `json:"rank"`
	UserID      *uuid.UUID `json:"user_id,omitempty"`
	UserName    string     `json:"user_name,omitempty"`
	FullName    *string    `json:"full_name,omitempty"`
	AvatarURL   *string    `json:"avatar_url,omitempty"`
	DisplayName string     `json:"display_name"`
	Points      int        `json:"points"`
	IsMe        bool       `json:"is_me"`
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
	TotalPoints          int `json:"total_points"`
	Level                int `json:"level"`
	LevelProgress        int `json:"level_progress"`
	CurrentStreak        int `json:"current_streak"`
	LongestStreak        int `json:"longest_streak"`
	TotalCheckins        int `json:"total_checkins"`
	AchievementCount     int `json:"achievement_count"`
	CoursesCompleted     int `json:"courses_completed"`
	LessonsCompleted     int `json:"lessons_completed"`
	TotalStudyTimeMinutes int `json:"total_study_time_minutes"`
}

type PublicProfileAchievementDTO struct {
	ID       uuid.UUID  `json:"id"`
	Name     string     `json:"name"`
	IconURL  *string    `json:"icon_url,omitempty"`
	BadgeURL *string    `json:"badge_url,omitempty"`
	Category string     `json:"category"`
	EarnedAt time.Time  `json:"earned_at"`
}

type PublicProfileActivityDTO struct {
	Date  string `json:"date"`
	Count int    `json:"count"`
}

type PublicProfileCompletedCourseDTO struct {
	ID           uuid.UUID `json:"id"`
	Title        string    `json:"title"`
	ThumbnailURL *string   `json:"thumbnail_url,omitempty"`
	CompletedAt  time.Time `json:"completed_at"`
}

type PublicProfileResponse struct {
	UserID               uuid.UUID                         `json:"user_id"`
	UserName             string                            `json:"user_name"`
	FullName             *string                           `json:"full_name,omitempty"`
	AvatarURL            *string                           `json:"avatar_url,omitempty"`
	Bio                  *string                           `json:"bio,omitempty"`
	JoinedAt             time.Time                         `json:"joined_at"`
	// IsPrivate (S6): chủ hồ sơ đặt riêng tư nên người xem chỉ nhận tên + avatar; mọi trường còn lại
	// (stats, bio, thành tích, hoạt động, khoá đã hoàn thành) để trống.
	IsPrivate            bool                              `json:"is_private"`
	Stats                UserStatsDTO                      `json:"stats"`
	FeaturedAchievements []PublicProfileAchievementDTO     `json:"featured_achievements"`
	Activity             []PublicProfileActivityDTO        `json:"activity"`
	CompletedCourses     []PublicProfileCompletedCourseDTO `json:"completed_courses"`
}
