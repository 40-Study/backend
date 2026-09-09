package seeds

import (
	"fmt"
	"log"

	"github.com/google/uuid"
	"study.com/v1/internal/model"
)

// demoNotificationSpec mô tả một thông báo trong hộp thư của học viên.
type demoNotificationSpec struct {
	Title   string
	Content string
	// Type phải nằm trong check constraint của cột notification_type.
	Type    string
	IsRead  bool
	DaysAgo int
}

// demoNotificationsByUser gán danh sách thông báo cho từng tài khoản demo.
var demoNotificationsByUser = map[string][]demoNotificationSpec{
	"student1@demo.com": {
		{Title: "Chúc mừng! Bạn đã nhận chứng chỉ",
			Content: "Bạn vừa hoàn thành khoá \"Git & GitHub cho người mới bắt đầu\". Chứng chỉ đã sẵn sàng để tải về.",
			Type:    "certificate_earned", DaysAgo: 3},
		{Title: "Bài học mới trong khoá React + Next.js",
			Content: "Giảng viên vừa thêm bài \"Tối ưu hiệu năng với Suspense\" vào chương 4.",
			Type:    "new_lesson", DaysAgo: 2},
		{Title: "Nhắc làm bài kiểm tra",
			Content: "Bài kiểm tra chương 3 của khoá React + Next.js sẽ đóng sau 2 ngày nữa.",
			Type:    "quiz_reminder", DaysAgo: 1},
		{Title: "Bạn nhận được 200 xu",
			Content: "Phần thưởng cho thành tựu \"Hoàn thành khoá học đầu tiên\".",
			Type:    "point_earned", IsRead: true, DaysAgo: 3},
		{Title: "Ưu đãi hè - giảm đến 50%",
			Content: "Nhập mã SUMMER50K để giảm 50.000đ cho đơn từ 300.000đ.",
			Type:    "promotion", IsRead: true, DaysAgo: 8},
	},
	"student2@demo.com": {
		{Title: "Chào mừng bạn đến với 40Study",
			Content: "Hoàn thiện hồ sơ để nhận gợi ý khoá học phù hợp với mục tiêu của bạn.",
			Type:    "system", IsRead: true, DaysAgo: 12},
		{Title: "Đừng bỏ lỡ chuỗi học của bạn",
			Content: "Bạn đã học 3 ngày liên tiếp. Học thêm hôm nay để giữ chuỗi!",
			Type:    "streak", DaysAgo: 1},
		{Title: "Bài học mới trong khoá Python",
			Content: "Chương \"Pandas nâng cao\" vừa được bổ sung 3 bài học mới.",
			Type:    "new_lesson", DaysAgo: 4},
	},
	"teacher1@demo.com": {
		{Title: "Học viên mới ghi danh",
			Content: "Có 2 học viên vừa ghi danh khoá React + Next.js của bạn trong tuần này.",
			Type:    "course_update", DaysAgo: 2},
	},
	"parent1@demo.com": {
		{Title: "Báo cáo học tập tuần của Lê Văn C",
			Content: "Tuần này con bạn hoàn thành 5 bài học và đạt 90% bài kiểm tra chương 1.",
			Type:    "system", DaysAgo: 1},
	},
}

// SeedDemoNotifications tạo thông báo và cấu hình thông báo cho tài khoản demo.
// Idempotent: thông báo chỉ sinh khi user chưa có bản ghi nào, tránh nhân bản
// danh sách qua mỗi lần seed (bảng notifications không có unique key tự nhiên).
func (s *Seeder) SeedDemoNotifications(users map[string]model.User) error {
	log.Println("Seeding demo notifications...")

	total := 0
	for email, specs := range demoNotificationsByUser {
		user, ok := users[email]
		if !ok {
			return fmt.Errorf("notification owner %s not found", email)
		}

		if err := s.upsertNotificationSettings(user.ID); err != nil {
			return err
		}

		var existing int64
		if err := s.db.Model(&model.Notification{}).
			Where("user_id = ?", user.ID).
			Count(&existing).Error; err != nil {
			return fmt.Errorf("failed to count notifications: %w", err)
		}
		if existing > 0 {
			continue
		}

		for _, spec := range specs {
			notification := model.Notification{
				UserID:           user.ID,
				Title:            spec.Title,
				Content:          spec.Content,
				NotificationType: spec.Type,
				IsRead:           spec.IsRead,
				CreatedAt:        daysAgo(spec.DaysAgo),
			}
			if spec.IsRead {
				readAt := daysAgo(spec.DaysAgo)
				notification.ReadAt = &readAt
			}
			if err := s.db.Create(&notification).Error; err != nil {
				return fmt.Errorf("failed to seed notification for %s: %w", email, err)
			}
			total++
		}
	}

	log.Printf("Seeded %d notifications\n", total)
	return nil
}

func (s *Seeder) upsertNotificationSettings(userID uuid.UUID) error {
	settings := model.NotificationSettings{
		UserID:               userID,
		EmailCourseUpdates:   true,
		EmailPromotions:      true,
		EmailRecommendations: true,
		PushCourseUpdates:    true,
		PushQuizReminders:    true,
		PushAchievements:     true,
		PushStreakReminders:  true,
	}
	if err := s.db.Where("user_id = ?", userID).
		Attrs(settings).
		FirstOrCreate(&settings).Error; err != nil {
		return fmt.Errorf("failed to seed notification settings: %w", err)
	}
	return nil
}
