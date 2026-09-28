package seeds

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"study.com/v1/internal/model"
	"study.com/v1/internal/repository"
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

	// S-P0-3/S-P0-4 (QA 260927): dùng CHUNG repository thật (không viết lại công thức tính %) để
	// tính lại completed_lessons/total_lessons/progress_percentage sau khi seed lesson_progress —
	// xem recalcEnrollmentProgress.
	enrollmentRepo := repository.NewEnrollmentRepository(s.db)

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
		if err := s.recalcEnrollmentProgress(enrollmentRepo, enrollment); err != nil {
			return err
		}
	}

	log.Printf("Seeded %d enrollments\n", len(demoEnrollments))
	return nil
}

// recalcEnrollmentProgress (S-P0-3/S-P0-4, QA 260927): trước đây upsertEnrollment chỉ set thẳng
// ProgressPercent bằng SQL, KHÔNG đụng tới completed_lessons/total_lessons — 3 cột dẫn xuất này
// lệch nhau ngay từ lúc seed (vd "Git & GitHub" hiện 100% nhưng "0/0 bài học" — verify-260927-
// student-admin.md, root cause enrollment_service.go recalculateProgress không hề chạy qua seed).
// Hàm này gọi ĐÚNG các query đếm mà luồng ghi tiến độ thật dùng
// (EnrollmentRepository.CountCompletedMandatory/CountTotalMandatory — cùng hàm
// enrollment_service.go#recalculateProgress gọi nội bộ, không phải bản sao thứ hai của logic đếm)
// rồi ghi lại qua UpdateEnrollmentProgress, nên 3 số luôn khớp nhau như khi có hành động thật.
func (s *Seeder) recalcEnrollmentProgress(repo repository.EnrollmentRepositoryInterface, enrollment model.Enrollment) error {
	ctx := context.Background()

	completed, err := repo.CountCompletedMandatory(ctx, enrollment.ID)
	if err != nil {
		return fmt.Errorf("failed to count completed lessons for enrollment %s: %w", enrollment.ID, err)
	}
	total, err := repo.CountTotalMandatory(ctx, enrollment.CourseID)
	if err != nil {
		return fmt.Errorf("failed to count total lessons for course %s: %w", enrollment.CourseID, err)
	}

	var percent decimal.Decimal
	if total > 0 {
		percent = decimal.NewFromInt(completed).Mul(decimal.NewFromInt(100)).Div(decimal.NewFromInt(total))
	}

	if err := repo.UpdateEnrollmentProgress(ctx, enrollment.ID, percent, int(completed), int(total), time.Now()); err != nil {
		return fmt.Errorf("failed to update enrollment progress for %s: %w", enrollment.ID, err)
	}
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
