package seeds

import (
	"fmt"
	"log"
)

// SeedDemoData chạy toàn bộ seed dữ liệu demo theo đúng thứ tự phụ thuộc.
// Yêu cầu: permissions + system roles đã được seed trước (SeedAll).
// Toàn bộ bước đều idempotent — chạy lại nhiều lần không tạo bản ghi trùng.
func (s *Seeder) SeedDemoData() error {
	log.Println("=== Seeding demo data ===")

	users, err := s.SeedDemoUsers()
	if err != nil {
		return fmt.Errorf("demo users: %w", err)
	}

	if err := s.SeedDemoOrganizations(); err != nil {
		return fmt.Errorf("demo organizations: %w", err)
	}

	categories, err := s.SeedDemoCategories()
	if err != nil {
		return fmt.Errorf("demo categories: %w", err)
	}

	tags, err := s.SeedDemoTags()
	if err != nil {
		return fmt.Errorf("demo tags: %w", err)
	}

	courses, err := s.SeedDemoCourses(users, categories, tags)
	if err != nil {
		return fmt.Errorf("demo courses: %w", err)
	}

	if err := s.SeedDemoEnrollments(users, courses); err != nil {
		return fmt.Errorf("demo enrollments: %w", err)
	}

	if err := s.SeedDemoVouchers(); err != nil {
		return fmt.Errorf("demo vouchers: %w", err)
	}

	// S-P1-3 (QA 260927): quiz demo, trỏ đúng course/lesson thật (xem SeedDemoQuiz).
	if err := s.SeedDemoQuiz(courses); err != nil {
		return fmt.Errorf("demo quiz: %w", err)
	}

	// MVP "Cuộc thi" (contract §1.7): 1 cuộc thi đang diễn ra + 1 cuộc thi đã chốt.
	if err := s.SeedDemoContests(users); err != nil {
		return fmt.Errorf("demo contests: %w", err)
	}

	// parent P0-2 (QA 260927): liên kết cha-con active để test luồng phụ huynh (xem SeedDemoFamilyLinks).
	if err := s.SeedDemoFamilyLinks(users); err != nil {
		return fmt.Errorf("demo family links: %w", err)
	}

	log.Println("=== Demo data seeded successfully ===")
	return nil
}
