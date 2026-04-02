package repository

import (
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"gorm.io/gorm"
	"study.com/v1/internal/model"
)

// WalletRepository aggregates wallet data from the orders table
type WalletRepository struct {
	db *gorm.DB
}

func NewWalletRepository(db *gorm.DB) *WalletRepository {
	return &WalletRepository{db: db}
}

// GetTotalSpent returns total amount spent by the user across completed orders
func (r *WalletRepository) GetTotalSpent(userID uuid.UUID) (decimal.Decimal, int64, error) {
	type result struct {
		TotalSpent decimal.Decimal
		Count      int64
	}
	var res result
	err := r.db.Model(&model.Order{}).
		Where("user_id = ? AND status = ?", userID, "completed").
		Select("COALESCE(SUM(total_amount), 0) AS total_spent, COUNT(*) AS count").
		Scan(&res).Error
	return res.TotalSpent, res.Count, err
}

// GetTransactions returns paginated orders for a user, optionally filtered by transaction type.
// type "expense" → completed orders (money out).
// type "income" → refunded orders (money back) – treated as income for the user.
// empty type → all orders.
func (r *WalletRepository) GetTransactions(userID uuid.UUID, txType string, page, limit int) ([]model.Order, int64, error) {
	var orders []model.Order
	var total int64

	query := r.db.Model(&model.Order{}).Where("user_id = ?", userID)

	switch txType {
	case "expense":
		query = query.Where("status = ?", "completed")
	case "income":
		query = query.Where("status = ?", "refunded")
	}

	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	offset := (page - 1) * limit
	err := query.
		Order("created_at DESC").
		Offset(offset).Limit(limit).
		Find(&orders).Error

	return orders, total, err
}
