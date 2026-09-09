package seeds

import (
	"fmt"
	"log"
	"time"

	"github.com/google/uuid"
	"study.com/v1/internal/model"
)

// demoClassSpec mô tả một lớp học demo gắn với khoá học, giáo viên và học viên.
type demoClassSpec struct {
	Name          string
	Description   string
	CourseSlug    string
	TeacherEmail  string
	StudentEmails []string
	MaxStudents   int
	StartDaysAgo  int
	EndDaysAhead  int
}

var demoClasses = []demoClassSpec{
	{
		Name:          "React K01 - Ca tối T3/T5",
		Description:   "Lớp React + Next.js khoá 01, học tối thứ 3 và thứ 5 hàng tuần.",
		CourseSlug:    "react-nextjs-tu-co-ban-den-nang-cao",
		TeacherEmail:  "teacher1@demo.com",
		StudentEmails: []string{"student1@demo.com", "student2@demo.com"},
		MaxStudents:   30,
		StartDaysAgo:  30,
		EndDaysAhead:  60,
	},
	{
		Name:          "Data Science K02 - Ca sáng T7",
		Description:   "Lớp Python cho Khoa học Dữ liệu khoá 02, học sáng thứ 7.",
		CourseSlug:    "python-cho-khoa-hoc-du-lieu",
		TeacherEmail:  "teacher2@demo.com",
		StudentEmails: []string{"student2@demo.com"},
		MaxStudents:   25,
		StartDaysAgo:  14,
		EndDaysAhead:  75,
	},
}

// attendancePattern là lịch sử điểm danh tương đối của một học viên,
// tính lùi từ hôm nay. Dùng chung cho mọi lớp để giữ seeder ngắn gọn.
var attendancePattern = []struct {
	DaysAgo     int
	Status      string
	LateMinutes int
	Note        string
}{
	{DaysAgo: 21, Status: "present"},
	{DaysAgo: 14, Status: "late", LateMinutes: 12, Note: "Kẹt xe, vào muộn 12 phút"},
	{DaysAgo: 7, Status: "present"},
	{DaysAgo: 4, Status: "excused", Note: "Xin phép nghỉ do lịch thi ở trường"},
	{DaysAgo: 2, Status: "present"},
}

// SeedDemoClasses tạo lớp học, phân công giáo viên, xếp học viên vào lớp
// và sinh lịch sử điểm danh cho trang /my-attendance.
// Idempotent: lớp khoá theo tên; các bảng nối và điểm danh đều có unique index.
func (s *Seeder) SeedDemoClasses(
	users map[string]model.User,
	courses map[string]model.Course,
) error {
	log.Println("Seeding demo classes & attendance...")

	attendanceCount := 0
	for _, spec := range demoClasses {
		course, ok := courses[spec.CourseSlug]
		if !ok {
			return fmt.Errorf("class course %s not found", spec.CourseSlug)
		}
		teacher, ok := users[spec.TeacherEmail]
		if !ok {
			return fmt.Errorf("class teacher %s not found", spec.TeacherEmail)
		}

		class, err := s.upsertClass(spec, course.ID)
		if err != nil {
			return err
		}

		teacherClass := model.TeacherClass{TeacherID: teacher.ID, ClassID: class.ID, Role: "primary"}
		if err := s.db.Where("teacher_id = ? AND class_id = ?", teacher.ID, class.ID).
			Attrs(teacherClass).
			FirstOrCreate(&teacherClass).Error; err != nil {
			return fmt.Errorf("failed to assign teacher to class %s: %w", spec.Name, err)
		}

		for _, email := range spec.StudentEmails {
			student, ok := users[email]
			if !ok {
				return fmt.Errorf("class student %s not found", email)
			}

			studentClass := model.StudentClass{StudentID: student.ID, ClassID: class.ID, Status: "active"}
			if err := s.db.Where("student_id = ? AND class_id = ?", student.ID, class.ID).
				Attrs(studentClass).
				FirstOrCreate(&studentClass).Error; err != nil {
				return fmt.Errorf("failed to enroll student into class %s: %w", spec.Name, err)
			}

			n, err := s.seedAttendance(class.ID, student.ID, teacher.ID)
			if err != nil {
				return err
			}
			attendanceCount += n
		}
	}

	log.Printf("Seeded %d classes, %d attendance records\n", len(demoClasses), attendanceCount)
	return nil
}

func (s *Seeder) upsertClass(spec demoClassSpec, courseID uuid.UUID) (model.Class, error) {
	start := daysAgo(spec.StartDaysAgo)
	end := daysAhead(spec.EndDaysAhead)

	class := model.Class{
		Name:        spec.Name,
		Description: ptr(spec.Description),
		CourseID:    &courseID,
		Status:      "active",
		MaxStudents: ptr(spec.MaxStudents),
		StartDate:   &start,
		EndDate:     &end,
	}
	if err := s.db.Where("name = ?", spec.Name).
		Attrs(class).
		FirstOrCreate(&class).Error; err != nil {
		return model.Class{}, fmt.Errorf("failed to seed class %s: %w", spec.Name, err)
	}
	return class, nil
}

// seedAttendance sinh điểm danh theo attendancePattern. Ngày được cắt về đầu
// ngày vì cột date là kiểu DATE và nằm trong unique index (class, student, date).
func (s *Seeder) seedAttendance(classID, studentID, verifierID uuid.UUID) (int, error) {
	created := 0
	for _, p := range attendancePattern {
		day := truncateToDay(daysAgo(p.DaysAgo))

		record := model.Attendance{
			ClassID:     classID,
			StudentID:   studentID,
			Date:        day,
			Status:      p.Status,
			LateMinutes: p.LateMinutes,
			Location:    ptr("online"),
			VerifiedBy:  &verifierID,
		}
		if p.Note != "" {
			record.Note = ptr(p.Note)
		}
		if p.Status == "present" || p.Status == "late" {
			checkIn := day.Add(19*time.Hour + time.Duration(p.LateMinutes)*time.Minute)
			checkOut := day.Add(21 * time.Hour)
			record.CheckInTime = &checkIn
			record.CheckOutTime = &checkOut
		}

		if err := s.db.Where("class_id = ? AND student_id = ? AND date = ?", classID, studentID, day).
			Attrs(record).
			FirstOrCreate(&record).Error; err != nil {
			return 0, fmt.Errorf("failed to seed attendance: %w", err)
		}
		created++
	}
	return created, nil
}

// truncateToDay cắt bỏ phần giờ/phút để khớp với cột DATE.
func truncateToDay(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, t.Location())
}
