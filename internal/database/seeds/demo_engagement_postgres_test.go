package seeds

// Lane C seed demo — tương tác/thành tích/xu/chứng chỉ/voucher/đánh giá trên Postgres THẬT, trong
// schema tạm (pgtest.IsolatedSchema) bị DROP khi test xong. Không có Postgres: Skip ở local.

import (
	"strings"
	"testing"

	"study.com/v1/internal/database"
	"study.com/v1/internal/model"
	"study.com/v1/internal/testutil/pgtest"
)

// TestSeedDemoEngagement_Postgres_DuSoLieuVaIdempotent: seed đủ bản ghi cho 6 trang engagement, ví
// xu khớp sổ giao dịch, chứng chỉ chỉ cho enrollment đã hoàn thành, và chạy lần 2 không tạo trùng.
func TestSeedDemoEngagement_Postgres_DuSoLieuVaIdempotent(t *testing.T) {
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

	want := []struct {
		model interface{}
		n     int64
	}{
		{&model.User{}, 9},
		{&model.Notification{}, 27},
		{&model.Achievement{}, 9},
		{&model.UserAchievement{}, 14},
		{&model.UserPoint{}, 5},
		{&model.UserStreak{}, 5},
		{&model.LeaderboardEntry{}, 10},
		{&model.Reward{}, 4},
		{&model.CoinPackage{}, 4},
		{&model.UserCoinWallet{}, 2},
		{&model.CoinTransaction{}, 10},
		{&model.CoinPurchase{}, 3},
		{&model.Certificate{}, 1},
		{&model.Voucher{}, 4},
		{&model.UserVoucher{}, 6},
		{&model.Review{}, 4},
		{&model.Wishlist{}, 4},
		{&model.UserNote{}, 4},
	}

	for run := 1; run <= 2; run++ {
		if err := s.SeedDemoEngagement(users, courses); err != nil {
			t.Fatalf("lần %d: SeedDemoEngagement: %v", run, err)
		}
		for _, w := range want {
			if got := countRows(t, db, w.model); got != w.n {
				t.Errorf("lần %d: %T có %d dòng, muốn %d", run, w.model, got, w.n)
			}
		}

		var unread int64
		db.Model(&model.Notification{}).Where("user_id = ? AND is_read = false", users["student1@demo.com"].ID).Count(&unread)
		if unread == 0 {
			t.Errorf("lần %d: student1 không có thông báo chưa đọc", run)
		}

		for email, balance := range map[string]int64{"student1@demo.com": 590, "student2@demo.com": 100} {
			var wallet model.UserCoinWallet
			if err := db.Where("user_id = ?", users[email].ID).First(&wallet).Error; err != nil {
				t.Fatalf("lần %d: đọc ví %s: %v", run, email, err)
			}
			if wallet.Balance != balance {
				t.Errorf("lần %d: ví %s balance=%d, muốn %d (tổng sổ giao dịch)", run, email, wallet.Balance, balance)
			}
		}

		var cert model.Certificate
		if err := db.First(&cert).Error; err != nil {
			t.Fatalf("lần %d: đọc chứng chỉ: %v", run, err)
		}
		if cert.CourseID != courses["git-github-cho-nguoi-moi-bat-dau"].ID || !strings.HasPrefix(cert.CertificateNumber, "CERT-") {
			t.Errorf("lần %d: chứng chỉ sai khoá hoặc số %q", run, cert.CertificateNumber)
		}

		var git model.Course
		db.First(&git, "id = ?", courses["git-github-cho-nguoi-moi-bat-dau"].ID)
		if git.TotalReviews != 1 || git.AverageRating.IntPart() != 5 {
			t.Errorf("lần %d: khoá Git total_reviews=%d avg=%s, muốn 1/5", run, git.TotalReviews, git.AverageRating)
		}
	}
}
