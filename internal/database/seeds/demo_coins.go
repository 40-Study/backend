package seeds

import (
	"fmt"
	"time"

	"github.com/google/uuid"
	"gorm.io/datatypes"
	"study.com/v1/internal/model"
)

var demoCoinPackages = []model.CoinPackage{
	{Name: "Gói Khởi đầu", Description: ptr("100 xu để thử dùng gợi ý bài tập."), CoinAmount: 100, Price: money(20000), SortOrder: 1},
	{Name: "Gói Tiêu chuẩn", Description: ptr("500 xu tặng thêm 50 xu, lựa chọn phổ biến nhất."), CoinAmount: 500, BonusAmount: 50, Price: money(95000), DiscountPercent: 5, IsFeatured: true, SortOrder: 2},
	{Name: "Gói Nâng cao", Description: ptr("1.000 xu tặng thêm 150 xu cho người học đều đặn."), CoinAmount: 1000, BonusAmount: 150, Price: money(180000), DiscountPercent: 10, SortOrder: 3},
	{Name: "Gói Siêu tiết kiệm", Description: ptr("2.500 xu tặng thêm 500 xu, giá tốt nhất."), CoinAmount: 2500, BonusAmount: 500, Price: money(425000), DiscountPercent: 15, SortOrder: 4},
}

// demoCoinTx là một giao dịch xu demo. Package (nếu có) sinh kèm một coin_purchase COMPLETED.
// Amount âm = tiêu xu (cùng quy ước coin_service ghi SPEND/gift).
type demoCoinTx struct {
	Type        model.CoinTransactionType
	Amount      int64
	Description string
	DaysAgo     int
	Package     string
}

var demoCoinLedger = map[string][]demoCoinTx{
	"student1@demo.com": {
		{Type: model.CoinTxEarnPurchase, Amount: 550, Description: "Nạp Gói Tiêu chuẩn (500 + 50 xu thưởng)", DaysAgo: 4, Package: "Gói Tiêu chuẩn"},
		{Type: model.CoinTxEarnLessonComplete, Amount: 20, Description: "Hoàn thành bài \"State & Hooks\"", DaysAgo: 3},
		{Type: model.CoinTxEarnStreakBonus, Amount: 50, Description: "Thưởng chuỗi học 10 ngày", DaysAgo: 2},
		{Type: model.CoinTxSpendHint, Amount: -30, Description: "Mở gợi ý bài tập Todo App", DaysAgo: 2},
		{Type: model.CoinTxEarnAchievement, Amount: 100, Description: "Mở khoá huy hiệu \"Chuỗi 7 ngày\"", DaysAgo: 1},
		{Type: model.CoinTxSpendStreakFreeze, Amount: -100, Description: "Mua 1 lượt đóng băng chuỗi học", DaysAgo: 1},
	},
	"student2@demo.com": {
		{Type: model.CoinTxEarnDailyLogin, Amount: 10, Description: "Điểm danh ngày đầu tiên", DaysAgo: 6},
		{Type: model.CoinTxEarnPurchase, Amount: 100, Description: "Nạp Gói Khởi đầu (100 xu)", DaysAgo: 5, Package: "Gói Khởi đầu"},
		{Type: model.CoinTxEarnLessonComplete, Amount: 20, Description: "Hoàn thành bài \"Biến, kiểu dữ liệu và toán tử\"", DaysAgo: 3},
		{Type: model.CoinTxSpendHint, Amount: -30, Description: "Mở gợi ý bài tập NumPy", DaysAgo: 1},
	},
}

// SeedDemoCoins seed gói xu (coin_packages), ví + lịch sử giao dịch nạp/tiêu cho student1/student2
// và lịch sử mua gói — dữ liệu cho trang (app)/coins. student2 có thêm 1 lần mua FAILED khớp thông
// báo payment_failed. Số dư ví luôn đồng bộ lại theo tổng sổ giao dịch (không lệch khi chạy lại).
func (s *Seeder) SeedDemoCoins(users map[string]model.User) error {
	packages := make(map[string]model.CoinPackage, len(demoCoinPackages))
	for _, p := range demoCoinPackages {
		record := p
		record.Currency = "VND"
		record.IsActive = true
		if err := s.db.Where("name = ?", p.Name).Attrs(record).FirstOrCreate(&record).Error; err != nil {
			return fmt.Errorf("failed to seed coin package %s: %w", p.Name, err)
		}
		packages[p.Name] = record
	}

	for email, ledger := range demoCoinLedger {
		user, err := demoUser(users, email)
		if err != nil {
			return err
		}
		if err := s.seedCoinWalletLedger(user, ledger, packages); err != nil {
			return err
		}
	}

	student2, err := demoUser(users, "student2@demo.com")
	if err != nil {
		return err
	}
	std := packages["Gói Tiêu chuẩn"]
	failed := model.CoinPurchase{
		UserID: student2.ID, PackageID: &std.ID, CoinAmount: std.CoinAmount, BonusAmount: std.BonusAmount,
		Price: std.Price, Currency: "VND", Status: model.CoinPurchaseFailed,
		PaymentMethod: ptr("bank_transfer"), PaymentReference: ptr("DEMO-COIN-student2-failed"),
		CreatedAt: time.Now().Add(-36 * time.Hour),
	}
	if err := s.db.Where("user_id = ? AND payment_reference = ?", student2.ID, *failed.PaymentReference).
		Attrs(failed).FirstOrCreate(&failed).Error; err != nil {
		return fmt.Errorf("failed to seed failed coin purchase: %w", err)
	}
	return nil
}

// seedCoinWalletLedger tạo ví, ghi giao dịch theo thứ tự thời gian (balance_after cộng dồn) rồi
// đồng bộ balance/total_earned/total_spent của ví bằng tổng thật trong coin_transactions.
func (s *Seeder) seedCoinWalletLedger(user model.User, ledger []demoCoinTx, packages map[string]model.CoinPackage) error {
	wallet := model.UserCoinWallet{UserID: user.ID}
	if err := s.db.Where("user_id = ?", user.ID).FirstOrCreate(&wallet).Error; err != nil {
		return fmt.Errorf("failed to seed coin wallet for %s: %w", user.Email, err)
	}

	var running int64
	for i, item := range ledger {
		running += item.Amount
		tx := model.CoinTransaction{
			WalletID: wallet.ID, UserID: user.ID, Type: item.Type, Amount: item.Amount,
			BalanceAfter: running, Description: ptr(item.Description),
			Metadata:  datatypes.JSON(`{"source":"demo_seed"}`),
			CreatedAt: daysAgo(item.DaysAgo).Add(time.Duration(i) * time.Minute),
		}
		if err := s.db.Where("user_id = ? AND type = ? AND description = ?", user.ID, item.Type, item.Description).
			Attrs(tx).FirstOrCreate(&tx).Error; err != nil {
			return fmt.Errorf("failed to seed coin transaction %q for %s: %w", item.Description, user.Email, err)
		}
		if item.Package != "" {
			if err := s.seedCompletedCoinPurchase(user, packages[item.Package], tx, i); err != nil {
				return err
			}
		}
	}

	var sums struct {
		Balance int64
		Earned  int64
		Spent   int64
	}
	if err := s.db.Model(&model.CoinTransaction{}).Where("wallet_id = ?", wallet.ID).
		Select("COALESCE(SUM(amount),0) AS balance, " +
			"COALESCE(SUM(CASE WHEN amount > 0 THEN amount END),0) AS earned, " +
			"COALESCE(-SUM(CASE WHEN amount < 0 THEN amount END),0) AS spent").
		Scan(&sums).Error; err != nil {
		return fmt.Errorf("failed to sum coin ledger for %s: %w", user.Email, err)
	}
	if err := s.db.Model(&model.UserCoinWallet{}).Where("id = ?", wallet.ID).
		UpdateColumns(map[string]interface{}{"balance": sums.Balance, "total_earned": sums.Earned, "total_spent": sums.Spent}).Error; err != nil {
		return fmt.Errorf("failed to sync coin wallet for %s: %w", user.Email, err)
	}
	return nil
}

// seedCompletedCoinPurchase ghi lần mua gói COMPLETED trỏ tới giao dịch nạp tương ứng.
func (s *Seeder) seedCompletedCoinPurchase(user model.User, pkg model.CoinPackage, tx model.CoinTransaction, idx int) error {
	if pkg.ID == uuid.Nil {
		return fmt.Errorf("coin package for purchase of %s not seeded", user.Email)
	}
	ref := fmt.Sprintf("DEMO-COIN-%s-%d", user.UserName, idx+1)
	completed := tx.CreatedAt
	purchase := model.CoinPurchase{
		UserID: user.ID, PackageID: &pkg.ID, CoinAmount: pkg.CoinAmount, BonusAmount: pkg.BonusAmount,
		Price: pkg.Price, Currency: "VND", Status: model.CoinPurchaseCompleted,
		PaymentMethod: ptr("bank_transfer"), PaymentReference: &ref, TransactionID: &tx.ID,
		CompletedAt: &completed, CreatedAt: tx.CreatedAt.Add(-2 * time.Minute),
	}
	if err := s.db.Where("user_id = ? AND payment_reference = ?", user.ID, ref).
		Attrs(purchase).FirstOrCreate(&purchase).Error; err != nil {
		return fmt.Errorf("failed to seed coin purchase %s: %w", ref, err)
	}
	return nil
}
