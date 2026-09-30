package seeds

import (
	"fmt"
	"log"
	"time"

	"study.com/v1/internal/model"
)

// demoPlatformFeePercent là % phí nền tảng CHỐT vào đơn demo (orders.platform_fee_percent). Ví giảng
// viên (WalletRepository.GetTeacherEarnings) trừ đúng số phí đã chốt này, nên trang ví hiện cả phần phí.
const demoPlatformFeePercent = 10

type demoOrderSpec struct {
	Number       string // khoá tự nhiên orders.order_number
	StudentEmail string
	CourseSlug   string
	PaidDaysAgo  int
}

// demoOrderSpecs: 1 đơn hoàn tất cho mỗi khoá CÓ PHÍ mà học viên demo đã ghi danh (demoEnrollments).
// Ví giảng viên chỉ tính từ order_items của đơn completed — không có đơn thì ví luôn 0đ.
var demoOrderSpecs = []demoOrderSpec{
	{Number: "DEMO-ORD-0001", StudentEmail: "student1@demo.com", CourseSlug: "react-nextjs-tu-co-ban-den-nang-cao", PaidDaysAgo: 20},
	{Number: "DEMO-ORD-0002", StudentEmail: "student1@demo.com", CourseSlug: "flutter-mobile-development", PaidDaysAgo: 20},
	{Number: "DEMO-ORD-0003", StudentEmail: "student2@demo.com", CourseSlug: "python-cho-khoa-hoc-du-lieu", PaidDaysAgo: 20},
}

type demoBankSpec struct{ Bank, Number, Holder string }

// demoTeacherBanks: thông tin ngân hàng bắt buộc để giáo viên gửi được yêu cầu rút (hasBankInfo).
var demoTeacherBanks = map[string]demoBankSpec{
	"teacher1@demo.com": {Bank: "Vietcombank", Number: "0071001234567", Holder: "NGUYEN VAN A"},
	"teacher2@demo.com": {Bank: "Techcombank", Number: "19036677889900", Holder: "TRAN THI B"},
}

type demoPayoutSpec struct {
	TeacherEmail     string
	Amount           int64
	Status           string
	Note             string // khoá tự nhiên (cùng instructor_id)
	CreatedDaysAgo   int
	ProcessedDaysAgo int // 0 = chưa xử lý
	RejectionReason  string
	TransactionID    string
}

// demoPayoutSpecs phủ đủ trạng thái model.PayoutStatuses. Ràng buộc nghiệp vụ được giữ: mỗi giảng
// viên tối đa 1 yêu cầu pending/approved (PayoutOpenStatuses) và tổng giữ chỗ không vượt thu nhập:
//   - teacher1: thu nhập (449.100 + 539.100) = 988.200; giữ chỗ 300.000 + 250.000 → còn 438.200.
//   - teacher2: thu nhập 629.100; giữ chỗ 200.000 → còn 429.100.
var demoPayoutSpecs = []demoPayoutSpec{
	{TeacherEmail: "teacher1@demo.com", Amount: 300000, Status: model.PayoutStatusCompleted,
		Note: "Rút doanh thu đợt 1 khoá React + Next.js", CreatedDaysAgo: 15, ProcessedDaysAgo: 13, TransactionID: "FT26258901234"},
	{TeacherEmail: "teacher1@demo.com", Amount: 200000, Status: model.PayoutStatusRejected,
		Note: "Rút doanh thu khoá Flutter", CreatedDaysAgo: 8, ProcessedDaysAgo: 7,
		RejectionReason: "Tên chủ tài khoản không khớp hồ sơ giảng viên, vui lòng cập nhật lại rồi gửi yêu cầu mới."},
	{TeacherEmail: "teacher1@demo.com", Amount: 250000, Status: model.PayoutStatusPending,
		Note: "Rút doanh thu tháng 9", CreatedDaysAgo: 1},
	{TeacherEmail: "teacher2@demo.com", Amount: 100000, Status: model.PayoutStatusCancelled,
		Note: "Nhập nhầm số tiền, huỷ để gửi lại", CreatedDaysAgo: 9},
	{TeacherEmail: "teacher2@demo.com", Amount: 200000, Status: model.PayoutStatusApproved,
		Note: "Rút doanh thu khoá Python cho Khoa học Dữ liệu", CreatedDaysAgo: 3, ProcessedDaysAgo: 2},
}

// SeedDemoTeacherPayouts tạo đơn hàng hoàn tất (nguồn thu nhập của ví giảng viên), điền thông tin ngân
// hàng vào hồ sơ giảng viên và tạo yêu cầu rút tiền nhiều trạng thái cho (teacher)/teacher/wallet và
// (admin)/admin/withdrawals.
func (s *Seeder) SeedDemoTeacherPayouts(users map[string]model.User, courses map[string]model.Course) error {
	for _, spec := range demoOrderSpecs {
		if err := s.upsertDemoOrder(spec, users, courses); err != nil {
			return err
		}
	}
	for email, bank := range demoTeacherBanks {
		if err := s.ensureDemoTeacherBank(users[email], bank); err != nil {
			return err
		}
	}
	for _, spec := range demoPayoutSpecs {
		if err := s.upsertDemoPayout(spec, users); err != nil {
			return err
		}
	}
	log.Printf("Seeded %d demo orders, %d payouts\n", len(demoOrderSpecs), len(demoPayoutSpecs))
	return nil
}

func (s *Seeder) upsertDemoOrder(spec demoOrderSpec, users map[string]model.User, courses map[string]model.Course) error {
	course, ok := courses[spec.CourseSlug]
	if !ok {
		return fmt.Errorf("order %s: course %s not found", spec.Number, spec.CourseSlug)
	}
	// Giá hiệu lực (khuyến mãi nếu có) — cùng cách OrderService.CreateOrder tính subtotal/giá item.
	price := course.EffectivePrice()
	fee := price.Mul(money(demoPlatformFeePercent)).Div(money(100)).Round(2)
	paidAt := daysAgo(spec.PaidDaysAgo)

	order := model.Order{
		UserID: users[spec.StudentEmail].ID, OrderNumber: spec.Number,
		Subtotal: price, TotalAmount: price, Currency: "VND", Status: "completed",
		PaymentMethod: ptr("bank_transfer"), PaymentGateway: ptr("sepay"),
		PaymentTransactionID: ptr("SEPAY" + spec.Number[len("DEMO-ORD-"):]),
		PaidAt:               &paidAt, PlatformFeePercent: money(demoPlatformFeePercent), PlatformFeeAmount: fee,
		CreatedAt: paidAt.Add(-10 * time.Minute),
	}
	if err := s.db.Where("order_number = ?", spec.Number).Attrs(order).FirstOrCreate(&order).Error; err != nil {
		return fmt.Errorf("failed to seed order %s: %w", spec.Number, err)
	}

	item := model.OrderItem{OrderID: order.ID, CourseID: course.ID, Price: price, FinalPrice: price, CreatedAt: order.CreatedAt}
	if err := s.db.Where("order_id = ? AND course_id = ?", order.ID, course.ID).
		Attrs(item).FirstOrCreate(&item).Error; err != nil {
		return fmt.Errorf("failed to seed item of order %s: %w", spec.Number, err)
	}
	return nil
}

// ensureDemoTeacherBank điền thông tin ngân hàng khi hồ sơ còn trống; không ghi đè thông tin giảng viên
// đã tự cập nhật.
func (s *Seeder) ensureDemoTeacherBank(teacher model.User, bank demoBankSpec) error {
	profile := model.TeacherProfile{UserID: teacher.ID, BankName: ptr(bank.Bank),
		BankAccountNumber: ptr(bank.Number), BankAccountName: ptr(bank.Holder)}
	if err := s.db.Where("user_id = ?", teacher.ID).Attrs(profile).FirstOrCreate(&profile).Error; err != nil {
		return fmt.Errorf("failed to load teacher profile %s: %w", teacher.Email, err)
	}
	if err := s.db.Model(&model.TeacherProfile{}).
		Where("id = ? AND (bank_name IS NULL OR bank_name = '')", profile.ID).
		Updates(map[string]interface{}{"bank_name": bank.Bank, "bank_account_number": bank.Number,
			"bank_account_name": bank.Holder}).Error; err != nil {
		return fmt.Errorf("failed to fill bank info for %s: %w", teacher.Email, err)
	}
	return nil
}

func (s *Seeder) upsertDemoPayout(spec demoPayoutSpec, users map[string]model.User) error {
	teacher, ok := users[spec.TeacherEmail]
	if !ok {
		return fmt.Errorf("payout: teacher %s not found", spec.TeacherEmail)
	}
	bank := demoTeacherBanks[spec.TeacherEmail]
	created := daysAgo(spec.CreatedDaysAgo)
	payout := model.InstructorPayout{
		InstructorID: teacher.ID, Amount: money(spec.Amount), Currency: "VND", Status: spec.Status,
		// Snapshot ngân hàng giống WithdrawalService.Create (đổi hồ sơ sau này không làm sai yêu cầu cũ).
		PaymentMethod: ptr("bank_transfer"), BankName: ptr(bank.Bank),
		BankAccountNumber: ptr(bank.Number), BankAccountName: ptr(bank.Holder), Notes: ptr(spec.Note),
	}
	payout.CreatedAt = created
	if spec.ProcessedDaysAgo > 0 {
		payout.ProcessedAt = ptr(daysAgo(spec.ProcessedDaysAgo))
	}
	if spec.RejectionReason != "" {
		payout.RejectionReason = ptr(spec.RejectionReason)
	}
	if spec.TransactionID != "" {
		payout.TransactionID = ptr(spec.TransactionID)
	}
	if err := s.db.Where("instructor_id = ? AND notes = ?", teacher.ID, spec.Note).
		Attrs(payout).FirstOrCreate(&payout).Error; err != nil {
		return fmt.Errorf("failed to seed payout %q of %s: %w", spec.Note, spec.TeacherEmail, err)
	}
	return nil
}
