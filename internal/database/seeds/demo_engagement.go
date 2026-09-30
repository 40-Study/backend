package seeds

import (
	"errors"
	"fmt"
	"log"

	"gorm.io/gorm"
	"study.com/v1/internal/model"
	"study.com/v1/internal/utils"
)

// demoExtraStudents là học viên demo PHỤ, chỉ để bảng xếp hạng (/leaderboard, luôn xếp theo
// user_points.total_points của mọi user active) có đủ ≥5 dòng học viên có điểm. Không ghi danh
// khoá nào, nên không ảnh hưởng số liệu học tập của student1/student2.
var demoExtraStudents = []demoUserSpec{
	{Email: "student3@demo.com", UserName: "student3", FullName: "Vũ Thu Hà", SystemRole: "STUDENT",
		Bio: "Nhân viên văn phòng học thêm lập trình buổi tối."},
	{Email: "student4@demo.com", UserName: "student4", FullName: "Đặng Quốc Huy", SystemRole: "STUDENT",
		Bio: "Học sinh lớp 12, thích làm game và ứng dụng di động."},
	{Email: "student5@demo.com", UserName: "student5", FullName: "Bùi Ngọc Lan", SystemRole: "STUDENT",
		Bio: "Sinh viên kế toán chuyển hướng sang phân tích dữ liệu."},
}

// SeedDemoEngagement seed toàn bộ dữ liệu tương tác cho các trang (app)/notifications,
// (app)/achievements, (app)/leaderboard, (app)/coins, (app)/certificates, (app)/my-vouchers,
// cùng đánh giá khoá học, wishlist và ghi chú bài học.
//
// Phụ thuộc: phải chạy SAU SeedDemoUsers, SeedDemoCourses, SeedDemoEnrollments và
// SeedDemoVouchers (dùng enrollment đã hoàn thành để cấp chứng chỉ, voucher có sẵn để gán).
// Tạo thêm 3 học viên phụ student3..student5@demo.com (mật khẩu DemoPassword).
// Idempotent: mọi bản ghi tra theo khoá tự nhiên trước khi tạo.
func (s *Seeder) SeedDemoEngagement(users map[string]model.User, courses map[string]model.Course) error {
	log.Println("Seeding demo engagement...")

	all, err := s.SeedDemoExtraStudents(users)
	if err != nil {
		return fmt.Errorf("demo extra students: %w", err)
	}

	steps := []struct {
		name string
		run  func() error
	}{
		{"notifications", func() error { return s.SeedDemoNotifications(all, courses) }},
		{"gamification", func() error { return s.SeedDemoGamification(all) }},
		{"coins", func() error { return s.SeedDemoCoins(all) }},
		{"certificates", func() error { return s.SeedDemoCertificates(all, courses) }},
		{"user vouchers", func() error { return s.SeedDemoUserVouchers(all) }},
		{"reviews", func() error { return s.SeedDemoReviews(all, courses) }},
		{"wishlists & notes", func() error { return s.SeedDemoWishlistsAndNotes(all, courses) }},
	}
	for _, step := range steps {
		if err := step.run(); err != nil {
			return fmt.Errorf("demo %s: %w", step.name, err)
		}
	}

	log.Println("Seeded demo engagement")
	return nil
}

// SeedDemoExtraStudents tạo học viên demo phụ (student3..student5) theo đúng cách của
// SeedDemoUsers (hash DemoPassword, gán role STUDENT, từ chối dùng lại tài khoản không phải demo).
// Trả về bản sao của users đã gộp thêm học viên phụ; map đầu vào không bị sửa.
func (s *Seeder) SeedDemoExtraStudents(users map[string]model.User) (map[string]model.User, error) {
	hash, err := utils.HashPassword(DemoPassword)
	if err != nil {
		return nil, fmt.Errorf("failed to hash demo password: %w", err)
	}

	all := make(map[string]model.User, len(users)+len(demoExtraStudents))
	for k, v := range users {
		all[k] = v
	}

	for _, spec := range demoExtraStudents {
		var user model.User
		err := s.db.Where("email = ?", spec.Email).First(&user).Error
		switch {
		case errors.Is(err, gorm.ErrRecordNotFound):
			user = model.User{
				Email:        spec.Email,
				PasswordHash: hash,
				UserName:     spec.UserName,
				FullName:     ptr(spec.FullName),
				Bio:          ptr(spec.Bio),
				IsVerified:   true,
				IsActive:     true,
			}
			if err := s.db.Create(&user).Error; err != nil {
				return nil, fmt.Errorf("failed to seed user %s: %w", spec.Email, err)
			}
		case err != nil:
			return nil, fmt.Errorf("failed to look up demo user %s: %w", spec.Email, err)
		case !isDemoOwnedUser(user, spec):
			return nil, fmt.Errorf("refusing to reuse existing non-demo account %s", spec.Email)
		}

		if err := s.assignSystemRole(user.ID, spec.SystemRole); err != nil {
			return nil, err
		}
		all[spec.Email] = user
	}
	return all, nil
}

// demoUser lấy user demo theo email, báo lỗi rõ ràng thay vì dùng uuid rỗng.
func demoUser(users map[string]model.User, email string) (model.User, error) {
	u, ok := users[email]
	if !ok {
		return model.User{}, fmt.Errorf("demo user %s not found", email)
	}
	return u, nil
}

// demoCourse lấy khoá demo theo slug, báo lỗi rõ ràng nếu thiếu.
func demoCourse(courses map[string]model.Course, slug string) (model.Course, error) {
	c, ok := courses[slug]
	if !ok {
		return model.Course{}, fmt.Errorf("demo course %s not found", slug)
	}
	return c, nil
}
