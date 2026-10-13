package seeds

// QA vòng 2 PG-M4 + PG-L4 trên Postgres THẬT (schema tạm):
//   - PG-M4: thông báo khuyến mãi của phụ huynh không được hứa "mua khoá học cho con" (web không có luồng đó).
//   - PG-L4: số % trong thông báo tiến độ tuần của phụ huynh phải LẤY TỪ enrollment của con (trang con tính từ
//     enrollment đó), không phải số gõ cứng; seed chạy lại phải sửa cả dòng cũ đã seed với nội dung lệch.

import (
	"fmt"
	"strings"
	"testing"

	"study.com/v1/internal/database"
	"study.com/v1/internal/model"
	"study.com/v1/internal/testutil/pgtest"
)

const (
	parentPromoTitle  = "Ưu đãi gia đình: giảm 50.000đ"
	parentDigestTitle = "Tiến độ tuần này của Lê Văn C"
)

func TestSeedDemoNotifications_Postgres_ParentCopyKhopDuLieu(t *testing.T) {
	db := pgtest.IsolatedSchema(t, database.Migrate)
	s := NewSeeder(db)
	for _, name := range []string{"SYSTEM_ADMIN", "TEACHER", "STUDENT", "PARENT"} {
		if err := db.Create(&model.SystemRole{Name: name, Status: "active"}).Error; err != nil {
			t.Fatalf("tạo role %s: %v", name, err)
		}
	}
	users, err := s.SeedDemoUsers()
	if err != nil {
		t.Fatalf("seed users: %v", err)
	}
	courses := seedDemoCoursesOnce(t, s, users)
	if err := s.SeedDemoEnrollments(users, courses); err != nil {
		t.Fatalf("seed enrollments: %v", err)
	}
	if err := s.SeedDemoVouchers(); err != nil {
		t.Fatalf("seed vouchers: %v", err)
	}

	var enr model.Enrollment
	if err := db.Where("user_id = ? AND course_id = ?", users["student1@demo.com"].ID,
		courses["react-nextjs-tu-co-ban-den-nang-cao"].ID).First(&enr).Error; err != nil {
		t.Fatalf("đọc enrollment React của student1: %v", err)
	}
	wantPct := fmt.Sprintf("%s%%", enr.ProgressPercent.Round(0).String())

	parentContent := func(title string) string {
		var n model.Notification
		if err := db.Where("user_id = ? AND title = ?", users["parent1@demo.com"].ID, title).First(&n).Error; err != nil {
			t.Fatalf("đọc thông báo %q của parent1: %v", title, err)
		}
		return n.Content
	}

	for run := 1; run <= 2; run++ {
		if err := s.SeedDemoEngagement(users, courses); err != nil {
			t.Fatalf("lần %d: SeedDemoEngagement: %v", run, err)
		}
		if got := parentContent(parentDigestTitle); !strings.Contains(got, "đạt khoảng "+wantPct) {
			t.Errorf("lần %d: thông báo tiến độ %q không chứa %q lấy từ enrollment (progress=%s)", run, got, wantPct, enr.ProgressPercent)
		}
		if got := parentContent(parentPromoTitle); strings.Contains(got, "cho con") {
			t.Errorf("lần %d: khuyến mãi phụ huynh còn hứa mua khoá học cho con: %q", run, got)
		}

		// Dòng cũ đã seed với nội dung lệch (DB đang chạy từ trước bản sửa) phải được seed chạy lại sửa đúng.
		stale := map[string]string{
			parentDigestTitle: "Tuần này con hoàn thành 4 bài học, tổng 95 phút. Khoá React + Next.js đạt khoảng 99%.",
			parentPromoTitle:  "Mã SUMMER50K áp dụng cho đơn từ 300.000đ khi mua khoá học cho con.",
		}
		for title, content := range stale {
			if err := db.Model(&model.Notification{}).Where("user_id = ? AND title = ?", users["parent1@demo.com"].ID, title).
				Update("content", content).Error; err != nil {
				t.Fatalf("làm lệch %q: %v", title, err)
			}
		}
	}
}
