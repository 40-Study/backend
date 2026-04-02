package repository

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"study.com/v1/internal/model"
)

// ============================================================
// Achievement Repository
// ============================================================

type AchievementRepository struct {
	db *gorm.DB
}

func NewAchievementRepository(db *gorm.DB) *AchievementRepository {
	return &AchievementRepository{db: db}
}

// ListActive returns all active (non-hidden) achievements
func (r *AchievementRepository) ListActive(ctx context.Context) ([]model.Achievement, error) {
	var achievements []model.Achievement
	err := r.db.WithContext(ctx).
		Where("is_active = true AND is_hidden = false").
		Order("category, name").
		Find(&achievements).Error
	return achievements, err
}

// GetByID returns a single achievement by id
func (r *AchievementRepository) GetByID(ctx context.Context, id uuid.UUID) (*model.Achievement, error) {
	var achievement model.Achievement
	err := r.db.WithContext(ctx).First(&achievement, "id = ?", id).Error
	if err == gorm.ErrRecordNotFound {
		return nil, nil
	}
	return &achievement, err
}

// GetUserAchievements returns all UserAchievement records for a user (preloads Achievement)
func (r *AchievementRepository) GetUserAchievements(ctx context.Context, userID uuid.UUID) ([]model.UserAchievement, error) {
	var userAchievements []model.UserAchievement
	err := r.db.WithContext(ctx).
		Preload("Achievement").
		Where("user_id = ?", userID).
		Order("earned_at DESC").
		Find(&userAchievements).Error
	return userAchievements, err
}

// HasAchievement checks if a user already owns an achievement
func (r *AchievementRepository) HasAchievement(ctx context.Context, userID, achievementID uuid.UUID) (bool, error) {
	var count int64
	err := r.db.WithContext(ctx).Model(&model.UserAchievement{}).
		Where("user_id = ? AND achievement_id = ?", userID, achievementID).
		Count(&count).Error
	return count > 0, err
}

// UnlockAchievement creates a UserAchievement row
func (r *AchievementRepository) UnlockAchievement(ctx context.Context, userID, achievementID uuid.UUID) (*model.UserAchievement, error) {
	ua := &model.UserAchievement{
		UserID:        userID,
		AchievementID: achievementID,
		EarnedAt:      time.Now(),
		Progress:      100,
	}
	err := r.db.WithContext(ctx).Create(ua).Error
	return ua, err
}

// ============================================================
// Leaderboard Repository
// ============================================================

type LeaderboardRepository struct {
	db *gorm.DB
}

func NewLeaderboardRepository(db *gorm.DB) *LeaderboardRepository {
	return &LeaderboardRepository{db: db}
}

// LeaderboardRow is a flattened result row
type LeaderboardRow struct {
	UserID   uuid.UUID `json:"user_id"`
	UserName string    `json:"user_name"`
	FullName *string   `json:"full_name"`
	AvatarURL *string  `json:"avatar_url"`
	Points   int       `json:"points"`
	Rank     int       `json:"rank"`
}

// GetTopUsers returns ranked users for the given period/type up to limit.
// For all_time it uses user_points.total_points directly.
func (r *LeaderboardRepository) GetTopUsers(ctx context.Context, periodType string, period string, limit int) ([]LeaderboardRow, error) {
	var rows []LeaderboardRow

	if periodType == "all_time" {
		sql := `
			SELECT
				u.id                                               AS user_id,
				u.user_name,
				u.full_name,
				u.avatar_url,
				COALESCE(up.total_points, 0)                       AS points,
				RANK() OVER (ORDER BY COALESCE(up.total_points, 0) DESC) AS rank
			FROM users u
			LEFT JOIN user_points up ON up.user_id = u.id
			WHERE u.is_active = true
			ORDER BY points DESC
			LIMIT ?`
		err := r.db.WithContext(ctx).Raw(sql, limit).Scan(&rows).Error
		return rows, err
	}

	// weekly or monthly – join leaderboard_entries
	sql := `
		SELECT
			u.id                                       AS user_id,
			u.user_name,
			u.full_name,
			u.avatar_url,
			COALESCE(le.points, 0)                     AS points,
			RANK() OVER (ORDER BY COALESCE(le.points, 0) DESC) AS rank
		FROM users u
		INNER JOIN leaderboard_entries le ON le.user_id = u.id
			AND le.period_type = ?
			AND le.period     = ?
		WHERE u.is_active = true
		ORDER BY points DESC
		LIMIT ?`
	err := r.db.WithContext(ctx).Raw(sql, periodType, period, limit).Scan(&rows).Error
	return rows, err
}

// GetUserRank returns the rank and points for a single user
func (r *LeaderboardRepository) GetUserRank(ctx context.Context, userID uuid.UUID, periodType string, period string) (*LeaderboardRow, error) {
	var row LeaderboardRow

	if periodType == "all_time" {
		sql := `
			SELECT sub.user_id, sub.user_name, sub.full_name, sub.avatar_url, sub.points, sub.rank
			FROM (
				SELECT
					u.id                                               AS user_id,
					u.user_name,
					u.full_name,
					u.avatar_url,
					COALESCE(up.total_points, 0)                       AS points,
					RANK() OVER (ORDER BY COALESCE(up.total_points, 0) DESC) AS rank
				FROM users u
				LEFT JOIN user_points up ON up.user_id = u.id
				WHERE u.is_active = true
			) sub
			WHERE sub.user_id = ?`
		err := r.db.WithContext(ctx).Raw(sql, userID).Scan(&row).Error
		if err != nil {
			return nil, err
		}
		if row.UserID == uuid.Nil {
			return nil, nil
		}
		return &row, nil
	}

	sql := `
		SELECT sub.user_id, sub.user_name, sub.full_name, sub.avatar_url, sub.points, sub.rank
		FROM (
			SELECT
				u.id                                       AS user_id,
				u.user_name,
				u.full_name,
				u.avatar_url,
				COALESCE(le.points, 0)                     AS points,
				RANK() OVER (ORDER BY COALESCE(le.points, 0) DESC) AS rank
			FROM users u
			INNER JOIN leaderboard_entries le ON le.user_id = u.id
				AND le.period_type = ?
				AND le.period     = ?
			WHERE u.is_active = true
		) sub
		WHERE sub.user_id = ?`
	err := r.db.WithContext(ctx).Raw(sql, periodType, period, userID).Scan(&row).Error
	if err != nil {
		return nil, err
	}
	if row.UserID == uuid.Nil {
		return nil, nil
	}
	return &row, nil
}

// ============================================================
// User Stats Repository
// ============================================================

type UserStatsRepository struct {
	db *gorm.DB
}

func NewUserStatsRepository(db *gorm.DB) *UserStatsRepository {
	return &UserStatsRepository{db: db}
}

type PublicProfileRow struct {
	UserID    uuid.UUID `json:"user_id"`
	UserName  string    `json:"user_name"`
	FullName  *string   `json:"full_name"`
	AvatarURL *string   `json:"avatar_url"`
	Bio       *string   `json:"bio"`
	// stats
	TotalPoints    int `json:"total_points"`
	Level          int `json:"level"`
	LevelProgress  int `json:"level_progress"`
	CurrentStreak  int `json:"current_streak"`
	LongestStreak  int `json:"longest_streak"`
	TotalCheckins  int `json:"total_checkins"`
	AchievementCount int `json:"achievement_count"`
}

func (r *UserStatsRepository) GetPublicProfile(ctx context.Context, userID uuid.UUID) (*PublicProfileRow, error) {
	var row PublicProfileRow
	sql := `
		SELECT
			u.id                              AS user_id,
			u.user_name,
			u.full_name,
			u.avatar_url,
			u.bio,
			COALESCE(up.total_points, 0)      AS total_points,
			COALESCE(up.level, 1)             AS level,
			COALESCE(up.level_progress, 0)    AS level_progress,
			COALESCE(us.current_streak, 0)    AS current_streak,
			COALESCE(us.longest_streak, 0)    AS longest_streak,
			COALESCE(us.total_checkins, 0)    AS total_checkins,
			COUNT(ua.id)                      AS achievement_count
		FROM users u
		LEFT JOIN user_points  up ON up.user_id = u.id
		LEFT JOIN user_streaks us ON us.user_id = u.id
		LEFT JOIN user_achievements ua ON ua.user_id = u.id
		WHERE u.id = ? AND u.is_active = true
		GROUP BY u.id, up.total_points, up.level, up.level_progress,
		         us.current_streak, us.longest_streak, us.total_checkins`
	err := r.db.WithContext(ctx).Raw(sql, userID).Scan(&row).Error
	if err != nil {
		return nil, err
	}
	if row.UserID == uuid.Nil {
		return nil, nil
	}
	return &row, nil
}

// ============================================================
// Period helpers
// ============================================================

// CurrentPeriod returns the period string for the given period_type
func CurrentPeriod(periodType string) string {
	now := time.Now()
	switch periodType {
	case "weekly":
		year, week := now.ISOWeek()
		return fmt.Sprintf("%d-W%02d", year, week)
	case "monthly":
		return now.Format("2006-01")
	default:
		return "all_time"
	}
}
