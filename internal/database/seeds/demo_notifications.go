package seeds

import (
	"fmt"
	"log"
	"time"

	"study.com/v1/internal/model"
)

// notificationSpec mô tả một thông báo demo. CourseSlug (nếu có) gắn reference_type="course"
// để toast WebSocket phía web dẫn tới /courses/:id — loại tham chiếu web đang hiểu.
type notificationSpec struct {
	Title      string
	Content    string
	Type       string
	CourseSlug string
	HoursAgo   int
	IsRead     bool
}

var demoNotifications = map[string][]notificationSpec{
	"student1@demo.com": {
		{Title: "Chúc mừng! Bạn đã nhận chứng chỉ Git & GitHub", Content: "Chứng chỉ hoàn thành khoá \"Git & GitHub cho người mới bắt đầu\" đã sẵn sàng trong mục Chứng chỉ.", Type: "certificate_earned", CourseSlug: "git-github-cho-nguoi-moi-bat-dau", HoursAgo: 70, IsRead: true},
		{Title: "Mở khoá huy hiệu \"Chuỗi 7 ngày\"", Content: "Bạn đã học liên tục 7 ngày. Giữ vững phong độ nhé!", Type: "achievement", HoursAgo: 50, IsRead: true},
		{Title: "Bài học mới: Data Fetching & Caching", Content: "Giảng viên vừa cập nhật video và tài liệu cho bài \"Data Fetching & Caching\" trong khoá React + Next.js.", Type: "new_lesson", CourseSlug: "react-nextjs-tu-co-ban-den-nang-cao", HoursAgo: 30},
		{Title: "Nhắc lịch làm bài kiểm tra", Content: "Bài kiểm tra chương \"React Fundamentals\" đang chờ bạn. Hoàn thành để nhận thêm 50 điểm.", Type: "quiz_reminder", CourseSlug: "react-nextjs-tu-co-ban-den-nang-cao", HoursAgo: 20},
		{Title: "Bạn vừa nhận 20 điểm thưởng", Content: "Hoàn thành bài \"State & Hooks\" giúp bạn cộng 20 điểm. Tổng điểm hiện tại: 1.850.", Type: "point_earned", HoursAgo: 12},
		{Title: "Thanh toán gói xu thành công", Content: "Bạn đã nạp gói \"Gói Tiêu chuẩn\" (500 xu + 50 xu thưởng). Số xu đã được cộng vào ví.", Type: "payment_success", HoursAgo: 96, IsRead: true},
		{Title: "Đừng để mất chuỗi học tập!", Content: "Hôm nay bạn chưa học bài nào. Chỉ cần 10 phút để giữ chuỗi 12 ngày.", Type: "streak", HoursAgo: 3},
		{Title: "Ưu đãi cuối tuần: giảm 20% khoá Docker", Content: "Dùng mã WELCOME20 khi thanh toán khoá \"Docker & Kubernetes thực chiến\" trước Chủ nhật.", Type: "promotion", CourseSlug: "docker-kubernetes-thuc-chien", HoursAgo: 8},
	},
	"student2@demo.com": {
		{Title: "Chào mừng bạn đến với 40Study", Content: "Tài khoản đã sẵn sàng. Hãy bắt đầu với khoá học bạn đã đăng ký.", Type: "system", HoursAgo: 240, IsRead: true},
		{Title: "Khoá Python cho Khoa học Dữ liệu có cập nhật", Content: "Chương \"NumPy & Pandas\" vừa được bổ sung bài tập thực hành mới.", Type: "course_update", CourseSlug: "python-cho-khoa-hoc-du-lieu", HoursAgo: 60},
		{Title: "Mở khoá huy hiệu \"Bước chân đầu tiên\"", Content: "Bạn đã hoàn thành bài học đầu tiên. Hành trình bắt đầu từ đây!", Type: "achievement", HoursAgo: 48, IsRead: true},
		{Title: "Thanh toán chưa thành công", Content: "Giao dịch nạp xu lúc 21:14 bị ngân hàng từ chối. Bạn có thể thử lại hoặc chọn phương thức khác.", Type: "payment_failed", HoursAgo: 36},
		{Title: "Bạn vừa nhận 10 điểm điểm danh", Content: "Điểm danh hằng ngày giúp bạn cộng 10 điểm. Chuỗi hiện tại: 3 ngày.", Type: "point_earned", HoursAgo: 10},
		{Title: "Nhắc lịch làm bài kiểm tra", Content: "Bài kiểm tra \"Python Cơ bản\" đang chờ bạn làm thử.", Type: "quiz_reminder", CourseSlug: "python-cho-khoa-hoc-du-lieu", HoursAgo: 5},
		{Title: "Voucher chào mừng dành cho bạn", Content: "Mã WELCOME20 giảm 20% cho đơn đầu tiên đã được lưu vào Voucher của tôi.", Type: "promotion", HoursAgo: 2},
	},
	"teacher1@demo.com": {
		{Title: "Khoá học của bạn có đánh giá mới", Content: "Lê Văn C đánh giá 5 sao khoá \"React + Next.js từ cơ bản đến nâng cao\".", Type: "course_update", CourseSlug: "react-nextjs-tu-co-ban-den-nang-cao", HoursAgo: 110, IsRead: true},
		{Title: "Có học viên mới ghi danh", Content: "Khoá \"Flutter Mobile Development\" vừa có thêm học viên mới.", Type: "course_update", CourseSlug: "flutter-mobile-development", HoursAgo: 72, IsRead: true},
		{Title: "Doanh thu tháng này đã được ghi nhận", Content: "Đơn hàng khoá React + Next.js đã thanh toán thành công, doanh thu được cộng vào số dư chờ đối soát.", Type: "payment_success", HoursAgo: 40},
		{Title: "Bảo trì hệ thống đêm thứ Bảy", Content: "Hệ thống bảo trì từ 23:00 đến 01:00. Các buổi livestream nên dời lịch tránh khung giờ này.", Type: "system", HoursAgo: 26},
		{Title: "Nhắc chuẩn bị bài kiểm tra chương mới", Content: "Chương \"Next.js App Router\" chưa có bài kiểm tra. Thêm quiz giúp học viên ôn tập tốt hơn.", Type: "quiz_reminder", CourseSlug: "react-nextjs-tu-co-ban-den-nang-cao", HoursAgo: 14},
		{Title: "Chương trình ưu đãi giảng viên tháng 10", Content: "Tham gia chiến dịch giảm giá để khoá học xuất hiện ở trang chủ trong 2 tuần.", Type: "promotion", HoursAgo: 4},
	},
	"parent1@demo.com": {
		{Title: "Liên kết tài khoản con thành công", Content: "Bạn đã liên kết với tài khoản của Lê Văn C và có thể theo dõi tiến độ học tập.", Type: "system", HoursAgo: 200, IsRead: true},
		{Title: "Con bạn đã nhận chứng chỉ", Content: "Lê Văn C vừa hoàn thành khoá \"Git & GitHub cho người mới bắt đầu\" và nhận chứng chỉ.", Type: "certificate_earned", CourseSlug: "git-github-cho-nguoi-moi-bat-dau", HoursAgo: 70, IsRead: true},
		{Title: "Con bạn mở khoá huy hiệu mới", Content: "Lê Văn C đạt huy hiệu \"Chuỗi 7 ngày\" nhờ học đều đặn cả tuần.", Type: "achievement", HoursAgo: 50},
		{Title: "Tiến độ tuần này của Lê Văn C", Content: "Tuần này con hoàn thành 4 bài học, tổng 95 phút. Khoá React + Next.js đạt khoảng 65%.", Type: "course_update", CourseSlug: "react-nextjs-tu-co-ban-den-nang-cao", HoursAgo: 18},
		{Title: "Con bạn sắp mất chuỗi học tập", Content: "Hôm nay Lê Văn C chưa học bài nào. Một lời nhắc nhỏ có thể giúp con giữ chuỗi 12 ngày.", Type: "streak", HoursAgo: 3},
		{Title: "Ưu đãi gia đình: giảm 50.000đ", Content: "Mã SUMMER50K áp dụng cho đơn từ 300.000đ khi mua khoá học cho con.", Type: "promotion", HoursAgo: 30},
	},
}

// SeedDemoNotifications tạo 6-8 thông báo cho student1, student2, teacher1, parent1 (trang
// (app)/notifications): trộn đã đọc/chưa đọc để unread_count > 0, đủ các notification_type hợp lệ
// theo CHECK của model.Notification. Khoá tự nhiên: (user_id, title).
func (s *Seeder) SeedDemoNotifications(users map[string]model.User, courses map[string]model.Course) error {
	count := 0
	for email, specs := range demoNotifications {
		user, err := demoUser(users, email)
		if err != nil {
			return err
		}
		for _, spec := range specs {
			n := model.Notification{
				UserID:           user.ID,
				Title:            spec.Title,
				Content:          spec.Content,
				NotificationType: spec.Type,
				IsRead:           spec.IsRead,
				CreatedAt:        time.Now().Add(-time.Duration(spec.HoursAgo) * time.Hour),
			}
			if spec.IsRead {
				readAt := n.CreatedAt.Add(time.Hour)
				n.ReadAt = &readAt
			}
			if spec.CourseSlug != "" {
				course, err := demoCourse(courses, spec.CourseSlug)
				if err != nil {
					return err
				}
				n.ReferenceType = ptr("course")
				n.ReferenceID = ptr(course.ID)
			}
			if err := s.db.Where("user_id = ? AND title = ?", user.ID, spec.Title).
				Attrs(n).FirstOrCreate(&n).Error; err != nil {
				return fmt.Errorf("failed to seed notification %q for %s: %w", spec.Title, email, err)
			}
			count++
		}
	}
	log.Printf("Seeded %d demo notifications\n", count)
	return nil
}
