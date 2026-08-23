package seeds

import (
	"fmt"
	"log"
	"time"

	"github.com/google/uuid"
	"study.com/v1/internal/model"
)

type enrollmentSpec struct {
	StudentEmail string
	CourseSlug   string
	Progress     float64 // phần trăm hoàn thành khoá học
}

var demoEnrollments = []enrollmentSpec{
	{StudentEmail: "student1@demo.com", CourseSlug: "react-nextjs-tu-co-ban-den-nang-cao", Progress: 65},
	{StudentEmail: "student1@demo.com", CourseSlug: "flutter-mobile-development", Progress: 20},
	{StudentEmail: "student1@demo.com", CourseSlug: "git-github-cho-nguoi-moi-bat-dau", Progress: 100},
	{StudentEmail: "student2@demo.com", CourseSlug: "python-cho-khoa-hoc-du-lieu", Progress: 10},
}

// SeedDemoEnrollments ghi danh học viên demo và sinh tiến độ từng bài học
// khớp với phần trăm hoàn thành của khoá.
func (s *Seeder) SeedDemoEnrollments(
	users map[string]model.User,
	courses map[string]model.Course,
) error {
	log.Println("Seeding demo enrollments...")

	for _, spec := range demoEnrollments {
		student, ok := users[spec.StudentEmail]
		if !ok {
			return fmt.Errorf("student %s not found", spec.StudentEmail)
		}
		course, ok := courses[spec.CourseSlug]
		if !ok {
			return fmt.Errorf("course %s not found", spec.CourseSlug)
		}

		enrollment, err := s.upsertEnrollment(student.ID, course.ID, spec.Progress)
		if err != nil {
			return err
		}
		if err := s.seedLessonProgress(enrollment, course.ID, spec.Progress); err != nil {
			return err
		}
	}

	log.Printf("Seeded %d enrollments\n", len(demoEnrollments))
	return nil
}

func (s *Seeder) upsertEnrollment(userID, courseID uuid.UUID, progress float64) (model.Enrollment, error) {
	lastAccessed := daysAgo(2)

	enrollment := model.Enrollment{
		UserID:          userID,
		CourseID:        courseID,
		EnrolledAt:      daysAgo(20),
		ProgressPercent: rating(progress),
		LastAccessedAt:  &lastAccessed,
	}
	if progress >= 100 {
		completed := daysAgo(3)
		enrollment.CompletedAt = &completed
	}

	if err := s.db.Where("user_id = ? AND course_id = ?", userID, courseID).
		Attrs(enrollment).
		FirstOrCreate(&enrollment).Error; err != nil {
		return model.Enrollment{}, fmt.Errorf("failed to seed enrollment: %w", err)
	}
	return enrollment, nil
}

// seedLessonProgress đánh dấu N bài đầu tiên là completed sao cho tỉ lệ hoàn
// thành xấp xỉ progress; bài kế tiếp để in_progress cho tự nhiên.
func (s *Seeder) seedLessonProgress(enrollment model.Enrollment, courseID uuid.UUID, progress float64) error {
	lessons, err := s.courseLessonsInOrder(courseID)
	if err != nil {
		return err
	}
	if len(lessons) == 0 {
		return nil
	}

	completedCount := int(float64(len(lessons)) * progress / 100)

	for i, lesson := range lessons {
		status := "not_started"
		percent := 0.0
		var completedAt *time.Time

		switch {
		case i < completedCount:
			status = "completed"
			percent = 100
			ts := daysAgo(5)
			completedAt = &ts
		case i == completedCount && progress < 100:
			status = "in_progress"
			percent = 40
		}

		if status == "not_started" {
			continue // không tạo bản ghi rác cho bài chưa học
		}

		record := model.LessonProgress{
			UserID:              enrollment.UserID,
			LessonID:            lesson.ID,
			EnrollmentID:        enrollment.ID,
			Status:              status,
			ProgressPercent:     rating(percent),
			VideoWatchedSecs:    lesson.DurationMins * 60 * int(percent) / 100,
			TimeSpentSeconds:    lesson.DurationMins * 60 * int(percent) / 100,
			LastPositionSeconds: lesson.DurationMins * 60 * int(percent) / 100,
			ViewsCount:          1,
			CompletedAt:         completedAt,
			LastAccessedAt:      daysAgo(2),
		}

		if err := s.db.Where("user_id = ? AND lesson_id = ?", enrollment.UserID, lesson.ID).
			Attrs(record).
			FirstOrCreate(&record).Error; err != nil {
			return fmt.Errorf("failed to seed lesson progress: %w", err)
		}
	}
	return nil
}

// courseLessonsInOrder trả về toàn bộ bài học của khoá theo đúng thứ tự hiển thị.
func (s *Seeder) courseLessonsInOrder(courseID uuid.UUID) ([]model.Lesson, error) {
	var lessons []model.Lesson
	err := s.db.
		Joins("JOIN sections ON sections.id = lessons.section_id").
		Where("sections.course_id = ?", courseID).
		Order("sections.display_order ASC, lessons.display_order ASC").
		Find(&lessons).Error
	if err != nil {
		return nil, fmt.Errorf("failed to load lessons for course %s: %w", courseID, err)
	}
	return lessons, nil
}
