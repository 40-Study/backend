package seeds

import (
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"study.com/v1/internal/model"
)

// QA-A-04: trước đây chuỗi ngày, số bài và huy hiệu của học viên demo là số gõ tay trong demoLearnerStats, độc lập với
// lesson_progress mà widget "Hoạt động học tập" đọc. Kết quả: "3 ngày liên tiếp" cạnh biểu đồ 7 ngày toàn 0, huy hiệu
// "Hoàn thành bài học đầu tiên" khi 0 bài hoàn thành. Giờ seed SUY RA chuỗi ngày và huy hiệu từ chính dữ liệu học đã seed.

// demoActivityHour: giờ (VN) của các lần hoàn thành bài demo. 09:00 VN = 02:00 UTC nên ngày theo UTC trùng ngày theo VN:
// truy vấn hoạt động của backend nhóm theo DATE(completed_at) ở múi giờ phiên DB (UTC), giờ tối VN sẽ lệch sang hôm sau.
const demoActivityHour = 9

// demoCompletionTime: mốc hoàn thành bài cách lần gần nhất daysBefore ngày. Lần gần nhất là 09:00 hôm nay (VN), hoặc
// 09:00 hôm qua nếu lúc chạy seed chưa tới 09:00 — không ghi mốc ở tương lai.
func demoCompletionTime(now time.Time, daysBefore int) time.Time {
	anchor := demoWallClock(demoTodayAt(now), 0, demoActivityHour, 0)
	if now.Before(anchor) {
		anchor = anchor.AddDate(0, 0, -1)
	}
	return anchor.AddDate(0, 0, -daysBefore)
}

// demoLearnerFacts: các mốc thời gian (tăng dần) chứng minh tiến độ thật của một học viên trong DB.
type demoLearnerFacts struct {
	Lessons       []time.Time // lesson_progress hoàn thành
	Courses       []time.Time // enrollment hoàn thành
	PassedQuizzes []time.Time // quiz_attempt đạt
	Reviews       []time.Time // đánh giá đã viết
}

// demoActivityDays: các ngày (00:00 giờ VN) khác nhau có hoàn thành bài, tăng dần.
func demoActivityDays(times []time.Time) []time.Time {
	var days []time.Time
	for _, t := range times {
		d := demoTodayAt(t)
		if len(days) == 0 || !days[len(days)-1].Equal(d) {
			days = append(days, d)
		}
	}
	return days
}

// demoStreaks tính chuỗi ngày học từ các mốc hoàn thành bài (đã sắp tăng dần). Chuỗi hiện tại là chuỗi ngày liền nhau
// kết thúc ở hôm nay hoặc hôm qua (hôm nay chưa học thì chuỗi chưa gãy); xa hơn thì bằng 0.
func demoStreaks(now time.Time, lessonTimes []time.Time) (current, longest int) {
	days := demoActivityDays(lessonTimes)
	run := 0
	for i, d := range days {
		if i > 0 && days[i-1].AddDate(0, 0, 1).Equal(d) {
			run++
		} else {
			run = 1
		}
		longest = max(longest, run)
	}
	if len(days) == 0 {
		return 0, 0
	}
	if last := days[len(days)-1]; !last.Before(demoTodayAt(now).AddDate(0, 0, -1)) {
		current = run
	}
	return current, longest
}

// demoStreakReachedAt: lần đầu chuỗi ngày học chạm threshold ngày (mốc hoàn thành bài của ngày đó).
func demoStreakReachedAt(lessonTimes []time.Time, threshold int) (time.Time, bool) {
	run := 0
	var prev time.Time
	for _, t := range lessonTimes {
		d := demoTodayAt(t)
		switch {
		case run > 0 && d.Equal(prev):
			continue
		case run > 0 && prev.AddDate(0, 0, 1).Equal(d):
			run++
		default:
			run = 1
		}
		prev = d
		if run >= threshold {
			return t, true
		}
	}
	return time.Time{}, false
}

// nthTime: mốc thứ n (từ 1) — lúc đạt ngưỡng n lần.
func nthTime(times []time.Time, n int) (time.Time, bool) {
	n = max(n, 1)
	if len(times) < n {
		return time.Time{}, false
	}
	return times[n-1], true
}

// demoAchievementEarnedAt quyết định học viên có đạt thành tích không, dựa trên dữ liệu thật, và đạt lúc nào. Yêu cầu
// mà seed không có dữ liệu chứng minh (học sau 22 giờ, thành tích bí mật) thì không cấp: thà thiếu huy hiệu còn hơn
// huy hiệu nói dối.
func demoAchievementEarnedAt(a model.Achievement, f demoLearnerFacts) (time.Time, bool) {
	req := a.Requirement
	switch {
	case strings.HasPrefix(req, "complete_") && strings.Contains(req, "lesson"):
		return nthTime(f.Lessons, a.Threshold)
	case strings.HasPrefix(req, "complete_") && strings.Contains(req, "course"):
		return nthTime(f.Courses, a.Threshold)
	case strings.HasPrefix(req, "pass_") && strings.Contains(req, "quiz"):
		return nthTime(f.PassedQuizzes, a.Threshold)
	case strings.HasPrefix(req, "write_") && strings.Contains(req, "review"):
		return nthTime(f.Reviews, a.Threshold)
	case strings.HasPrefix(req, "streak_"):
		return demoStreakReachedAt(f.Lessons, max(a.Threshold, 1))
	default:
		return time.Time{}, false
	}
}

// loadDemoLearnerFacts đọc các mốc tiến độ của học viên từ DB (sau khi ghi danh, bài làm, đánh giá đã seed).
func (s *Seeder) loadDemoLearnerFacts(userID uuid.UUID) (demoLearnerFacts, error) {
	var f demoLearnerFacts
	queries := []struct {
		dst   *[]time.Time
		model interface{}
		col   string
		where string
	}{
		{&f.Lessons, &model.LessonProgress{}, "completed_at", "status = 'completed' AND completed_at IS NOT NULL"},
		{&f.Courses, &model.Enrollment{}, "completed_at", "completed_at IS NOT NULL"},
		{&f.PassedQuizzes, &model.QuizAttempt{}, "completed_at", "is_passed = true AND completed_at IS NOT NULL"},
		{&f.Reviews, &model.Review{}, "created_at", "1 = 1"},
	}
	for _, q := range queries {
		if err := s.db.Model(q.model).Where("user_id = ?", userID).Where(q.where).
			Order(q.col).Pluck(q.col, q.dst).Error; err != nil {
			return f, fmt.Errorf("load learner facts (%s): %w", q.col, err)
		}
	}
	return f, nil
}
