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

	log.Println("=== Demo data seeded successfully ===")
	return nil
}
