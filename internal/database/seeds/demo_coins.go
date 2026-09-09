package seeds

import (
	"fmt"
	"log"

	"github.com/google/uuid"
	"study.com/v1/internal/model"
)

// demoCoinPackages là catalog gói xu bán trên trang /coins.
var demoCoinPackages = []model.CoinPackage{
	{Name: "Gói Khởi Động", CoinAmount: 100, BonusAmount: 0, Price: 20000,
		Description: ptr("100 xu — dùng thử các tính năng gợi ý và mở khoá bài học."), SortOrder: 1},
	{Name: "Gói Phổ Thông", CoinAmount: 500, BonusAmount: 50, Price: 90000, DiscountPercent: 10,
		Description: ptr("500 xu + tặng 50 xu. Lựa chọn phổ biến nhất."), IsFeatured: true, SortOrder: 2},
	{Name: "Gói Chăm Chỉ", CoinAmount: 1200, BonusAmount: 200, Price: 200000, DiscountPercent: 15,
		Description: ptr("1.200 xu + tặng 200 xu, đủ dùng cả học kỳ."), SortOrder: 3},
	{Name: "Gói Học Kỳ", CoinAmount: 3000, BonusAmount: 700, Price: 450000, DiscountPercent: 20,
		Description: ptr("3.000 xu + tặng 700 xu — tiết kiệm nhất cho người học dài hạn."), SortOrder: 4},
}

// demoCoinTxSpec mô tả một giao dịch xu trong lịch sử ví.
type demoCoinTxSpec struct {
	Type        model.CoinTransactionType
	Amount      int64 // dương = cộng, âm = trừ
	Description string
	DaysAgo     int
}

// demoWalletSpec mô tả ví xu demo của một học viên kèm lịch sử giao dịch.
type demoWalletSpec struct {
	StudentEmail string
	Transactions []demoCoinTxSpec
}

var demoWallets = []demoWalletSpec{
	{
		StudentEmail: "student1@demo.com",
		Transactions: []demoCoinTxSpec{
			{Type: model.CoinTxEarnDailyLogin, Amount: 10, Description: "Điểm danh hằng ngày", DaysAgo: 14},
			{Type: model.CoinTxEarnLessonComplete, Amount: 25, Description: "Hoàn thành bài: Giới thiệu React", DaysAgo: 12},
			{Type: model.CoinTxEarnQuizPass, Amount: 50, Description: "Đạt 90% bài kiểm tra chương 1", DaysAgo: 10},
			{Type: model.CoinTxEarnStreakBonus, Amount: 100, Description: "Thưởng chuỗi 7 ngày học liên tục", DaysAgo: 7},
			{Type: model.CoinTxSpendHint, Amount: -20, Description: "Mở gợi ý bài tập thuật toán", DaysAgo: 5},
			{Type: model.CoinTxEarnAchievement, Amount: 200, Description: "Đạt thành tựu: Hoàn thành khoá học đầu tiên", DaysAgo: 3},
			{Type: model.CoinTxSpendStreakFreeze, Amount: -50, Description: "Dùng băng bảo vệ chuỗi học", DaysAgo: 1},
		},
	},
	{
		StudentEmail: "student2@demo.com",
		Transactions: []demoCoinTxSpec{
			{Type: model.CoinTxEarnDailyLogin, Amount: 10, Description: "Điểm danh hằng ngày", DaysAgo: 6},
			{Type: model.CoinTxEarnLessonComplete, Amount: 25, Description: "Hoàn thành bài: Làm quen Python", DaysAgo: 4},
			{Type: model.CoinTxEarnDailyLogin, Amount: 10, Description: "Điểm danh hằng ngày", DaysAgo: 1},
		},
	},
}

// SeedDemoCoins tạo catalog gói xu, ví xu và lịch sử giao dịch demo.
// Idempotent: gói xu khoá theo tên, ví khoá theo user_id; giao dịch chỉ sinh
// khi ví còn trống để tránh nhân đôi lịch sử qua mỗi lần seed.
func (s *Seeder) SeedDemoCoins(users map[string]model.User) error {
	log.Println("Seeding demo coins...")

	for i := range demoCoinPackages {
		pkg := demoCoinPackages[i]
		if err := s.db.Where("name = ?", pkg.Name).
			Attrs(pkg).
			FirstOrCreate(&pkg).Error; err != nil {
			return fmt.Errorf("failed to seed coin package %s: %w", pkg.Name, err)
		}
	}

	for _, spec := range demoWallets {
		student, ok := users[spec.StudentEmail]
		if !ok {
			return fmt.Errorf("coin wallet owner %s not found", spec.StudentEmail)
		}
		if err := s.seedCoinWallet(student.ID, spec.Transactions); err != nil {
			return err
		}
	}

	log.Printf("Seeded %d coin packages, %d wallets\n", len(demoCoinPackages), len(demoWallets))
	return nil
}

// seedCoinWallet tạo ví và phát lại lịch sử giao dịch theo thứ tự thời gian,
// tính balance_after tăng dần rồi chốt số dư cuối vào ví.
func (s *Seeder) seedCoinWallet(userID uuid.UUID, txs []demoCoinTxSpec) error {
	wallet := model.UserCoinWallet{UserID: userID}
	if err := s.db.Where("user_id = ?", userID).
		Attrs(wallet).
		FirstOrCreate(&wallet).Error; err != nil {
		return fmt.Errorf("failed to seed coin wallet: %w", err)
	}

	var existing int64
	if err := s.db.Model(&model.CoinTransaction{}).
		Where("wallet_id = ?", wallet.ID).
		Count(&existing).Error; err != nil {
		return fmt.Errorf("failed to count coin transactions: %w", err)
	}
	if existing > 0 {
		return nil
	}

	var balance, earned, spent int64
	for _, tx := range txs {
		balance += tx.Amount
		if tx.Amount >= 0 {
			earned += tx.Amount
		} else {
			spent += -tx.Amount
		}

		record := model.CoinTransaction{
			WalletID:     wallet.ID,
			UserID:       userID,
			Type:         tx.Type,
			Amount:       tx.Amount,
			BalanceAfter: balance,
			Description:  ptr(tx.Description),
			CreatedAt:    daysAgo(tx.DaysAgo),
		}
		if err := s.db.Create(&record).Error; err != nil {
			return fmt.Errorf("failed to seed coin transaction: %w", err)
		}
	}

	if err := s.db.Model(&model.UserCoinWallet{}).
		Where("id = ?", wallet.ID).
		Updates(map[string]any{
			"balance":      balance,
			"total_earned": earned,
			"total_spent":  spent,
		}).Error; err != nil {
		return fmt.Errorf("failed to update coin wallet balance: %w", err)
	}
	return nil
}
