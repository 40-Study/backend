-- =============================================================================
-- 40Study Gamification Schema
-- Platform: PostgreSQL
-- Description: Schema mở rộng cho hệ thống điểm, điểm danh, thành tích
-- =============================================================================

-- =============================================================================
-- 1. POINT SYSTEM - Hệ thống điểm
-- =============================================================================

-- Điểm tổng của user
CREATE TABLE user_points (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    user_id UUID UNIQUE NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    total_points INT DEFAULT 0,
    current_points INT DEFAULT 0, -- Điểm có thể tiêu
    lifetime_points INT DEFAULT 0, -- Tổng điểm từng kiếm
    level INT DEFAULT 1,
    level_progress INT DEFAULT 0 -- % tiến độ level
);

-- Lịch sử giao dịch điểm
CREATE TABLE point_transactions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    points INT NOT NULL, -- Dương = cộng, Âm = trừ
    type VARCHAR(30) NOT NULL CHECK (type IN (
        'lesson_complete', 'quiz_pass', 'course_complete', 
        'daily_checkin', 'streak_bonus', 'first_login', 
        'review_write', 'referral', 'redemption', 'admin_adjust'
    )),
    description VARCHAR(255),
    reference_id UUID, -- ID của lesson/quiz/course liên quan
    balance INT NOT NULL -- Số dư sau giao dịch
);

-- Quy tắc cộng điểm
CREATE TABLE point_rules (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    action_type VARCHAR(30) UNIQUE NOT NULL,
    points INT NOT NULL,
    description VARCHAR(255),
    is_active BOOLEAN DEFAULT TRUE
);

-- Seed dữ liệu mặc định cho point_rules
INSERT INTO point_rules (action_type, points, description) VALUES
('lesson_complete', 10, 'Hoàn thành một bài học'),
('quiz_pass', 20, 'Vượt qua bài kiểm tra'),
('course_complete', 100, 'Hoàn thành khóa học'),
('daily_checkin', 5, 'Điểm danh hàng ngày'),
('streak_bonus', 10, 'Bonus streak 7 ngày'),
('first_login', 50, 'Đăng nhập lần đầu'),
('review_write', 15, 'Viết đánh giá khóa học'),
('referral', 100, 'Giới thiệu bạn bè');

-- =============================================================================
-- 2. DAILY CHECK-IN - Điểm danh hàng ngày
-- =============================================================================

-- Lịch sử điểm danh
CREATE TABLE daily_checkins (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    checkin_date DATE NOT NULL,
    streak_count INT DEFAULT 1,
    points_earned INT DEFAULT 0,
    bonus_earned INT DEFAULT 0,
    UNIQUE (user_id, checkin_date)
);

-- Thông tin streak hiện tại
CREATE TABLE user_streaks (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    user_id UUID UNIQUE NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    current_streak INT DEFAULT 0,
    longest_streak INT DEFAULT 0,
    last_checkin_date DATE,
    total_checkins INT DEFAULT 0
);

-- =============================================================================
-- 3. ACHIEVEMENTS & BADGES - Thành tích
-- =============================================================================

-- Định nghĩa thành tích
CREATE TABLE achievements (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    name VARCHAR(100) NOT NULL,
    slug VARCHAR(100) UNIQUE NOT NULL,
    description TEXT,
    icon_url VARCHAR(500),
    badge_url VARCHAR(500),
    category VARCHAR(30) NOT NULL CHECK (category IN ('learning', 'streak', 'social', 'milestone', 'special')),
    points INT DEFAULT 0,
    requirement VARCHAR(50),
    threshold INT DEFAULT 1,
    is_hidden BOOLEAN DEFAULT FALSE,
    is_active BOOLEAN DEFAULT TRUE
);

-- Seed thành tích mặc định
INSERT INTO achievements (name, slug, description, category, points, requirement, threshold) VALUES
('Người mới bắt đầu', 'first-lesson', 'Hoàn thành bài học đầu tiên', 'learning', 10, 'complete_lessons', 1),
('Học viên chăm chỉ', 'complete-10-lessons', 'Hoàn thành 10 bài học', 'learning', 50, 'complete_lessons', 10),
('Học viên xuất sắc', 'complete-50-lessons', 'Hoàn thành 50 bài học', 'learning', 200, 'complete_lessons', 50),
('Bậc thầy học tập', 'complete-100-lessons', 'Hoàn thành 100 bài học', 'learning', 500, 'complete_lessons', 100),
('Khởi đầu mới', 'first-course', 'Hoàn thành khóa học đầu tiên', 'milestone', 100, 'complete_courses', 1),
('Người ham học', 'complete-5-courses', 'Hoàn thành 5 khóa học', 'milestone', 300, 'complete_courses', 5),
('Chuyên gia', 'complete-10-courses', 'Hoàn thành 10 khóa học', 'milestone', 500, 'complete_courses', 10),
('Streak 7 ngày', 'streak-7', 'Học liên tục 7 ngày', 'streak', 50, 'streak_days', 7),
('Streak 30 ngày', 'streak-30', 'Học liên tục 30 ngày', 'streak', 200, 'streak_days', 30),
('Streak 100 ngày', 'streak-100', 'Học liên tục 100 ngày', 'streak', 1000, 'streak_days', 100),
('Quiz Master', 'pass-10-quizzes', 'Vượt qua 10 bài quiz', 'learning', 100, 'pass_quizzes', 10),
('Perfect Score', 'perfect-quiz', 'Đạt điểm tuyệt đối trong quiz', 'special', 50, 'perfect_quiz', 1),
('Người đánh giá', 'first-review', 'Viết đánh giá khóa học đầu tiên', 'social', 20, 'write_reviews', 1);

-- Thành tích user đã đạt được
CREATE TABLE user_achievements (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    achievement_id UUID NOT NULL REFERENCES achievements(id) ON DELETE CASCADE,
    earned_at TIMESTAMP NOT NULL,
    progress INT DEFAULT 100,
    UNIQUE (user_id, achievement_id)
);

-- Tiến độ thành tích chưa hoàn thành
CREATE TABLE user_achievement_progress (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    achievement_id UUID NOT NULL REFERENCES achievements(id) ON DELETE CASCADE,
    current_value INT DEFAULT 0,
    target_value INT NOT NULL,
    UNIQUE (user_id, achievement_id)
);

-- =============================================================================
-- 4. LEADERBOARD - Bảng xếp hạng
-- =============================================================================

CREATE TABLE leaderboard_entries (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    period VARCHAR(20) NOT NULL, -- "2026-W03" hoặc "2026-01"
    period_type VARCHAR(10) NOT NULL CHECK (period_type IN ('weekly', 'monthly', 'all_time')),
    points INT DEFAULT 0,
    rank INT DEFAULT 0,
    lessons_completed INT DEFAULT 0,
    quizzes_completed INT DEFAULT 0,
    study_minutes INT DEFAULT 0
);

-- =============================================================================
-- 5. REWARDS & REDEMPTION - Đổi thưởng
-- =============================================================================

CREATE TABLE rewards (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    name VARCHAR(100) NOT NULL,
    description TEXT,
    image_url VARCHAR(500),
    points_cost INT NOT NULL,
    reward_type VARCHAR(30) NOT NULL CHECK (reward_type IN ('course_discount', 'free_course', 'certificate_badge', 'custom')),
    reward_value VARCHAR(255),
    stock INT,
    is_active BOOLEAN DEFAULT TRUE,
    expires_at TIMESTAMP
);

CREATE TABLE reward_redemptions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    reward_id UUID NOT NULL REFERENCES rewards(id) ON DELETE CASCADE,
    points_used INT NOT NULL,
    status VARCHAR(20) DEFAULT 'pending' CHECK (status IN ('pending', 'approved', 'delivered', 'cancelled')),
    code VARCHAR(50),
    used_at TIMESTAMP,
    notes TEXT
);

-- =============================================================================
-- 6. LEARNING GOALS - Mục tiêu học tập
-- =============================================================================

CREATE TABLE learning_goals (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    goal_type VARCHAR(30) NOT NULL CHECK (goal_type IN ('daily_study_time', 'weekly_lessons', 'monthly_courses', 'streak_days', 'quiz_score')),
    target_value INT NOT NULL,
    current_value INT DEFAULT 0,
    period VARCHAR(20),
    is_completed BOOLEAN DEFAULT FALSE,
    completed_at TIMESTAMP
);

-- Cài đặt cá nhân user
CREATE TABLE user_preferences (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    user_id UUID UNIQUE NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    daily_study_goal_minutes INT DEFAULT 30,
    weekly_lesson_goal INT DEFAULT 5,
    reminder_enabled BOOLEAN DEFAULT TRUE,
    reminder_time VARCHAR(5), -- "09:00"
    timezone VARCHAR(50) DEFAULT 'Asia/Ho_Chi_Minh',
    language VARCHAR(10) DEFAULT 'vi'
);

-- =============================================================================
-- INDEXES
-- =============================================================================

CREATE INDEX idx_user_points_user_id ON user_points(user_id);
CREATE INDEX idx_point_transactions_user_id ON point_transactions(user_id);
CREATE INDEX idx_point_transactions_created_at ON point_transactions(created_at);
CREATE INDEX idx_daily_checkins_user_id ON daily_checkins(user_id);
CREATE INDEX idx_daily_checkins_date ON daily_checkins(checkin_date);
CREATE INDEX idx_user_streaks_user_id ON user_streaks(user_id);
CREATE INDEX idx_user_achievements_user_id ON user_achievements(user_id);
CREATE INDEX idx_user_achievements_achievement_id ON user_achievements(achievement_id);
CREATE INDEX idx_leaderboard_period ON leaderboard_entries(period, period_type);
CREATE INDEX idx_leaderboard_points ON leaderboard_entries(points DESC);
CREATE INDEX idx_reward_redemptions_user_id ON reward_redemptions(user_id);
CREATE INDEX idx_learning_goals_user_id ON learning_goals(user_id);

-- =============================================================================
-- TRIGGERS
-- =============================================================================

-- Auto update updated_at
CREATE TRIGGER update_user_points_updated_at BEFORE UPDATE ON user_points FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();
CREATE TRIGGER update_point_rules_updated_at BEFORE UPDATE ON point_rules FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();
CREATE TRIGGER update_user_streaks_updated_at BEFORE UPDATE ON user_streaks FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();
CREATE TRIGGER update_achievements_updated_at BEFORE UPDATE ON achievements FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();
CREATE TRIGGER update_user_achievement_progress_updated_at BEFORE UPDATE ON user_achievement_progress FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();
CREATE TRIGGER update_rewards_updated_at BEFORE UPDATE ON rewards FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();
CREATE TRIGGER update_learning_goals_updated_at BEFORE UPDATE ON learning_goals FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();
CREATE TRIGGER update_user_preferences_updated_at BEFORE UPDATE ON user_preferences FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();
