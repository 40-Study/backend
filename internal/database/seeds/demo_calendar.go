package seeds

import (
	"fmt"
	"log"
	"time"

	"study.com/v1/internal/model"
)

type demoClassSpec struct {
	Name, Description string
	CourseSlug        string
	TeacherEmail      string
	StudentEmails     []string
}

// demoClassSpecs: livestream_sessions.class_id là NOT NULL và học viên chỉ thấy phiên qua lớp mình
// học (student_classes active) — nên mỗi livestream demo cần 1 lớp có đủ giảng viên + học viên.
var demoClassSpecs = []demoClassSpec{
	{Name: "Lớp React & Next.js - K01", Description: "Lớp học trực tuyến buổi tối cho học viên khoá React + Next.js.",
		CourseSlug: "react-nextjs-tu-co-ban-den-nang-cao", TeacherEmail: "teacher1@demo.com",
		StudentEmails: []string{"student1@demo.com"}},
	{Name: "Lớp Python Data - K01", Description: "Lớp thực hành phân tích dữ liệu với Pandas mỗi cuối tuần.",
		CourseSlug: "python-cho-khoa-hoc-du-lieu", TeacherEmail: "teacher2@demo.com",
		StudentEmails: []string{"student2@demo.com"}},
}

type demoLivestreamSpec struct {
	RoomName, Title, Description string
	ClassName                    string
	HostEmail                    string
	Status                       model.LivestreamSessionStatus
	StartDay, StartHour          int // StartDay âm = đã qua
	DurationMin                  int
}

// demoLivestreamSpecs: 1 buổi sắp diễn ra (teacher1) và 1 buổi đã kết thúc (teacher2).
var demoLivestreamSpecs = []demoLivestreamSpec{
	{RoomName: "demo-react-k01-chua-bai-todo", Title: "Live chữa bài Todo App & hỏi đáp Server Components",
		Description: "Chữa bài tập Todo App, giải đáp thắc mắc về Server Components và Client Components.",
		ClassName:   "Lớp React & Next.js - K01", HostEmail: "teacher1@demo.com",
		Status: model.LivestreamStatusScheduled, StartDay: 2, StartHour: 20, DurationMin: 90},
	{RoomName: "demo-python-k01-pandas-thuc-chien", Title: "Pandas thực chiến: làm sạch dữ liệu bán hàng",
		Description: "Buổi live làm sạch bộ dữ liệu bán hàng thật: xử lý thiếu, trùng lặp và định dạng ngày.",
		ClassName:   "Lớp Python Data - K01", HostEmail: "teacher2@demo.com",
		Status: model.LivestreamStatusEnded, StartDay: -3, StartHour: 19, DurationMin: 75},
}

// calendarSlot trả mốc giờ cố định (giờ:00) của ngày cách hôm nay dayOffset ngày, theo giờ máy chủ.
func calendarSlot(dayOffset, hour int) time.Time {
	d := time.Now().AddDate(0, 0, dayOffset)
	return time.Date(d.Year(), d.Month(), d.Day(), hour, 0, 0, 0, d.Location())
}

// SeedDemoClassesAndLivestreams tạo lớp (gắn khoá, giảng viên, học viên) và phiên livestream cho trang
// livestream/lịch dạy. Khoá tự nhiên: lớp theo (name, course_id), phiên theo room_name (unique).
func (s *Seeder) SeedDemoClassesAndLivestreams(users map[string]model.User, courses map[string]model.Course) error {
	classes := make(map[string]model.Class, len(demoClassSpecs))
	for _, spec := range demoClassSpecs {
		course, ok := courses[spec.CourseSlug]
		if !ok {
			return fmt.Errorf("class %s: course %s not found", spec.Name, spec.CourseSlug)
		}
		teacher := users[spec.TeacherEmail]
		class := model.Class{Name: spec.Name, Description: ptr(spec.Description), CourseID: &course.ID,
			Status: "active", MaxStudents: ptr(40), StartDate: ptr(daysAgo(20)), EndDate: ptr(daysAhead(60)),
			CreatedBy: &teacher.ID}
		if err := s.db.Where("name = ? AND course_id = ?", spec.Name, course.ID).
			Attrs(class).FirstOrCreate(&class).Error; err != nil {
			return fmt.Errorf("failed to seed class %s: %w", spec.Name, err)
		}
		tc := model.TeacherClass{TeacherID: teacher.ID, ClassID: class.ID, Role: "primary"}
		if err := s.db.Where("teacher_id = ? AND class_id = ?", teacher.ID, class.ID).
			Attrs(tc).FirstOrCreate(&tc).Error; err != nil {
			return fmt.Errorf("failed to assign teacher to class %s: %w", spec.Name, err)
		}
		for _, email := range spec.StudentEmails {
			sc := model.StudentClass{StudentID: users[email].ID, ClassID: class.ID, Status: "active"}
			if err := s.db.Where("student_id = ? AND class_id = ?", sc.StudentID, class.ID).
				Attrs(sc).FirstOrCreate(&sc).Error; err != nil {
				return fmt.Errorf("failed to add %s to class %s: %w", email, spec.Name, err)
			}
		}
		classes[spec.Name] = class
	}

	for _, spec := range demoLivestreamSpecs {
		class := classes[spec.ClassName]
		start := calendarSlot(spec.StartDay, spec.StartHour)
		session := model.LivestreamSession{Title: spec.Title, Description: ptr(spec.Description),
			HostID: users[spec.HostEmail].ID, ClassID: class.ID, CourseID: class.CourseID, RoomName: spec.RoomName,
			Status: spec.Status, ScheduledAt: &start, MaxViewers: 1000, IsRecorded: true,
			Settings: model.LivestreamSettings{IsChatEnabled: true, IsQAEnabled: true, IsWhiteboardEnabled: true,
				IsScreenShareEnabled: true, IsPollsEnabled: true}}
		if spec.Status == model.LivestreamStatusEnded {
			end := start.Add(time.Duration(spec.DurationMin) * time.Minute)
			session.StartedAt, session.EndedAt = &start, &end
		}
		if err := s.db.Where("room_name = ?", spec.RoomName).Attrs(session).FirstOrCreate(&session).Error; err != nil {
			return fmt.Errorf("failed to seed livestream %s: %w", spec.RoomName, err)
		}
	}
	log.Printf("Seeded %d demo classes, %d livestream sessions\n", len(demoClassSpecs), len(demoLivestreamSpecs))
	return nil
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
