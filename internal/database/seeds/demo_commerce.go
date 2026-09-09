package seeds

import (
	"fmt"
	"log"

	"github.com/google/uuid"
	"study.com/v1/internal/model"
)

// demoBasketSpec mô tả giỏ hàng và danh sách yêu thích của một học viên.
// Chỉ đặt vào giỏ những khoá học viên CHƯA ghi danh — giỏ hàng chứa khoá
// đã mua là dữ liệu mâu thuẫn và làm hỏng luồng thanh toán demo.
type demoBasketSpec struct {
	StudentEmail  string
	CartSlugs     []string
	WishlistSlugs []string
}

var demoBaskets = []demoBasketSpec{
	{
		StudentEmail:  "student1@demo.com",
		CartSlugs:     []string{"docker-kubernetes-thuc-chien"},
		WishlistSlugs: []string{"python-cho-khoa-hoc-du-lieu"},
	},
	{
		StudentEmail:  "student2@demo.com",
		CartSlugs:     []string{"react-nextjs-tu-co-ban-den-nang-cao"},
		WishlistSlugs: []string{"flutter-mobile-development", "docker-kubernetes-thuc-chien"},
	},
}

// SeedDemoBaskets tạo giỏ hàng và wishlist demo.
// Idempotent: cả hai bảng đều có unique index (user_id, course_id).
func (s *Seeder) SeedDemoBaskets(
	users map[string]model.User,
	courses map[string]model.Course,
) error {
	log.Println("Seeding demo cart & wishlist...")

	cartCount, wishCount := 0, 0
	for _, spec := range demoBaskets {
		student, ok := users[spec.StudentEmail]
		if !ok {
			return fmt.Errorf("basket owner %s not found", spec.StudentEmail)
		}

		for _, slug := range spec.CartSlugs {
			course, ok := courses[slug]
			if !ok {
				return fmt.Errorf("cart course %s not found", slug)
			}
			enrolled, err := s.isEnrolled(student.ID, course.ID)
			if err != nil {
				return err
			}
			if enrolled {
				log.Printf("skip cart item: %s đã ghi danh %s\n", spec.StudentEmail, slug)
				continue
			}

			item := model.CartItem{UserID: student.ID, CourseID: course.ID}
			if err := s.db.Where("user_id = ? AND course_id = ?", student.ID, course.ID).
				Attrs(item).
				FirstOrCreate(&item).Error; err != nil {
				return fmt.Errorf("failed to seed cart item: %w", err)
			}
			cartCount++
		}

		for _, slug := range spec.WishlistSlugs {
			course, ok := courses[slug]
			if !ok {
				return fmt.Errorf("wishlist course %s not found", slug)
			}

			item := model.Wishlist{UserID: student.ID, CourseID: course.ID}
			if err := s.db.Where("user_id = ? AND course_id = ?", student.ID, course.ID).
				Attrs(item).
				FirstOrCreate(&item).Error; err != nil {
				return fmt.Errorf("failed to seed wishlist item: %w", err)
			}
			wishCount++
		}
	}

	log.Printf("Seeded %d cart items, %d wishlist items\n", cartCount, wishCount)
	return nil
}

// isEnrolled kiểm tra học viên đã ghi danh khoá chưa.
// Dùng Count thay vì First để GORM không ghi log ErrRecordNotFound ở mức error
// cho một phép kiểm tra tồn tại hoàn toàn bình thường.
func (s *Seeder) isEnrolled(userID, courseID uuid.UUID) (bool, error) {
	var count int64
	if err := s.db.Model(&model.Enrollment{}).
		Where("user_id = ? AND course_id = ?", userID, courseID).
		Count(&count).Error; err != nil {
		return false, fmt.Errorf("failed to check enrollment: %w", err)
	}
	return count > 0, nil
}
