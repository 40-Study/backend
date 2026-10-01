package seeds

import (
	"fmt"

	"study.com/v1/internal/model"
)

// demoExpiredVoucherCode: voucher đã hết hạn do lane này tạo, vì 3 voucher của SeedDemoVouchers
// đều còn hạn — trang (app)/my-vouchers chỉ phân nhóm theo is_active + end_date.
const demoExpiredVoucherCode = "BACKTOSCHOOL30"

type userVoucherSpec struct {
	Email   string
	Code    string
	Source  string
	DaysAgo int
	Notes   string
}

var demoUserVouchers = []userVoucherSpec{
	{Email: "student1@demo.com", Code: "WELCOME20", Source: "manual", DaysAgo: 10, Notes: "Lưu để mua khoá Docker & Kubernetes."},
	{Email: "student1@demo.com", Code: "SUMMER50K", Source: "admin_grant", DaysAgo: 6, Notes: "Quà tặng học viên tích cực tháng 9."},
	{Email: "student1@demo.com", Code: demoExpiredVoucherCode, Source: "event_reward", DaysAgo: 40, Notes: "Phần thưởng sự kiện mùa tựu trường."},
	{Email: "student2@demo.com", Code: "WELCOME20", Source: "manual", DaysAgo: 8, Notes: "Ưu đãi chào mừng học viên mới."},
	{Email: "student2@demo.com", Code: "VIP100", Source: "admin_grant", DaysAgo: 3, Notes: "Tặng kèm khi đăng ký nhận bản tin."},
	{Email: "student2@demo.com", Code: demoExpiredVoucherCode, Source: "event_reward", DaysAgo: 35, Notes: "Phần thưởng sự kiện mùa tựu trường."},
}

// SeedDemoCertificates cấp chứng chỉ cho MỌI enrollment đã hoàn thành (completed_at NOT NULL) của
// học viên demo — đúng điều kiện CertificateService.IssueCertificate dùng — cho trang
// (app)/certificates và trang xác thực chứng chỉ. Số chứng chỉ theo định dạng thật
// CERT-YYYYMMDD-xxxxxxxx nhưng tất định (8 ký tự đầu enrollment id) để chạy lại không sinh số mới.
func (s *Seeder) SeedDemoCertificates(users map[string]model.User, _ map[string]model.Course) error {
	userIDs := make([]interface{}, 0, len(users))
	for _, u := range users {
		userIDs = append(userIDs, u.ID)
	}
	if len(userIDs) == 0 {
		return nil
	}

	var enrollments []model.Enrollment
	if err := s.db.Where("user_id IN ? AND completed_at IS NOT NULL", userIDs).Find(&enrollments).Error; err != nil {
		return fmt.Errorf("failed to load completed enrollments: %w", err)
	}

	for _, e := range enrollments {
		cert := model.Certificate{
			UserID:            e.UserID,
			CourseID:          e.CourseID,
			EnrollmentID:      e.ID,
			CertificateNumber: fmt.Sprintf("CERT-%s-%s", e.CompletedAt.Format("20060102"), e.ID.String()[:8]),
			IssuedAt:          *e.CompletedAt,
		}
		if err := s.db.Where("user_id = ? AND course_id = ?", e.UserID, e.CourseID).
			Attrs(cert).FirstOrCreate(&cert).Error; err != nil {
			return fmt.Errorf("failed to seed certificate for enrollment %s: %w", e.ID, err)
		}
	}
	return nil
}

// SeedDemoUserVouchers lưu voucher vào ví "Voucher của tôi" của student1/student2: 2 voucher còn
// dùng được + 1 voucher đã hết hạn mỗi người (trang (app)/my-vouchers). Khoá tự nhiên:
// (user_id, voucher_id) chưa xoá mềm.
func (s *Seeder) SeedDemoUserVouchers(users map[string]model.User) error {
	start, end := daysAgo(45), daysAgo(15)
	expired := model.Voucher{
		Code:                    demoExpiredVoucherCode,
		Name:                    "Mùa tựu trường - giảm 30%",
		Description:             "Giảm 30% tối đa 150.000đ cho khoá học bất kỳ, áp dụng đến hết đợt tựu trường.",
		DiscountUnit:            model.DiscountUnitMoney,
		DiscountMethod:          model.DiscountMethodPercent,
		DiscountPercent:         ptr(rating(30)),
		MaxDiscountMoney:        ptr(money(150000)),
		MinPurchaseMoney:        ptr(money(200000)),
		AcceptAllPaymentMethods: true,
		UsageLimit:              300,
		UsagePerUser:            1,
		StartDate:               &start,
		EndDate:                 &end,
		IsActive:                true,
	}
	if err := s.db.Where("code = ?", expired.Code).Attrs(expired).FirstOrCreate(&expired).Error; err != nil {
		return fmt.Errorf("failed to seed voucher %s: %w", expired.Code, err)
	}

	for _, spec := range demoUserVouchers {
		user, err := demoUser(users, spec.Email)
		if err != nil {
			return err
		}
		var voucher model.Voucher
		if err := s.db.Where("code = ?", spec.Code).First(&voucher).Error; err != nil {
			return fmt.Errorf("voucher %s not found (run SeedDemoVouchers first): %w", spec.Code, err)
		}
		uv := model.UserVoucher{UserID: user.ID, VoucherID: voucher.ID, Source: spec.Source, SavedAt: daysAgo(spec.DaysAgo), Notes: spec.Notes}
		if err := s.db.Where("user_id = ? AND voucher_id = ?", user.ID, voucher.ID).
			Attrs(uv).FirstOrCreate(&uv).Error; err != nil {
			return fmt.Errorf("failed to save voucher %s for %s: %w", spec.Code, spec.Email, err)
		}
	}
	return nil
}
