package seeds

import (
	"fmt"
	"log"
	"time"

	"study.com/v1/internal/model"
)

func (s *Seeder) SeedClasses() error {
	log.Println("Seeding classes...")

	// Load required users and courses
	var teacher1, teacher2 model.User
	if err := s.db.Where("email = ?", "teacher1@demo.com").First(&teacher1).Error; err != nil {
		return fmt.Errorf("teacher1 not found: %w", err)
	}
	if err := s.db.Where("email = ?", "teacher2@demo.com").First(&teacher2).Error; err != nil {
		return fmt.Errorf("teacher2 not found: %w", err)
	}

	var student1, student2 model.User
	if err := s.db.Where("email = ?", "student1@demo.com").First(&student1).Error; err != nil {
		return fmt.Errorf("student1 not found: %w", err)
	}
	if err := s.db.Where("email = ?", "student2@demo.com").First(&student2).Error; err != nil {
		return fmt.Errorf("student2 not found: %w", err)
	}

	var course1, course2 model.Course
	if err := s.db.Where("slug = ?", "react-nextjs-tu-co-ban-den-nang-cao").First(&course1).Error; err != nil {
		log.Printf("Warning: react course not found: %v\n", err)
	}
	if err := s.db.Where("slug = ?", "python-cho-khoa-hoc-du-lieu").First(&course2).Error; err != nil {
		log.Printf("Warning: python course not found: %v\n", err)
	}

	startDate := time.Now()
	endDate := startDate.AddDate(0, 3, 0) // 3 months
	maxStudents := 30

	classConfigs := []struct {
		name        string
		description string
		courseID    *model.Course
		teacher     model.User
		students    []model.User
		schedules   []struct {
			dayOfWeek int
			startTime string
			endTime   string
			room      string
		}
	}{
		{
			name:        "Lop React & Next.js K1/2026",
			description: "Lop hoc React va Next.js khoa 1 nam 2026",
			courseID:    &course1,
			teacher:     teacher1,
			students:    []model.User{student1},
			schedules: []struct {
				dayOfWeek int
				startTime string
				endTime   string
				room      string
			}{
				{dayOfWeek: 2, startTime: "18:00", endTime: "20:00", room: "Phong A101"}, // Mon
				{dayOfWeek: 4, startTime: "18:00", endTime: "20:00", room: "Phong A101"}, // Wed
			},
		},
		{
			name:        "Lop Python Data Science K1/2026",
			description: "Lop hoc Python cho Khoa hoc Du lieu khoa 1 nam 2026",
			courseID:    &course2,
			teacher:     teacher2,
			students:    []model.User{student2},
			schedules: []struct {
				dayOfWeek int
				startTime string
				endTime   string
				room      string
			}{
				{dayOfWeek: 3, startTime: "19:00", endTime: "21:00", room: "Phong B205"}, // Tue
				{dayOfWeek: 5, startTime: "19:00", endTime: "21:00", room: "Phong B205"}, // Thu
			},
		},
	}

	for _, cfg := range classConfigs {
		desc := cfg.description
		class := model.Class{
			Name:        cfg.name,
			Description: &desc,
			Status:      "active",
			MaxStudents: &maxStudents,
			StartDate:   &startDate,
			EndDate:     &endDate,
		}
		if cfg.courseID != nil && cfg.courseID.ID.String() != "00000000-0000-0000-0000-000000000000" {
			class.CourseID = &cfg.courseID.ID
		}

		result := s.db.Where("name = ?", cfg.name).FirstOrCreate(&class)
		if result.Error != nil {
			return fmt.Errorf("failed to seed class %s: %w", cfg.name, result.Error)
		}

		// Assign teacher
		teacherClass := model.TeacherClass{
			TeacherID: cfg.teacher.ID,
			ClassID:   class.ID,
			Role:      "primary",
		}
		if err := s.db.Where("teacher_id = ? AND class_id = ?", cfg.teacher.ID, class.ID).
			FirstOrCreate(&teacherClass).Error; err != nil {
			log.Printf("Warning: could not assign teacher to class %s: %v\n", cfg.name, err)
		}

		// Enroll students
		for _, student := range cfg.students {
			studentClass := model.StudentClass{
				StudentID: student.ID,
				ClassID:   class.ID,
				Status:    "active",
			}
			if err := s.db.Where("student_id = ? AND class_id = ?", student.ID, class.ID).
				FirstOrCreate(&studentClass).Error; err != nil {
				log.Printf("Warning: could not enroll student in class %s: %v\n", cfg.name, err)
			}
		}

		// Create schedules
		for _, sched := range cfg.schedules {
			room := sched.room
			schedule := model.ClassSchedule{
				ClassID:   class.ID,
				DayOfWeek: sched.dayOfWeek,
				StartTime: sched.startTime,
				EndTime:   sched.endTime,
				Room:      &room,
			}
			if err := s.db.Where("class_id = ? AND day_of_week = ? AND start_time = ?",
				class.ID, sched.dayOfWeek, sched.startTime).
				FirstOrCreate(&schedule).Error; err != nil {
				log.Printf("Warning: could not create schedule for class %s: %v\n", cfg.name, err)
			}
		}

		log.Printf("Seeded class: %s\n", cfg.name)
	}

	log.Printf("Successfully seeded %d classes\n", len(classConfigs))
	return nil
}
