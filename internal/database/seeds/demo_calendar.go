package seeds

import (
	"fmt"
	"log"
	"time"

	"study.com/v1/internal/model"
)

// calendarSlot trả mốc giờ cố định (giờ:00) của ngày cách hôm nay dayOffset ngày, theo giờ máy chủ.
func calendarSlot(dayOffset, hour int) time.Time {
	d := time.Now().AddDate(0, 0, dayOffset)
	return time.Date(d.Year(), d.Month(), d.Day(), hour, 0, 0, 0, d.Location())
}

type demoPersonalEventSpec struct {
	Email, Title, Description, Location, Color string
	Day, Hour, DurationMin                     int
	ReminderMin                                int
}

// demoPersonalEventSpecs: sự kiện cá nhân trong tuần này và tuần sau (API /me/events lọc theo khoảng
// ngày chồng lấn start_time/end_time).
var demoPersonalEventSpecs = []demoPersonalEventSpec{
	{Email: "student1@demo.com", Title: "Ôn tập React Hooks", Description: "Xem lại useEffect, useMemo trước buổi live.",
		Location: "Thư viện tầng 3", Color: "#3B82F6", Day: 1, Hour: 19, DurationMin: 90, ReminderMin: 30},
	{Email: "student1@demo.com", Title: "Họp nhóm đồ án K66", Description: "Chốt tiến độ API đăng nhập và giao diện.",
		Location: "Google Meet", Color: "#10B981", Day: 3, Hour: 9, DurationMin: 60, ReminderMin: 15},
	{Email: "student1@demo.com", Title: "Nộp báo cáo giữa kỳ", Description: "Hạn nộp báo cáo đồ án lên hệ thống của trường.",
		Location: "", Color: "#EF4444", Day: 8, Hour: 23, DurationMin: 30, ReminderMin: 1440},
	{Email: "teacher1@demo.com", Title: "Chuẩn bị slide Server Components", Description: "Soạn ví dụ minh hoạ cho buổi live chữa bài.",
		Location: "Văn phòng", Color: "#8B5CF6", Day: 1, Hour: 14, DurationMin: 120, ReminderMin: 60},
	{Email: "teacher1@demo.com", Title: "Chấm bài Todo App", Description: "Chấm các bài nộp và gửi nhận xét cho học viên.",
		Location: "", Color: "#F59E0B", Day: 6, Hour: 20, DurationMin: 90, ReminderMin: 30},
}

// SeedDemoPersonalEvents tạo lịch cá nhân cho student1/teacher1 (trang (app)/schedule, (teacher)/teacher/schedule).
// Khoá tự nhiên (user_id, title): chạy lại không nhân bản, không dời lịch đã có.
func (s *Seeder) SeedDemoPersonalEvents(users map[string]model.User) error {
	for _, spec := range demoPersonalEventSpecs {
		user, ok := users[spec.Email]
		if !ok {
			return fmt.Errorf("personal event %q: user %s not found", spec.Title, spec.Email)
		}
		start := calendarSlot(spec.Day, spec.Hour)
		event := model.PersonalEvent{UserID: user.ID, Title: spec.Title, Description: ptr(spec.Description),
			StartTime: start, EndTime: start.Add(time.Duration(spec.DurationMin) * time.Minute),
			Color: ptr(spec.Color), ReminderMin: ptr(spec.ReminderMin)}
		if spec.Location != "" {
			event.Location = ptr(spec.Location)
		}
		if err := s.db.Where("user_id = ? AND title = ?", user.ID, spec.Title).
			Attrs(event).FirstOrCreate(&event).Error; err != nil {
			return fmt.Errorf("failed to seed personal event %q: %w", spec.Title, err)
		}
	}
	log.Printf("Seeded %d demo personal events\n", len(demoPersonalEventSpecs))
	return nil
}
