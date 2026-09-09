package seeds

import (
	"errors"
	"fmt"
	"log"

	"gorm.io/gorm"
	"study.com/v1/internal/model"
)

// demoCertificateSpec mô tả chứng chỉ demo cấp cho một học viên đã hoàn thành khoá.
type demoCertificateSpec struct {
	StudentEmail string
	CourseSlug   string
	// Number cố định để trang tra cứu công khai luôn có mã kiểm thử ổn định.
	Number string
}

var demoCertificates = []demoCertificateSpec{
	{
		StudentEmail: "student1@demo.com",
		CourseSlug:   "git-github-cho-nguoi-moi-bat-dau",
		Number:       "CERT-DEMO-GIT00001",
	},
}

// SeedDemoCertificates cấp chứng chỉ cho các ghi danh đã hoàn thành 100%
// và liên kết ngược certificate_id vào bản ghi enrollment.
//
// Yêu cầu chạy SAU SeedDemoEnrollments: chứng chỉ tra enrollment theo
// (user_id, course_id) và chỉ cấp khi enrollment đã có completed_at.
func (s *Seeder) SeedDemoCertificates(
	users map[string]model.User,
	courses map[string]model.Course,
) error {
	log.Println("Seeding demo certificates...")

	issued := 0
	for _, spec := range demoCertificates {
		student, ok := users[spec.StudentEmail]
		if !ok {
			return fmt.Errorf("certificate student %s not found", spec.StudentEmail)
		}
		course, ok := courses[spec.CourseSlug]
		if !ok {
			return fmt.Errorf("certificate course %s not found", spec.CourseSlug)
		}

		var enrollment model.Enrollment
		err := s.db.Where("user_id = ? AND course_id = ?", student.ID, course.ID).
			First(&enrollment).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return fmt.Errorf("no enrollment for %s on %s — run SeedDemoEnrollments first",
				spec.StudentEmail, spec.CourseSlug)
		}
		if err != nil {
			return fmt.Errorf("failed to look up enrollment for certificate: %w", err)
		}

		// Chỉ cấp chứng chỉ khi khoá đã hoàn thành — giữ đúng ràng buộc của
		// CertificateService.IssueCertificate để dữ liệu demo không mâu thuẫn logic thật.
		if enrollment.CompletedAt == nil {
			log.Printf("skip certificate: enrollment %s/%s chưa hoàn thành\n",
				spec.StudentEmail, spec.CourseSlug)
			continue
		}

		cert := model.Certificate{
			UserID:            student.ID,
			CourseID:          course.ID,
			EnrollmentID:      enrollment.ID,
			CertificateNumber: spec.Number,
			IssuedAt:          daysAgo(3),
		}
		if err := s.db.Where("user_id = ? AND course_id = ?", student.ID, course.ID).
			Attrs(cert).
			FirstOrCreate(&cert).Error; err != nil {
			return fmt.Errorf("failed to seed certificate %s: %w", spec.Number, err)
		}

		// Liên kết ngược để trang "Khoá học của tôi" hiển thị nút tải chứng chỉ.
		if enrollment.CertificateID == nil || *enrollment.CertificateID != cert.ID {
			if err := s.db.Model(&model.Enrollment{}).
				Where("id = ?", enrollment.ID).
				Update("certificate_id", cert.ID).Error; err != nil {
				return fmt.Errorf("failed to link certificate to enrollment: %w", err)
			}
		}
		issued++
	}

	log.Printf("Seeded %d certificates\n", issued)
	return nil
}
