package repository

import (
	"time"

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

// ─── Student (buyer) queries ────────────────────────────────────────────────

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

// ─── Teacher (instructor) queries ───────────────────────────────────────────

// TeacherEarningsRow holds aggregated earnings for a teacher
type TeacherEarningsRow struct {
	TotalEarnings decimal.Decimal
	OrderCount    int64
}

// GetTeacherEarnings returns total earnings from completed orders containing the teacher's courses
func (r *WalletRepository) GetTeacherEarnings(teacherID uuid.UUID) (*TeacherEarningsRow, error) {
	var res TeacherEarningsRow
	err := r.db.Model(&model.OrderItem{}).
		Joins("JOIN orders ON orders.id = order_items.order_id").
		Joins("JOIN courses ON courses.id = order_items.course_id").
		Where("courses.instructor_id = ? AND orders.status = ?", teacherID, "completed").
		Select("COALESCE(SUM(order_items.final_price), 0) AS total_earnings, COUNT(DISTINCT orders.id) AS order_count").
		Scan(&res).Error
	return &res, err
}

// TeacherTransactionRow is a flattened row for teacher transaction history
type TeacherTransactionRow struct {
	OrderID       uuid.UUID
	OrderNumber   string
	OrderStatus   string
	PaymentMethod *string
	CourseName    string
	CourseID      uuid.UUID
	BuyerName     string
	FinalPrice    decimal.Decimal
	Currency      string
	CreatedAt     time.Time
	PaidAt        *time.Time
}

// GetTeacherTransactions returns paginated transaction rows for a teacher's courses.
// txType: "income" (completed), "refund" (refunded), empty = all.
func (r *WalletRepository) GetTeacherTransactions(teacherID uuid.UUID, txType string, page, limit int) ([]TeacherTransactionRow, int64, error) {
	base := r.db.Table("order_items").
		Joins("JOIN orders ON orders.id = order_items.order_id").
		Joins("JOIN courses ON courses.id = order_items.course_id").
		Joins("JOIN users ON users.id = orders.user_id").
		Where("courses.instructor_id = ?", teacherID)

	switch txType {
	case "income":
		base = base.Where("orders.status = ?", "completed")
	case "refund":
		base = base.Where("orders.status = ?", "refunded")
	}

	var total int64
	if err := base.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	var rows []TeacherTransactionRow
	offset := (page - 1) * limit
	err := base.
		Select(`
			orders.id AS order_id,
			orders.order_number,
			orders.status AS order_status,
			orders.payment_method,
			courses.title AS course_name,
			courses.id AS course_id,
			users.user_name AS buyer_name,
			order_items.final_price,
			orders.currency,
			orders.created_at,
			orders.paid_at
		`).
		Order("orders.created_at DESC").
		Offset(offset).Limit(limit).
		Scan(&rows).Error

	return rows, total, err
}

// GetTeacherPayoutTotal returns the sum of completed payouts for a teacher
func (r *WalletRepository) GetTeacherPayoutTotal(teacherID uuid.UUID) (decimal.Decimal, error) {
	var total decimal.Decimal
	err := r.db.Model(&model.InstructorPayout{}).
		Where("instructor_id = ? AND status = ?", teacherID, "completed").
		Select("COALESCE(SUM(amount), 0)").
		Scan(&total).Error
	return total, err
}
