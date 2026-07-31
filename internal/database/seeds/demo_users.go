package seeds

import (
	"errors"
	"fmt"
	"log"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"study.com/v1/internal/model"
	"study.com/v1/internal/utils"
)

type demoUserSpec struct {
	Email      string
	UserName   string
	FullName   string
	SystemRole string
	Bio        string
	// Teacher-only
	Specialization  string
	Education       string
	ExperienceYears int
}

var demoUsers = []demoUserSpec{
	{Email: "admin@demo.com", UserName: "admin", FullName: "Admin Demo", SystemRole: "SYSTEM_ADMIN",
		Bio: "Quản trị viên hệ thống 40Study."},
	{Email: "teacher1@demo.com", UserName: "teacher1", FullName: "Nguyễn Văn A", SystemRole: "TEACHER",
		Bio:             "Giảng viên Frontend, 8 năm kinh nghiệm React/Next.js.",
		Specialization:  "Lập trình Web (React, Next.js)",
		Education:       "Thạc sĩ Khoa học Máy tính - ĐH Bách Khoa",
		ExperienceYears: 8},
	{Email: "teacher2@demo.com", UserName: "teacher2", FullName: "Trần Thị B", SystemRole: "TEACHER",
		Bio:             "Giảng viên Data Science & DevOps.",
		Specialization:  "Khoa học Dữ liệu, DevOps",
		Education:       "Tiến sĩ Trí tuệ Nhân tạo - ĐH Công nghệ",
		ExperienceYears: 10},
	{Email: "student1@demo.com", UserName: "student1", FullName: "Lê Văn C", SystemRole: "STUDENT",
		Bio: "Sinh viên năm 3, đang học Web và Mobile."},
	{Email: "student2@demo.com", UserName: "student2", FullName: "Phạm Thị D", SystemRole: "STUDENT",
		Bio: "Người mới bắt đầu, quan tâm Data Science."},
	{Email: "parent1@demo.com", UserName: "parent1", FullName: "Hoàng Văn E", SystemRole: "PARENT",
		Bio: "Phụ huynh của Lê Văn C."},
}

// SeedDemoUsers tạo tài khoản demo, gán system role và hồ sơ giáo viên.
// Idempotent: existing accounts are reused only when they match demo ownership.
func (s *Seeder) SeedDemoUsers() (map[string]model.User, error) {
	log.Println("Seeding demo users...")

	hash, err := utils.HashPassword(DemoPassword)
	if err != nil {
		return nil, fmt.Errorf("failed to hash demo password: %w", err)
	}

	result := make(map[string]model.User, len(demoUsers))

	for _, spec := range demoUsers {
		var user model.User

		err := s.db.Where("email = ?", spec.Email).First(&user).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
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
		} else if err != nil {
			return nil, fmt.Errorf("failed to look up demo user %s: %w", spec.Email, err)
		} else if !isDemoOwnedUser(user, spec) {
			return nil, fmt.Errorf("refusing to reuse existing non-demo account %s", spec.Email)
		}

		if err := s.assignSystemRole(user.ID, spec.SystemRole); err != nil {
			return nil, err
		}

		if spec.SystemRole == "TEACHER" {
			if err := s.upsertTeacherProfile(user.ID, spec); err != nil {
				return nil, err
			}
		}

		result[spec.Email] = user
	}

	log.Printf("Seeded %d demo users (password: %s)\n", len(result), DemoPassword)
	return result, nil
}

func isDemoOwnedUser(user model.User, spec demoUserSpec) bool {
	return user.Email == spec.Email &&
		user.UserName == spec.UserName &&
		utils.CheckPassword(DemoPassword, user.PasswordHash)
}

// assignSystemRole gán system role (theo tên) cho user nếu chưa được gán.
func (s *Seeder) assignSystemRole(userID uuid.UUID, roleName string) error {
	var role model.SystemRole
	if err := s.db.Where("name = ?", roleName).First(&role).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			// roles.json chưa được seed -> báo rõ thay vì bỏ qua im lặng
			return fmt.Errorf("system role %s not found; run permission/role seeding first", roleName)
		}
		return fmt.Errorf("failed to look up system role %s: %w", roleName, err)
	}

	link := model.UserSystemRole{
		UserID:       userID,
		SystemRoleID: role.ID,
		Status:       model.UserSystemRoleStatusActive,
	}
	if err := s.db.Where("user_id = ? AND system_role_id = ?", userID, role.ID).
		FirstOrCreate(&link).Error; err != nil {
		return fmt.Errorf("failed to assign role %s to user %s: %w", roleName, userID, err)
	}
	return nil
}

// upsertTeacherProfile tạo hồ sơ giáo viên nếu chưa có.
func (s *Seeder) upsertTeacherProfile(userID uuid.UUID, spec demoUserSpec) error {
	profile := model.TeacherProfile{
		UserID:          userID,
		Specialization:  ptr(spec.Specialization),
		Education:       ptr(spec.Education),
		ExperienceYears: ptr(spec.ExperienceYears),
	}
	if err := s.db.Where("user_id = ?", userID).
		Attrs(profile).
		FirstOrCreate(&profile).Error; err != nil {
		return fmt.Errorf("failed to seed teacher profile for %s: %w", spec.Email, err)
	}
	return nil
}

// SeedDemoOrganizations tạo các tổ chức/trung tâm demo.
func (s *Seeder) SeedDemoOrganizations() error {
	log.Println("Seeding demo organizations...")

	orgs := []model.Organization{
		{Name: "Trung tâm Lập trình ABC", Description: nullStr("Trung tâm đào tạo lập trình thực chiến."), Status: "active"},
		{Name: "Học viện CNTT XYZ", Description: nullStr("Học viện công nghệ thông tin đa ngành."), Status: "active"},
	}

	for _, org := range orgs {
		record := org
		if err := s.db.Where("name = ?", org.Name).
			Attrs(record).
			FirstOrCreate(&record).Error; err != nil {
			return fmt.Errorf("failed to seed organization %s: %w", org.Name, err)
		}
	}

	log.Printf("Seeded %d organizations\n", len(orgs))
	return nil
}
