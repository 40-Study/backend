package model

import (
	"time"

	"github.com/google/uuid"
)

// ============================================================================
// POINT SYSTEM - Hệ thống điểm thưởng
// ============================================================================

// UserPoint lưu tổng điểm hiện tại của user
type UserPoint struct {
	ID             uuid.UUID `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
	UserID         uuid.UUID `gorm:"type:uuid;uniqueIndex;not null" json:"user_id"`
	TotalPoints    int       `gorm:"default:0" json:"total_points"`
	CurrentPoints  int       `gorm:"default:0" json:"current_points"`  // Điểm có thể tiêu
	LifetimePoints int       `gorm:"default:0" json:"lifetime_points"` // Tổng điểm từng kiếm được
	Level          int       `gorm:"default:1" json:"level"`
	LevelProgress  int       `gorm:"default:0" json:"level_progress"` // % tiến độ level hiện tại

	// Relationships
	User         User               `gorm:"foreignKey:UserID;constraint:OnDelete:CASCADE" json:"-"`
	Transactions []PointTransaction `gorm:"foreignKey:UserID" json:"-"`
}

func (UserPoint) TableName() string {
	return "user_points"
}

// PointTransaction ghi lại lịch sử cộng/trừ điểm
type PointTransaction struct {
	ID          uuid.UUID  `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	CreatedAt   time.Time  `json:"created_at"`
	UserID      uuid.UUID  `gorm:"type:uuid;not null;index" json:"user_id"`
	Points      int        `gorm:"not null" json:"points"` // Dương = cộng, Âm = trừ
	Type        string     `gorm:"type:varchar(30);not null;check:type IN ('lesson_complete', 'quiz_pass', 'course_complete', 'daily_checkin', 'streak_bonus', 'first_login', 'review_write', 'referral', 'redemption', 'admin_adjust')" json:"type"`
	Description string     `gorm:"type:varchar(255)" json:"description"`
	ReferenceID *uuid.UUID `gorm:"type:uuid" json:"reference_id,omitempty"` // ID của lesson/quiz/course liên quan
	Balance     int        `gorm:"not null" json:"balance"`                 // Số dư sau giao dịch

	// Relationships
	User User `gorm:"foreignKey:UserID;constraint:OnDelete:CASCADE" json:"-"`
}

func (PointTransaction) TableName() string {
	return "point_transactions"
}

// PointRule định nghĩa rule cộng điểm
type PointRule struct {
	ID          uuid.UUID `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
	ActionType  string    `gorm:"type:varchar(30);uniqueIndex;not null" json:"action_type"` // lesson_complete, quiz_pass, etc.
	Points      int       `gorm:"not null" json:"points"`
	Description string    `gorm:"type:varchar(255)" json:"description"`
	IsActive    bool      `gorm:"default:true" json:"is_active"`
}

func (PointRule) TableName() string {
	return "point_rules"
}

// ============================================================================
// DAILY CHECK-IN - Hệ thống điểm danh hàng ngày
// ============================================================================

// DailyCheckin lưu lịch sử điểm danh của user
type DailyCheckin struct {
	ID           uuid.UUID `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	CreatedAt    time.Time `json:"created_at"`
	UserID       uuid.UUID `gorm:"type:uuid;not null;index" json:"user_id"`
	CheckinDate  time.Time `gorm:"type:date;not null;index;uniqueIndex:idx_user_checkin_date,priority:2" json:"checkin_date"`
	StreakCount  int       `gorm:"default:1" json:"streak_count"` // Số ngày liên tiếp tính đến ngày này
	PointsEarned int       `gorm:"default:0" json:"points_earned"`
	BonusEarned  int       `gorm:"default:0" json:"bonus_earned"` // Bonus từ streak

	// Relationships
	User User `gorm:"foreignKey:UserID;constraint:OnDelete:CASCADE;uniqueIndex:idx_user_checkin_date,priority:1" json:"-"`
}

func (DailyCheckin) TableName() string {
	return "daily_checkins"
}

// UserStreak lưu thông tin streak hiện tại của user
type UserStreak struct {
	ID              uuid.UUID  `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	CreatedAt       time.Time  `json:"created_at"`
	UpdatedAt       time.Time  `json:"updated_at"`
	UserID          uuid.UUID  `gorm:"type:uuid;uniqueIndex;not null" json:"user_id"`
	CurrentStreak   int        `gorm:"default:0" json:"current_streak"` // Streak hiện tại
	LongestStreak   int        `gorm:"default:0" json:"longest_streak"` // Streak dài nhất từng đạt
	LastCheckinDate *time.Time `gorm:"type:date" json:"last_checkin_date,omitempty"`
	TotalCheckins   int        `gorm:"default:0" json:"total_checkins"`

	// Relationships
	User User `gorm:"foreignKey:UserID;constraint:OnDelete:CASCADE" json:"-"`
}

func (UserStreak) TableName() string {
	return "user_streaks"
}

// ============================================================================
// ACHIEVEMENTS & BADGES - Hệ thống thành tích
// ============================================================================

// Achievement định nghĩa các thành tích có thể đạt được
type Achievement struct {
	ID          uuid.UUID `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
	Name        string    `gorm:"type:varchar(100);not null" json:"name"`
	Slug        string    `gorm:"type:varchar(100);uniqueIndex;not null" json:"slug"`
	Description string    `gorm:"type:text" json:"description"`
	IconURL     *string   `gorm:"type:varchar(500)" json:"icon_url,omitempty"`
	BadgeURL    *string   `gorm:"type:varchar(500)" json:"badge_url,omitempty"`
	Category    string    `gorm:"type:varchar(30);not null;check:category IN ('learning', 'streak', 'social', 'milestone', 'special')" json:"category"`
	Points      int       `gorm:"default:0" json:"points"`             // Điểm thưởng khi đạt được
	Requirement string    `gorm:"type:varchar(50)" json:"requirement"` // Ví dụ: "complete_5_courses", "streak_30_days"
	Threshold   int       `gorm:"default:1" json:"threshold"`          // Số lượng cần đạt
	IsHidden    bool      `gorm:"default:false" json:"is_hidden"`      // Thành tích ẩn
	IsActive    bool      `gorm:"default:true" json:"is_active"`

	// Relationships
	UserAchievements []UserAchievement `gorm:"foreignKey:AchievementID" json:"-"`
}

func (Achievement) TableName() string {
	return "achievements"
}

// UserAchievement lưu thành tích user đã đạt được
type UserAchievement struct {
	ID            uuid.UUID `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	CreatedAt     time.Time `json:"created_at"`
	UserID        uuid.UUID `gorm:"type:uuid;not null;index;uniqueIndex:idx_user_achievement,priority:1" json:"user_id"`
	AchievementID uuid.UUID `gorm:"type:uuid;not null;index;uniqueIndex:idx_user_achievement,priority:2" json:"achievement_id"`
	EarnedAt      time.Time `gorm:"not null" json:"earned_at"`
	Progress      int       `gorm:"default:100" json:"progress"` // 100 = đã hoàn thành

	// Relationships
	User        User        `gorm:"foreignKey:UserID;constraint:OnDelete:CASCADE" json:"-"`
	Achievement Achievement `gorm:"foreignKey:AchievementID;constraint:OnDelete:CASCADE" json:"-"`
}

func (UserAchievement) TableName() string {
	return "user_achievements"
}

// UserAchievementProgress theo dõi tiến độ thành tích chưa hoàn thành
type UserAchievementProgress struct {
	ID            uuid.UUID `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
	UserID        uuid.UUID `gorm:"type:uuid;not null;index;uniqueIndex:idx_user_achievement_progress,priority:1" json:"user_id"`
	AchievementID uuid.UUID `gorm:"type:uuid;not null;index;uniqueIndex:idx_user_achievement_progress,priority:2" json:"achievement_id"`
	CurrentValue  int       `gorm:"default:0" json:"current_value"` // Giá trị hiện tại
	TargetValue   int       `gorm:"not null" json:"target_value"`   // Giá trị cần đạt

	// Relationships
	User        User        `gorm:"foreignKey:UserID;constraint:OnDelete:CASCADE" json:"-"`
	Achievement Achievement `gorm:"foreignKey:AchievementID;constraint:OnDelete:CASCADE" json:"-"`
}

func (UserAchievementProgress) TableName() string {
	return "user_achievement_progress"
}

// ============================================================================
// LEADERBOARD - Bảng xếp hạng
// ============================================================================

// LeaderboardEntry lưu thứ hạng theo tuần/tháng
type LeaderboardEntry struct {
	ID               uuid.UUID `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	CreatedAt        time.Time `json:"created_at"`
	UserID           uuid.UUID `gorm:"type:uuid;not null;index" json:"user_id"`
	Period           string    `gorm:"type:varchar(20);not null;index" json:"period"` // "2026-W03" (week) hoặc "2026-01" (month)
	PeriodType       string    `gorm:"type:varchar(10);not null;check:period_type IN ('weekly', 'monthly', 'all_time')" json:"period_type"`
	Points           int       `gorm:"default:0;index" json:"points"`
	Rank             int       `gorm:"default:0" json:"rank"`
	LessonsCompleted int       `gorm:"default:0" json:"lessons_completed"`
	QuizzesCompleted int       `gorm:"default:0" json:"quizzes_completed"`
	StudyMinutes     int       `gorm:"default:0" json:"study_minutes"`

	// Relationships
	User User `gorm:"foreignKey:UserID;constraint:OnDelete:CASCADE" json:"-"`
}

func (LeaderboardEntry) TableName() string {
	return "leaderboard_entries"
}

// ============================================================================
// REWARDS & REDEMPTION - Phần thưởng và đổi điểm
// ============================================================================

// Reward định nghĩa các phần thưởng có thể đổi
type Reward struct {
	ID          uuid.UUID  `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
	Name        string     `gorm:"type:varchar(100);not null" json:"name"`
	Description string     `gorm:"type:text" json:"description"`
	ImageURL    *string    `gorm:"type:varchar(500)" json:"image_url,omitempty"`
	PointsCost  int        `gorm:"not null" json:"points_cost"`
	RewardType  string     `gorm:"type:varchar(30);not null;check:reward_type IN ('course_discount', 'free_course', 'certificate_badge', 'custom')" json:"reward_type"`
	RewardValue *string    `gorm:"type:varchar(255)" json:"reward_value,omitempty"` // Giá trị cụ thể (ví dụ: "10%" cho discount)
	Stock       *int       `json:"stock,omitempty"`                                 // NULL = unlimited
	IsActive    bool       `gorm:"default:true" json:"is_active"`
	ExpiresAt   *time.Time `json:"expires_at,omitempty"`

	// Relationships
	Redemptions []RewardRedemption `gorm:"foreignKey:RewardID" json:"-"`
}

func (Reward) TableName() string {
	return "rewards"
}

// RewardRedemption lưu lịch sử đổi thưởng
type RewardRedemption struct {
	ID         uuid.UUID  `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	CreatedAt  time.Time  `json:"created_at"`
	UserID     uuid.UUID  `gorm:"type:uuid;not null;index" json:"user_id"`
	RewardID   uuid.UUID  `gorm:"type:uuid;not null;index" json:"reward_id"`
	PointsUsed int        `gorm:"not null" json:"points_used"`
	Status     string     `gorm:"type:varchar(20);default:'pending';check:status IN ('pending', 'approved', 'delivered', 'cancelled')" json:"status"`
	Code       *string    `gorm:"type:varchar(50)" json:"code,omitempty"` // Mã voucher/discount nếu có
	UsedAt     *time.Time `json:"used_at,omitempty"`
	Notes      *string    `gorm:"type:text" json:"notes,omitempty"`

	// Relationships
	User   User   `gorm:"foreignKey:UserID;constraint:OnDelete:CASCADE" json:"-"`
	Reward Reward `gorm:"foreignKey:RewardID;constraint:OnDelete:CASCADE" json:"-"`
}

func (RewardRedemption) TableName() string {
	return "reward_redemptions"
}

// ============================================================================
// LEARNING GOALS - Mục tiêu học tập
// ============================================================================

// LearningGoal lưu mục tiêu học tập của user
type LearningGoal struct {
	ID           uuid.UUID  `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	CreatedAt    time.Time  `json:"created_at"`
	UpdatedAt    time.Time  `json:"updated_at"`
	UserID       uuid.UUID  `gorm:"type:uuid;not null;index" json:"user_id"`
	GoalType     string     `gorm:"type:varchar(30);not null;check:goal_type IN ('daily_study_time', 'weekly_lessons', 'monthly_courses', 'streak_days', 'quiz_score')" json:"goal_type"`
	TargetValue  int        `gorm:"not null" json:"target_value"`
	CurrentValue int        `gorm:"default:0" json:"current_value"`
	Period       string     `gorm:"type:varchar(20)" json:"period,omitempty"` // "2026-01-21" cho daily, "2026-W03" cho weekly
	IsCompleted  bool       `gorm:"default:false" json:"is_completed"`
	CompletedAt  *time.Time `json:"completed_at,omitempty"`

	// Relationships
	User User `gorm:"foreignKey:UserID;constraint:OnDelete:CASCADE" json:"-"`
}

func (LearningGoal) TableName() string {
	return "learning_goals"
}

// UserPreference lưu các cài đặt cá nhân của user
type UserPreference struct {
	ID                    uuid.UUID `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	CreatedAt             time.Time `json:"created_at"`
	UpdatedAt             time.Time `json:"updated_at"`
	UserID                uuid.UUID `gorm:"type:uuid;uniqueIndex;not null" json:"user_id"`
	DailyStudyGoalMinutes int       `gorm:"default:30" json:"daily_study_goal_minutes"`
	WeeklyLessonGoal      int       `gorm:"default:5" json:"weekly_lesson_goal"`
	ReminderEnabled       bool      `gorm:"default:true" json:"reminder_enabled"`
	ReminderTime          *string   `gorm:"type:varchar(5)" json:"reminder_time,omitempty"` // "09:00"
	Timezone              string    `gorm:"type:varchar(50);default:'Asia/Ho_Chi_Minh'" json:"timezone"`
	Language              string    `gorm:"type:varchar(10);default:'vi'" json:"language"`

	// Relationships
	User User `gorm:"foreignKey:UserID;constraint:OnDelete:CASCADE" json:"-"`
}

func (UserPreference) TableName() string {
	return "user_preferences"
}
