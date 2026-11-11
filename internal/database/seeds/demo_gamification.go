package seeds

import (
	"fmt"
	"sort"
	"time"

	"study.com/v1/internal/model"
	"study.com/v1/internal/repository"
)

// demoPointsPerLevel: codebase chưa có công thức level nào (không service nào ghi Level), nên seed
// dùng quy ước đơn giản 500 điểm/level để level và level_progress khớp total_points.
const demoPointsPerLevel = 500

var demoAchievements = []model.Achievement{
	{Slug: "buoc-chan-dau-tien", Name: "Bước chân đầu tiên", Description: "Hoàn thành bài học đầu tiên trên 40Study.", Category: "learning", Points: 50, Requirement: "complete_1_lesson", Threshold: 1},
	{Slug: "hoc-vien-cham-chi", Name: "Học viên chăm chỉ", Description: "Hoàn thành 20 bài học bất kỳ.", Category: "learning", Points: 200, Requirement: "complete_20_lessons", Threshold: 20},
	{Slug: "bac-thay-trac-nghiem", Name: "Bậc thầy trắc nghiệm", Description: "Vượt qua 5 bài kiểm tra với điểm đạt.", Category: "learning", Points: 200, Requirement: "pass_5_quizzes", Threshold: 5},
	{Slug: "hoan-thanh-khoa-dau-tien", Name: "Về đích lần đầu", Description: "Hoàn thành trọn vẹn khoá học đầu tiên và nhận chứng chỉ.", Category: "milestone", Points: 300, Requirement: "complete_1_course", Threshold: 1},
	{Slug: "chuoi-7-ngay", Name: "Chuỗi 7 ngày", Description: "Học liên tục 7 ngày không nghỉ.", Category: "streak", Points: 100, Requirement: "streak_7_days", Threshold: 7},
	{Slug: "chuoi-30-ngay", Name: "Kiên trì 30 ngày", Description: "Giữ chuỗi học tập suốt 30 ngày liền.", Category: "streak", Points: 1000, Requirement: "streak_30_days", Threshold: 30},
	{Slug: "nguoi-danh-gia", Name: "Người góp ý", Description: "Viết đánh giá đầu tiên cho một khoá học đã học.", Category: "social", Points: 50, Requirement: "write_1_review", Threshold: 1},
	{Slug: "cu-dem", Name: "Cú đêm", Description: "Học sau 22 giờ trong 5 buổi khác nhau.", Category: "special", Points: 100, Requirement: "study_after_22h", Threshold: 5},
	// Thành tích ẩn: không hiện trong danh mục (ListActive lọc is_hidden=false) — minh hoạ bộ lọc.
	{Slug: "bi-mat-40study", Name: "Nhà thám hiểm", Description: "Khám phá tính năng ẩn của 40Study.", Category: "special", Points: 150, Requirement: "secret_explorer", Threshold: 1, IsHidden: true},
}

// demoLearnerStat: số liệu điểm của một học viên demo. Điểm tuần/tháng là phần của total_points kiếm được trong kỳ hiện
// tại. Chuỗi ngày, số bài đã học và huy hiệu KHÔNG khai ở đây: chúng suy ra từ dữ liệu học thật (demo_learning_facts.go),
// để không thể lệch với widget hoạt động/tiến độ (QA-A-04).
type demoLearnerStat struct {
	Email         string
	TotalPoints   int
	WeeklyPoints  int
	MonthlyPoints int
	TotalCheckins int
	Quizzes       int
	StudyMinutes  int
}

var demoLearnerStats = []demoLearnerStat{
	{Email: "student1@demo.com", TotalPoints: 1850, WeeklyPoints: 260, MonthlyPoints: 940, TotalCheckins: 48, Quizzes: 3, StudyMinutes: 540},
	{Email: "student3@demo.com", TotalPoints: 1420, WeeklyPoints: 310, MonthlyPoints: 820, TotalCheckins: 37, Quizzes: 6, StudyMinutes: 610},
	{Email: "student2@demo.com", TotalPoints: 980, WeeklyPoints: 120, MonthlyPoints: 450, TotalCheckins: 16, Quizzes: 1, StudyMinutes: 180},
	{Email: "student4@demo.com", TotalPoints: 760, WeeklyPoints: 180, MonthlyPoints: 390, TotalCheckins: 20, Quizzes: 2, StudyMinutes: 260},
	{Email: "student5@demo.com", TotalPoints: 430, WeeklyPoints: 70, MonthlyPoints: 210, TotalCheckins: 8, Quizzes: 1, StudyMinutes: 120},
}

var demoRewards = []model.Reward{
	{Name: "Giảm 10% một khoá học", Description: "Mã giảm 10% áp dụng cho một khoá học bất kỳ.", PointsCost: 500, RewardType: "course_discount", RewardValue: ptr("10%")},
	{Name: "Giảm 25% một khoá học", Description: "Mã giảm 25% cho khoá học từ 500.000đ.", PointsCost: 1200, RewardType: "course_discount", RewardValue: ptr("25%"), Stock: ptr(50)},
	{Name: "Mở khoá miễn phí Git & GitHub", Description: "Đổi điểm lấy khoá Git & GitHub cho người mới bắt đầu.", PointsCost: 800, RewardType: "free_course", RewardValue: ptr("git-github-cho-nguoi-moi-bat-dau")},
	{Name: "Khung huy hiệu Vàng", Description: "Khung viền vàng cho huy hiệu trên hồ sơ công khai.", PointsCost: 300, RewardType: "certificate_badge", RewardValue: ptr("gold-frame")},
}

// SeedDemoGamification seed danh mục thành tích, thành tích đã mở khoá, user_points, streak,
// leaderboard_entries (tuần/tháng hiện tại) và danh mục phần thưởng — dữ liệu cho trang
// (app)/achievements (+ hồ sơ công khai) và (app)/leaderboard (xếp theo user_points.total_points).
func (s *Seeder) SeedDemoGamification(users map[string]model.User) error {
	achievements := make(map[string]model.Achievement, len(demoAchievements))
	for _, a := range demoAchievements {
		record := a
		record.IsActive = true
		if err := s.db.Where("slug = ?", a.Slug).Attrs(record).FirstOrCreate(&record).Error; err != nil {
			return fmt.Errorf("failed to seed achievement %s: %w", a.Slug, err)
		}
		achievements[a.Slug] = record
	}

	now := time.Now()
	facts := make(map[string]demoLearnerFacts, len(demoLearnerStats))
	for _, st := range demoLearnerStats {
		user, err := demoUser(users, st.Email)
		if err != nil {
			return err
		}
		f, err := s.loadDemoLearnerFacts(user.ID)
		if err != nil {
			return err
		}
		facts[st.Email] = f
		if err := s.seedLearnerPointsAndStreak(user, st, f, now); err != nil {
			return err
		}
		// Duyệt theo demoAchievements (không theo map) để thứ tự ghi ổn định giữa các lần chạy.
		for _, a := range demoAchievements {
			earnedAt, ok := demoAchievementEarnedAt(a, f)
			if !ok {
				continue
			}
			ua := model.UserAchievement{UserID: user.ID, AchievementID: achievements[a.Slug].ID, EarnedAt: earnedAt, Progress: 100}
			if err := s.db.Where("user_id = ? AND achievement_id = ?", user.ID, ua.AchievementID).
				Attrs(ua).FirstOrCreate(&ua).Error; err != nil {
				return fmt.Errorf("failed to seed user achievement %s for %s: %w", a.Slug, st.Email, err)
			}
		}
	}

	if err := s.seedDemoLeaderboardEntries(users, facts); err != nil {
		return err
	}

	for _, r := range demoRewards {
		record := r
		record.IsActive = true
		if err := s.db.Where("name = ?", r.Name).Attrs(record).FirstOrCreate(&record).Error; err != nil {
			return fmt.Errorf("failed to seed reward %s: %w", r.Name, err)
		}
	}
	return nil
}

// seedLearnerPointsAndStreak tạo user_points và user_streaks (unique theo user_id). Chuỗi ngày tính từ ngày hoàn thành
// bài thật và được GHI ĐÈ mỗi lần chạy (Assign): đây là số dẫn xuất, và chưa service nào cập nhật user_streaks, nên chạy
// lại seed là cách duy nhất để nó khớp lại với lịch sử học sau khi thời gian trôi.
func (s *Seeder) seedLearnerPointsAndStreak(user model.User, st demoLearnerStat, f demoLearnerFacts, now time.Time) error {
	points := model.UserPoint{
		UserID:         user.ID,
		TotalPoints:    st.TotalPoints,
		CurrentPoints:  st.TotalPoints,
		LifetimePoints: st.TotalPoints,
		Level:          st.TotalPoints/demoPointsPerLevel + 1,
		LevelProgress:  st.TotalPoints % demoPointsPerLevel * 100 / demoPointsPerLevel,
	}
	if err := s.db.Where("user_id = ?", user.ID).Attrs(points).FirstOrCreate(&points).Error; err != nil {
		return fmt.Errorf("failed to seed user points for %s: %w", user.Email, err)
	}

	current, longest := demoStreaks(now, f.Lessons)
	days := demoActivityDays(f.Lessons)
	var lastCheckin *time.Time
	if len(days) > 0 {
		lastCheckin = ptr(days[len(days)-1])
	}
	// Lượt điểm danh không thể ít hơn số ngày đã học.
	checkins := max(st.TotalCheckins, len(days))
	streak := model.UserStreak{UserID: user.ID}
	if err := s.db.Where("user_id = ?", user.ID).
		Assign(map[string]interface{}{"current_streak": current, "longest_streak": longest,
			"last_checkin_date": lastCheckin, "total_checkins": checkins}).
		FirstOrCreate(&streak).Error; err != nil {
		return fmt.Errorf("failed to seed user streak for %s: %w", user.Email, err)
	}
	return nil
}

// seedDemoLeaderboardEntries tạo bảng xếp hạng tuần/tháng của KỲ HIỆN TẠI (định dạng period lấy từ
// repository.CurrentPeriod — cùng hàm API dùng). Trang web hiện luôn xếp all_time (lỗi tham số
// period_type/period phía handler), nhưng dữ liệu này sẵn sàng khi nút Tuần/Tháng được sửa.
func (s *Seeder) seedDemoLeaderboardEntries(users map[string]model.User, facts map[string]demoLearnerFacts) error {
	for _, periodType := range []string{"weekly", "monthly"} {
		period := repository.CurrentPeriod(periodType)
		ranked := append([]demoLearnerStat(nil), demoLearnerStats...)
		pointsOf := func(st demoLearnerStat) int {
			if periodType == "weekly" {
				return st.WeeklyPoints
			}
			return st.MonthlyPoints
		}
		sort.SliceStable(ranked, func(i, j int) bool { return pointsOf(ranked[i]) > pointsOf(ranked[j]) })

		for i, st := range ranked {
			user, err := demoUser(users, st.Email)
			if err != nil {
				return err
			}
			scale := 1
			if periodType == "monthly" {
				scale = 4
			}
			entry := model.LeaderboardEntry{
				UserID:           user.ID,
				Period:           period,
				PeriodType:       periodType,
				Points:           pointsOf(st),
				Rank:             i + 1,
				LessonsCompleted: min(len(facts[st.Email].Lessons), 2*scale),
				QuizzesCompleted: min(st.Quizzes, scale),
				StudyMinutes:     st.StudyMinutes * scale / 8,
			}
			if err := s.db.Where("user_id = ? AND period = ? AND period_type = ?", user.ID, period, periodType).
				Attrs(entry).FirstOrCreate(&entry).Error; err != nil {
				return fmt.Errorf("failed to seed leaderboard %s %s for %s: %w", periodType, period, st.Email, err)
			}
		}
	}
	return nil
}
