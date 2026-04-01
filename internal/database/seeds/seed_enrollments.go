package seeds

import (
	"fmt"
	"log"
	"time"

	"study.com/v1/internal/model"
)

// enrollmentSpec defines which student enrolls in which course (by slug)
type enrollmentSpec struct {
	studentEmail string
	courseSlugs  []string
}

func (s *Seeder) SeedEnrollments() error {
	log.Println("Seeding enrollments...")

	specs := []enrollmentSpec{
		{
			studentEmail: "student1@demo.com",
			courseSlugs: []string{
				"react-nextjs-tu-co-ban-den-nang-cao",
				"flutter-mobile-development",
				"git-github-cho-nguoi-moi-bat-dau",
			},
		},
		{
			studentEmail: "student2@demo.com",
			courseSlugs: []string{
				"python-cho-khoa-hoc-du-lieu",
			},
		},
	}

	// Build user email -> model map
	var users []model.User
	if err := s.db.Find(&users).Error; err != nil {
		return fmt.Errorf("failed to load users: %w", err)
	}
	userMap := make(map[string]model.User)
	for _, u := range users {
		userMap[u.Email] = u
	}

	// Build course slug -> model map
	var courses []model.Course
	if err := s.db.Find(&courses).Error; err != nil {
		return fmt.Errorf("failed to load courses: %w", err)
	}
	courseMap := make(map[string]model.Course)
	for _, c := range courses {
		courseMap[c.Slug] = c
	}

	count := 0
	now := time.Now()
	for _, spec := range specs {
		student, ok := userMap[spec.studentEmail]
		if !ok {
			log.Printf("Warning: student %s not found\n", spec.studentEmail)
			continue
		}

		for _, slug := range spec.courseSlugs {
			course, ok := courseMap[slug]
			if !ok {
				log.Printf("Warning: course %s not found\n", slug)
				continue
			}

			enrollment := model.Enrollment{
				UserID:     student.ID,
				CourseID:   course.ID,
				EnrolledAt: now,
			}

			result := s.db.Where("user_id = ? AND course_id = ?", student.ID, course.ID).
				FirstOrCreate(&enrollment)
			if result.Error != nil {
				return fmt.Errorf("failed to seed enrollment %s -> %s: %w",
					spec.studentEmail, slug, result.Error)
			}

			// Bump total_students on the course if newly created
			if result.RowsAffected > 0 {
				s.db.Model(&course).UpdateColumn("total_students",
					s.db.Raw("total_students + 1"))
			}

			count++
			log.Printf("Seeded enrollment: %s -> %s\n", spec.studentEmail, slug)
		}
	}

	log.Printf("Successfully seeded %d enrollments\n", count)
	return nil
}
