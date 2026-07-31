package seeds

import (
	"fmt"
	"log"

	"study.com/v1/internal/model"
)

var demoCategories = []model.Category{
	{Name: "Lập trình Web", Slug: "lap-trinh-web", DisplayOrder: 1, IsActive: true,
		Description: ptr("HTML, CSS, JavaScript, React, Next.js và hệ sinh thái web hiện đại.")},
	{Name: "Lập trình Mobile", Slug: "lap-trinh-mobile", DisplayOrder: 2, IsActive: true,
		Description: ptr("Flutter, React Native, Swift, Kotlin cho ứng dụng di động.")},
	{Name: "Khoa học Dữ liệu", Slug: "khoa-hoc-du-lieu", DisplayOrder: 3, IsActive: true,
		Description: ptr("Python, Pandas, trực quan hoá dữ liệu và Machine Learning.")},
	{Name: "DevOps & Cloud", Slug: "devops-cloud", DisplayOrder: 4, IsActive: true,
		Description: ptr("Docker, Kubernetes, CI/CD và triển khai trên cloud.")},
	{Name: "Thiết kế UI/UX", Slug: "thiet-ke-ui-ux", DisplayOrder: 5, IsActive: true,
		Description: ptr("Figma, nguyên tắc thiết kế giao diện và trải nghiệm người dùng.")},
	{Name: "Lập trình Game", Slug: "lap-trinh-game", DisplayOrder: 6, IsActive: true,
		Description: ptr("Unity, Godot và tư duy thiết kế gameplay.")},
}

var demoTags = []string{
	"JavaScript", "TypeScript", "React", "Next.js", "Node.js",
	"Python", "Django", "FastAPI", "Machine Learning",
	"Flutter", "React Native", "Swift", "Kotlin",
	"Docker", "Kubernetes", "AWS", "GCP",
	"Figma", "CSS", "Tailwind", "Git",
}

// SeedDemoCategories tạo danh mục khoá học, trả về map slug -> Category.
func (s *Seeder) SeedDemoCategories() (map[string]model.Category, error) {
	log.Println("Seeding demo categories...")

	result := make(map[string]model.Category, len(demoCategories))
	for _, cat := range demoCategories {
		record := cat
		if err := s.db.Where("slug = ?", cat.Slug).
			Attrs(record).
			FirstOrCreate(&record).Error; err != nil {
			return nil, fmt.Errorf("failed to seed category %s: %w", cat.Slug, err)
		}
		result[cat.Slug] = record
	}

	log.Printf("Seeded %d categories\n", len(result))
	return result, nil
}

// SeedDemoTags tạo tag, trả về map tên tag -> Tag.
func (s *Seeder) SeedDemoTags() (map[string]model.Tag, error) {
	log.Println("Seeding demo tags...")

	result := make(map[string]model.Tag, len(demoTags))
	for _, name := range demoTags {
		tag := model.Tag{Name: name, Slug: slugify(name)}
		if err := s.db.Where("name = ?", name).
			Attrs(tag).
			FirstOrCreate(&tag).Error; err != nil {
			return nil, fmt.Errorf("failed to seed tag %s: %w", name, err)
		}
		result[name] = tag
	}

	log.Printf("Seeded %d tags\n", len(result))
	return result, nil
}
