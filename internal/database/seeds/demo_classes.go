package seeds

import (
	"fmt"
	"log"
	"time"

	"study.com/v1/internal/model"
)

// SeedDemoClasses seed lớp học trực tiếp cho tài khoản demo: lớp + giáo viên + học sinh, lịch học
// lặp theo tuần, buổi học (quá khứ đã điểm danh, tương lai để hiện ở lịch), phiên livestream chữa
// bài, bài tập gắn phiên/lớp, bài nộp và điểm có nhận xét.
//
// Trang cần dữ liệu này: học sinh (app)/schedule (/me/timetable), (app)/my-attendance
// (/me/attendances), (app)/my-assignments (/livestream -> /assignments?session_id=); giáo viên
// teacher/schedule và teacher/assignments (/livestream?host_id=), teacher/classes/[id]/attendance
// (/classes/:id/sessions); phụ huynh parent/children/[id] (grades, timetable, attendance,
// assignments của student1).
//
// Phụ thuộc: SeedDemoUsers, SeedDemoCourses (cần khoá theo slug và giảng viên chủ khoá). Không cần
// organization: quyền xem lớp chỉ xét created_by, teacher_classes, giảng viên chủ khoá và
// student_classes (service/class_access.go). Idempotent: mọi bản ghi tra theo khoá tự nhiên.
func (s *Seeder) SeedDemoClasses(users map[string]model.User, courses map[string]model.Course) error {
	log.Println("Seeding demo classes...")

	for _, spec := range demoClassSpecs() {
		if err := s.seedDemoClass(spec, users, courses); err != nil {
			return fmt.Errorf("class %q: %w", spec.Name, err)
		}
	}

	log.Printf("Seeded %d demo classes with schedules, sessions, assignments and grades\n", len(demoClassSpecs()))
	return nil
}

// seedDemoClass seed trọn một lớp theo đúng thứ tự khoá ngoại.
func (s *Seeder) seedDemoClass(spec classSpec, users map[string]model.User, courses map[string]model.Course) error {
	teacher, ok := users[spec.TeacherEmail]
	if !ok {
		return fmt.Errorf("teacher %s not found", spec.TeacherEmail)
	}
	course, ok := courses[spec.CourseSlug]
	if !ok {
		return fmt.Errorf("course %s not found", spec.CourseSlug)
	}
	students := make([]model.User, 0, len(spec.StudentEmails))
	for _, email := range spec.StudentEmails {
		student, ok := users[email]
		if !ok {
			return fmt.Errorf("student %s not found", email)
		}
		students = append(students, student)
	}

	class, err := s.upsertDemoClass(spec, teacher, course)
	if err != nil {
		return err
	}
	if err := s.linkDemoClassMembers(class, teacher, students); err != nil {
		return err
	}
	schedules, err := s.upsertDemoSchedules(spec, class)
	if err != nil {
		return err
	}
	if err := s.seedDemoClassSessions(spec, class, teacher, students, schedules); err != nil {
		return err
	}
	lives, err := s.upsertDemoLivestreams(spec, class, teacher, course)
	if err != nil {
		return err
	}
	return s.seedDemoAssignments(spec, class, teacher, users, lives)
}

// upsertDemoClass tạo lớp (khoá tự nhiên: tên lớp). Ngày khai giảng chỉ đặt ở lần tạo đầu tiên nên
// số buổi học sinh ra ở các lần chạy sau vẫn ổn định.
func (s *Seeder) upsertDemoClass(spec classSpec, teacher model.User, course model.Course) (model.Class, error) {
	start := demoToday().AddDate(0, 0, -spec.StartDaysAgo)
	end := start.AddDate(0, 0, spec.DurationWeeks*7)
	class := model.Class{
		Name:        spec.Name,
		Description: ptr(spec.Description),
		CourseID:    &course.ID,
		Status:      "active",
		MaxStudents: ptr(spec.MaxStudents),
		StartDate:   &start,
		EndDate:     &end,
		CreatedBy:   &teacher.ID,
	}
	if err := s.db.Where("name = ?", spec.Name).Attrs(class).FirstOrCreate(&class).Error; err != nil {
		return model.Class{}, fmt.Errorf("failed to seed class: %w", err)
	}
	return class, nil
}

// linkDemoClassMembers gán giáo viên chính và ghi danh học sinh (active) vào lớp.
func (s *Seeder) linkDemoClassMembers(class model.Class, teacher model.User, students []model.User) error {
	tc := model.TeacherClass{TeacherID: teacher.ID, ClassID: class.ID, Role: "primary"}
	if err := s.db.Where("teacher_id = ? AND class_id = ?", teacher.ID, class.ID).
		Attrs(tc).FirstOrCreate(&tc).Error; err != nil {
		return fmt.Errorf("failed to assign teacher: %w", err)
	}
	for _, student := range students {
		sc := model.StudentClass{StudentID: student.ID, ClassID: class.ID, Status: "active", EnrolledAt: dateOrToday(class.StartDate)}
		if err := s.db.Where("student_id = ? AND class_id = ?", student.ID, class.ID).
			Attrs(sc).FirstOrCreate(&sc).Error; err != nil {
			return fmt.Errorf("failed to enroll student: %w", err)
		}
	}
	return nil
}

// upsertDemoSchedules tạo lịch lặp theo tuần, mỗi thứ trong tuần một bản ghi (khoá: lớp + thứ).
// Trả map thứ -> lịch để buổi học trỏ schedule_id đúng.
func (s *Seeder) upsertDemoSchedules(spec classSpec, class model.Class) (map[int]model.ClassSchedule, error) {
	result := make(map[int]model.ClassSchedule, len(spec.Days))
	from := dateOrToday(class.StartDate)
	startAt, err := clockStamp(from, spec.StartTime)
	if err != nil {
		return nil, err
	}
	endAt, err := clockStamp(from, spec.EndTime)
	if err != nil {
		return nil, err
	}
	for _, day := range spec.Days {
		sch := model.ClassSchedule{
			ClassID:        class.ID,
			DayOfWeek:      day,
			StartTime:      startAt, // cột TIMESTAMPTZ, xem clockStamp
			EndTime:        endAt,
			Room:           ptr(spec.Room),
			IsActive:       true,
			EffectiveFrom:  from,
			EffectiveUntil: class.EndDate,
		}
		if err := s.db.Where("class_id = ? AND day_of_week = ?", class.ID, day).
			Attrs(sch).FirstOrCreate(&sch).Error; err != nil {
			return nil, fmt.Errorf("failed to seed schedule: %w", err)
		}
		result[day] = sch
	}
	return result, nil
}

// demoToday trả về 00:00 hôm nay theo giờ máy chủ — mốc để chia buổi quá khứ/tương lai.
func demoToday() time.Time {
	return toLocalDate(time.Now())
}

// toLocalDate bỏ phần giờ, giữ nguyên ngày/tháng/năm (cột date đọc từ Postgres về ở UTC 00:00).
func toLocalDate(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.Local)
}

// dateOrToday chuẩn hoá con trỏ ngày của lớp; lớp demo luôn có start_date nên nhánh nil chỉ là an toàn.
func dateOrToday(t *time.Time) time.Time {
	if t == nil {
		return demoToday()
	}
	return toLocalDate(*t)
}
