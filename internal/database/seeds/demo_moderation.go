package seeds

import (
	"fmt"
	"log"

	"github.com/google/uuid"
	"study.com/v1/internal/model"
)

type demoReportSpec struct {
	ReporterEmail string
	TargetType    string // "course" | "user" — chỉ trỏ đối tượng seed chắc chắn có, không dựa vào discussions/reviews
	TargetKey     string // slug khoá học hoặc email user
	Reason        string
	Description   string
	Status        string
	AdminNotes    string
	DaysAgo       int
}

// demoReportSpecs phủ đủ 4 trạng thái của reports.status để trang (admin)/admin/moderation lọc được.
var demoReportSpecs = []demoReportSpec{
	{ReporterEmail: "student2@demo.com", TargetType: "course", TargetKey: "flutter-mobile-development", Reason: "copyright",
		Description: "Video chương \"State & Networking\" có đoạn giống hệt một khoá Flutter trả phí trên nền tảng khác.",
		Status:      "pending", DaysAgo: 1},
	{ReporterEmail: "student1@demo.com", TargetType: "user", TargetKey: "student2@demo.com", Reason: "spam",
		Description: "Tài khoản gửi liên tiếp nhiều tin nhắn quảng cáo nhóm ôn thi bên ngoài vào nhóm học React.",
		Status:      "reviewing", AdminNotes: "Đang đối chiếu lịch sử tin nhắn nhóm.", DaysAgo: 3},
	{ReporterEmail: "student2@demo.com", TargetType: "course", TargetKey: "react-nextjs-tu-co-ban-den-nang-cao", Reason: "inappropriate",
		Description: "Phần mô tả khoá ghi \"cam kết có việc làm sau khoá học\" dễ gây hiểu lầm.",
		Status:      "resolved", AdminNotes: "Đã yêu cầu giảng viên sửa mô tả, bỏ câu cam kết việc làm.", DaysAgo: 10},
	{ReporterEmail: "student1@demo.com", TargetType: "user", TargetKey: "teacher2@demo.com", Reason: "harassment",
		Description: "Giảng viên nhắc hạn nộp bài nhiều lần trong ngày.",
		Status:      "dismissed", AdminNotes: "Đã kiểm tra: tin nhắc hạn nộp bài bình thường, không vi phạm.", DaysAgo: 7},
}

// SeedDemoReports tạo báo cáo nội dung ở các trạng thái pending/reviewing/resolved/dismissed. Báo cáo đã
// xử lý ghi resolved_by = admin demo. Khoá tự nhiên (reporter, reported_type, reported_id) — trùng đúng
// điều kiện chống báo cáo trùng của ReportService.
func (s *Seeder) SeedDemoReports(users map[string]model.User, courses map[string]model.Course) error {
	admin := users["admin@demo.com"]
	for _, spec := range demoReportSpecs {
		targetID, err := demoReportTarget(spec, users, courses)
		if err != nil {
			return err
		}
		report := model.Report{ReporterID: users[spec.ReporterEmail].ID, ReportedType: spec.TargetType,
			ReportedID: targetID, Reason: spec.Reason, Description: ptr(spec.Description), Status: spec.Status,
			CreatedAt: daysAgo(spec.DaysAgo)}
		if spec.AdminNotes != "" {
			report.AdminNotes = ptr(spec.AdminNotes)
		}
		if spec.Status == "resolved" || spec.Status == "dismissed" {
			report.ResolvedBy, report.ResolvedAt = &admin.ID, ptr(daysAgo(spec.DaysAgo-1))
		}
		if err := s.db.Where("reporter_id = ? AND reported_type = ? AND reported_id = ?", report.ReporterID, spec.TargetType, targetID).
			Attrs(report).FirstOrCreate(&report).Error; err != nil {
			return fmt.Errorf("failed to seed report on %s %s: %w", spec.TargetType, spec.TargetKey, err)
		}
	}
	log.Printf("Seeded %d demo reports\n", len(demoReportSpecs))
	return nil
}

func demoReportTarget(spec demoReportSpec, users map[string]model.User, courses map[string]model.Course) (uuid.UUID, error) {
	switch spec.TargetType {
	case "course":
		if c, ok := courses[spec.TargetKey]; ok {
			return c.ID, nil
		}
	case "user":
		if u, ok := users[spec.TargetKey]; ok {
			return u.ID, nil
		}
	}
	return uuid.Nil, fmt.Errorf("report target %s %s not found", spec.TargetType, spec.TargetKey)
}
