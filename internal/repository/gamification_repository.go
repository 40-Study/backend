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
	UserID                uuid.UUID `json:"user_id"`
	UserName              string    `json:"user_name"`
	FullName              *string   `json:"full_name"`
	AvatarURL             *string   `json:"avatar_url"`
	Bio                   *string   `json:"bio"`
	JoinedAt              time.Time `json:"joined_at"`
	TotalPoints           int       `json:"total_points"`
	Level                 int       `json:"level"`
	LevelProgress         int       `json:"level_progress"`
	CurrentStreak         int       `json:"current_streak"`
	LongestStreak         int       `json:"longest_streak"`
	TotalCheckins         int       `json:"total_checkins"`
	AchievementCount      int       `json:"achievement_count"`
	CoursesCompleted      int       `json:"courses_completed"`
	LessonsCompleted      int       `json:"lessons_completed"`
	TotalStudyTimeMinutes int       `json:"total_study_time_minutes"`
}

type PublicProfileAchievementRow struct {
	ID       uuid.UUID `json:"id"`
	Name     string    `json:"name"`
	IconURL  *string   `json:"icon_url"`
	BadgeURL *string   `json:"badge_url"`
	Category string    `json:"category"`
	EarnedAt time.Time `json:"earned_at"`
}

type PublicProfileActivityRow struct {
	Date  time.Time `json:"date"`
	Count int       `json:"count"`
}

type PublicProfileCompletedCourseRow struct {
	ID           uuid.UUID `json:"id"`
	Title        string    `json:"title"`
	ThumbnailURL *string   `json:"thumbnail_url"`
	CompletedAt  time.Time `json:"completed_at"`
}

func (r *UserStatsRepository) GetPublicProfile(ctx context.Context, userID uuid.UUID) (*PublicProfileRow, error) {
	var row PublicProfileRow
	sql := `
		SELECT
			u.id AS user_id,
			u.user_name,
			u.full_name,
			u.avatar_url,
			u.bio,
			u.created_at AS joined_at,
			COALESCE(up.total_points, 0) AS total_points,
			COALESCE(up.level, 1) AS level,
			COALESCE(up.level_progress, 0) AS level_progress,
			COALESCE(us.current_streak, 0) AS current_streak,
			COALESCE(us.longest_streak, 0) AS longest_streak,
			COALESCE(us.total_checkins, 0) AS total_checkins,
			COALESCE(achievement_stats.achievement_count, 0) AS achievement_count,
			COALESCE(enrollment_stats.courses_completed, 0) AS courses_completed,
			COALESCE(lesson_stats.lessons_completed, 0) AS lessons_completed,
			COALESCE(lesson_stats.total_study_time_minutes, 0) AS total_study_time_minutes
		FROM users u
		LEFT JOIN user_points up ON up.user_id = u.id
		LEFT JOIN user_streaks us ON us.user_id = u.id
		LEFT JOIN (
			SELECT ua.user_id, COUNT(*) AS achievement_count
			FROM user_achievements ua
			GROUP BY ua.user_id
		) achievement_stats ON achievement_stats.user_id = u.id
		LEFT JOIN (
			SELECT e.user_id, COUNT(*) AS courses_completed
			FROM enrollments e
			WHERE e.completed_at IS NOT NULL
			GROUP BY e.user_id
		) enrollment_stats ON enrollment_stats.user_id = u.id
		LEFT JOIN (
			SELECT lp.user_id,
				COUNT(*) FILTER (WHERE lp.status = 'completed') AS lessons_completed,
				COALESCE(FLOOR(SUM(lp.video_watched_seconds) / 60.0), 0)::int AS total_study_time_minutes
			FROM lesson_progress lp
			GROUP BY lp.user_id
		) lesson_stats ON lesson_stats.user_id = u.id
		WHERE u.id = ? AND u.is_active = true`
	err := r.db.WithContext(ctx).Raw(sql, userID).Scan(&row).Error
	if err != nil {
		return nil, err
	}
	if row.UserID == uuid.Nil {
		return nil, nil
	}
	return &row, nil
}

func (r *UserStatsRepository) GetPublicProfileAchievements(ctx context.Context, userID uuid.UUID, limit int) ([]PublicProfileAchievementRow, error) {
	var rows []PublicProfileAchievementRow
	sql := `
		SELECT
			a.id,
			a.name,
			a.icon_url,
			a.badge_url,
			a.category,
			ua.earned_at
		FROM user_achievements ua
		JOIN achievements a ON a.id = ua.achievement_id
		WHERE ua.user_id = ?
		ORDER BY ua.earned_at DESC
		LIMIT ?`
	err := r.db.WithContext(ctx).Raw(sql, userID, limit).Scan(&rows).Error
	return rows, err
}

func (r *UserStatsRepository) GetPublicProfileActivity(ctx context.Context, userID uuid.UUID, days int) ([]PublicProfileActivityRow, error) {
	var rows []PublicProfileActivityRow
	sql := `
		SELECT
			DATE(lp.completed_at) AS date,
			COUNT(*) AS count
		FROM lesson_progress lp
		WHERE lp.user_id = ?
			AND lp.status = 'completed'
			AND lp.completed_at IS NOT NULL
			AND lp.completed_at >= NOW() - (? * INTERVAL '1 day')
		GROUP BY DATE(lp.completed_at)
		ORDER BY DATE(lp.completed_at) ASC`
	err := r.db.WithContext(ctx).Raw(sql, userID, days).Scan(&rows).Error
	return rows, err
}

func (r *UserStatsRepository) GetPublicProfileCompletedCourses(ctx context.Context, userID uuid.UUID, limit int) ([]PublicProfileCompletedCourseRow, error) {
	var rows []PublicProfileCompletedCourseRow
	sql := `
		SELECT
			c.id,
			c.title,
			c.thumbnail_url,
			e.completed_at
		FROM enrollments e
		JOIN courses c ON c.id = e.course_id
		WHERE e.user_id = ?
			AND e.completed_at IS NOT NULL
		ORDER BY e.completed_at DESC
		LIMIT ?`
	err := r.db.WithContext(ctx).Raw(sql, userID, limit).Scan(&rows).Error
	return rows, err
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
