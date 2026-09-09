package repository

import (
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"study.com/v1/internal/model"
)

var (
	ErrOrderNotFound = errors.New("order not found")
	ErrOrderConflict = errors.New("order conflict")
)

// OrderRepository - Repository for Order
type OrderRepository struct {
	db *gorm.DB
}

func NewOrderRepository(db *gorm.DB) *OrderRepository {
	return &OrderRepository{db: db}
}

// Create - Create new order
func (r *OrderRepository) Create(order *model.Order) error {
	return r.db.Create(order).Error
}

// GetByID - Get order by ID
func (r *OrderRepository) GetByID(id uuid.UUID) (*model.Order, error) {
	var order model.Order
	if err := r.db.Preload("Items").Preload("Coupon").First(&order, "id = ?", id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrOrderNotFound
		}
		return nil, err
	}
	return &order, nil
}

// GetByOrderNumber - Get order by order number
func (r *OrderRepository) GetByOrderNumber(orderNumber string) (*model.Order, error) {
	var order model.Order
	if err := r.db.Preload("Items").Preload("Coupon").First(&order, "order_number = ?", orderNumber).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrOrderNotFound
		}
		return nil, err
	}
	return &order, nil
}

// GetByUserID - Get orders by user ID with pagination
func (r *OrderRepository) GetByUserID(userID uuid.UUID, page, limit int, status string) ([]model.Order, int64, error) {
	var orders []model.Order
	var total int64

	query := r.db.Model(&model.Order{}).Where("user_id = ?", userID)

	if status != "" {
		query = query.Where("status = ?", status)
	}

	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	offset := (page - 1) * limit
	if err := query.Preload("Items").Preload("Coupon").
		Order("created_at DESC").
		Offset(offset).Limit(limit).
		Find(&orders).Error; err != nil {
		return nil, 0, err
	}

	return orders, total, nil
}

// GetByUserIDAndStatus - Get orders by user ID and status
func (r *OrderRepository) GetByUserIDAndStatus(userID uuid.UUID, status string) ([]model.Order, error) {
	var orders []model.Order
	if err := r.db.Preload("Items").Where("user_id = ? AND status = ?", userID, status).Find(&orders).Error; err != nil {
		return nil, err
	}
	return orders, nil
}

// UpdateStatus - Update order status
func (r *OrderRepository) UpdateStatus(orderID uuid.UUID, status string) error {
	return r.db.Model(&model.Order{}).Where("id = ?", orderID).Update("status", status).Error
}

// UpdatePaymentCode (item 25, review web vòng 1): TRƯỚC ĐÂY CreatePaymentIntent chỉ gọi
// UpdateStatus("processing") — mã thanh toán (paymentCode) sinh ra chỉ tồn tại trong response
// trả về client, KHÔNG được lưu vào order. CheckAndProcessPayment/GetPaymentStatus sau đó đọc
// order.PaymentTransactionID để lấy lại mã này thì luôn rỗng -> "payment code not found",
// khiến nút "Tôi đã chuyển khoản" và vòng poll đều lỗi vĩnh viễn. Giờ set cả status,
// payment_transaction_id (tạm dùng để lưu payment code lúc đang processing — sẽ bị
// UpdatePaymentInfo ghi đè bằng transaction ID THẬT của ngân hàng khi thanh toán xong, đúng ý
// nghĩa cột này sau khi hoàn tất) và payment_code_expired_at trong CÙNG một UPDATE.
func (r *OrderRepository) UpdatePaymentCode(orderID uuid.UUID, paymentCode string, expiredAt time.Time) error {
	updates := map[string]interface{}{
		"status":                  "processing",
		"payment_transaction_id":  paymentCode,
		"payment_code_expired_at": expiredAt,
	}
	return r.db.Model(&model.Order{}).Where("id = ?", orderID).Updates(updates).Error
}

// UpdatePaymentInfo - Update payment information
func (r *OrderRepository) UpdatePaymentInfo(orderID uuid.UUID, paymentMethod, paymentGateway, transactionID string, paidAt time.Time) error {
	updates := map[string]interface{}{
		"payment_method":         paymentMethod,
		"payment_gateway":        paymentGateway,
		"payment_transaction_id": transactionID,
		"paid_at":                paidAt,
		"status":                 "completed",
	}
	return r.db.Model(&model.Order{}).Where("id = ?", orderID).Updates(updates).Error
}

// Update - Update order
func (r *OrderRepository) Update(order *model.Order) error {
	return r.db.Save(order).Error
}

// Delete - Delete order (soft delete)
func (r *OrderRepository) Delete(id uuid.UUID) error {
	return r.db.Delete(&model.Order{}, "id = ?", id).Error
}

// WithTransaction - Execute within transaction
func (r *OrderRepository) WithTransaction(fn func(repo *OrderRepository) error) error {
	return r.db.Transaction(func(tx *gorm.DB) error {
		txRepo := &OrderRepository{db: tx}
		return fn(txRepo)
	})
}

// TxDB (H2-06, review vòng 3): trả về *gorm.DB gốc của repo — bên trong closure của
// WithTransaction, r.db chính là *gorm.DB của transaction (không phải connection gốc). Dùng để
// dựng các repo khác (OrderItem/OrderStatusHistory/Enrollment/Course) THAM GIA CÙNG transaction
// này thay vì chạy trên connection gốc sau khi transaction đã commit — đóng lỗ hổng "chỉ có
// bảng orders được bọc transaction, các bảng còn lại (order_items, history, enrollment,
// voucher used_count) chạy rời rạc" đã nêu trong báo cáo review vòng 3 (H2-06).
func (r *OrderRepository) TxDB() *gorm.DB {
	return r.db
}

// RecordBankTransactionUsage (M-06, audit 260909 vòng 2): ghi nhận 1 bank_transaction_id đã
// được dùng để xác nhận thanh toán — gọi bên trong closure của WithTransaction (r.db lúc đó
// là *gorm.DB của transaction, không phải connection gốc) để việc chống-replay và việc chuyển
// đơn sang "completed" thành công/thất bại CÙNG NHAU (unique constraint vi phạm -> insert lỗi
// -> transaction rollback -> đơn KHÔNG bị đánh dấu completed lần 2).
func (r *OrderRepository) RecordBankTransactionUsage(usage *model.BankTransactionUsage) error {
	return r.db.Create(usage).Error
}

// GetForUpdate - Get order with row lock for update
func (r *OrderRepository) GetForUpdate(tx *gorm.DB, id uuid.UUID) (*model.Order, error) {
	var order model.Order
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
		First(&order, "id = ?", id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrOrderNotFound
		}
		return nil, err
	}
	return &order, nil
}

// UpdateStatusWithTx - Update order status within transaction
func (r *OrderRepository) UpdateStatusWithTx(tx *gorm.DB, orderID uuid.UUID, status string) error {
	return tx.Model(&model.Order{}).Where("id = ?", orderID).Update("status", status).Error
}

// CheckOrderNumberExists - Check if order number exists
func (r *OrderRepository) CheckOrderNumberExists(orderNumber string) (bool, error) {
	var count int64
	if err := r.db.Model(&model.Order{}).Where("order_number = ?", orderNumber).Count(&count).Error; err != nil {
		return false, err
	}
	return count > 0, nil
}

// GetPendingOrders - Get all pending orders (for cleanup)
func (r *OrderRepository) GetPendingOrders(expiredBefore time.Time) ([]model.Order, error) {
	var orders []model.Order
	if err := r.db.Where("status = ? AND created_at < ?", "pending", expiredBefore).Find(&orders).Error; err != nil {
		return nil, err
	}
	return orders, nil
}

// GetExpiredHeldOrdersForUser (H3-01b, review vòng 4): trả về các đơn "pending"/"processing"
// CỦA MỘT USER đã quá hạn — payment_code_expired_at đã qua (đơn đã tạo payment intent), HOẶC
// (chưa có payment_code_expired_at NHƯNG created_at đã quá defaultTTL — đơn "pending" bị bỏ rơi
// ngay từ bước tạo đơn, chưa từng bấm tạo payment intent). Dùng cho lazy-sweep ngay lúc
// CreateOrder (xem OrderService.sweepExpiredHeldOrders) — không cần cron/worker riêng: mỗi lần
// user tạo đơn mới là một cơ hội dọn các đơn cũ CHÍNH HỌ đã bỏ rơi, trả lại used_count voucher
// đã reserve trước khi tính usage_per_user cho đơn mới.
func (r *OrderRepository) GetExpiredHeldOrdersForUser(userID uuid.UUID, defaultTTL time.Duration) ([]model.Order, error) {
	var orders []model.Order
	now := time.Now()
	err := r.db.
		Where("user_id = ? AND status IN ('pending','processing')", userID).
		Where("(payment_code_expired_at IS NOT NULL AND payment_code_expired_at < ?) OR (payment_code_expired_at IS NULL AND created_at < ?)",
			now, now.Add(-defaultTTL)).
		Find(&orders).Error
	if err != nil {
		return nil, err
	}
	return orders, nil
}

// CalculateUserSpent - Calculate total amount spent by user
func (r *OrderRepository) CalculateUserSpent(userID uuid.UUID) (decimal.Decimal, error) {
	var total decimal.Decimal
	err := r.db.Model(&model.Order{}).
		Where("user_id = ? AND status = ?", userID, "completed").
		Select("COALESCE(SUM(total_amount), 0)").
		Scan(&total).Error
	return total, err
}
