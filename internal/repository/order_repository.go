package repository

import (
	"errors"
	"log"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"study.com/v1/internal/model"
)

var (
	ErrOrderNotFound = errors.New("order not found")
	// ErrOrderConflict (M-02, review vòng 5): trả về khi UpdatePaymentInfo chạy UPDATE CÓ ĐIỀU
	// KIỆN (WHERE status IN ('pending','processing')) nhưng RowsAffected==0 — nghĩa là đơn đã bị
	// đổi trạng thái bởi một luồng KHÁC (thường gặp nhất: sweepExpiredHeldOrders lazy-sweep của
	// một request tạo-đơn-mới khác đẩy đơn này sang "expired" + release voucher, đúng lúc nhánh
	// thanh toán của CheckAndProcessPayment cũng đang xử lý CÙNG đơn) giữa lúc đọc order.Status
	// (guard "processing" ở đầu CheckAndProcessPayment) và lúc UPDATE thật thực thi. Trước vòng
	// 5, UpdatePaymentInfo UPDATE vô điều kiện — nếu thắng race này, đơn bị ép "completed" dù
	// vừa được sweep sang "expired" + đã release used_count, khiến used_count hụt 1 so với thực
	// tế VÀ FromStatus ghi trong history sai (ghi "processing" dù DB lúc UPDATE thật đã là
	// "expired"). Field cũ TRƯỚC ĐÂY không có call site nào (dead), giờ dùng lại đúng mục đích.
	ErrOrderConflict = errors.New("order conflict")
)

// paymentCodeMaxLen — độ rộng cột orders.payment_code (varchar(64), xem model.Order.PaymentCode). Giữ khớp với tag
// gorm đó: đổi một nơi mà không đổi nơi kia thì UpdatePaymentInfo lại chép mã không vừa cột.
const paymentCodeMaxLen = 64

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
	if err := r.db.Preload("Items.Course").Preload("Coupon").First(&order, "id = ?", id).Error; err != nil {
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
	if err := r.db.Preload("Items.Course").Preload("Coupon").First(&order, "order_number = ?", orderNumber).Error; err != nil {
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
	if err := query.Preload("Items.Course").Preload("Coupon").
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
	if err := r.db.Preload("Items.Course").Where("user_id = ? AND status = ?", userID, status).Find(&orders).Error; err != nil {
		return nil, err
	}
	return orders, nil
}

// UpdatePaymentCode (item 25, review web vòng 1): TRƯỚC ĐÂY CreatePaymentIntent chỉ gọi
// UpdateStatus("processing") — mã thanh toán (paymentCode) sinh ra chỉ tồn tại trong response
// trả về client, KHÔNG được lưu vào order. CheckAndProcessPayment/GetPaymentStatus sau đó đọc
// order.PaymentTransactionID để lấy lại mã này thì luôn rỗng -> "payment code not found",
// khiến nút "Tôi đã chuyển khoản" và vòng poll đều lỗi vĩnh viễn. Giờ set cả status,
// payment_transaction_id (tạm dùng để lưu payment code lúc đang processing — sẽ bị
// UpdatePaymentInfo ghi đè bằng transaction ID THẬT của ngân hàng khi thanh toán xong, đúng ý
// nghĩa cột này sau khi hoàn tất) và payment_code_expired_at trong CÙNG một UPDATE.
// buildUpdatePaymentCodeQuery (B-03, review vòng 5) — tách phần XÂY câu UPDATE có điều kiện ra
// khỏi phần map RowsAffected -> error, cùng mẫu buildUpdatePaymentInfoQuery/
// buildReserveVoucherUsageQuery, để test DryRun gọi được ĐÚNG hàm sản xuất thật.
func (r *OrderRepository) buildUpdatePaymentCodeQuery(orderID uuid.UUID, paymentCode string, expiredAt time.Time) *gorm.DB {
	updates := map[string]interface{}{
		"status":                  "processing",
		"payment_transaction_id":  paymentCode,
		"payment_code":            paymentCode, // bản bất biến: payment_transaction_id sẽ bị mã giao dịch ghi đè
		"payment_code_expired_at": expiredAt,
	}
	// B-03 (review vòng 5): UPDATE CÓ ĐIỀU KIỆN thay vì vô điều kiện như trước — 2 request tạo
	// payment intent đồng thời cho CÙNG 1 đơn (double-click, 2 tab) trước đây có thể cùng đọc
	// order.Status="pending" (guard đọc TRƯỚC transaction ở CreatePaymentIntent), rồi cả hai
	// cùng UPDATE thành công, sinh 2 mã thanh toán khác nhau cho cùng 1 đơn — mã sau ghi đè mã
	// trước, người dùng nhìn thấy mã KHÁC với mã họ vừa được cấp ở request đầu. Cho phép UPDATE
	// khi: (1) đơn đang "pending" (tạo intent lần đầu), HOẶC (2) đơn đã "processing" NHƯNG CHƯA
	// có payment_transaction_id (trạng thái biên phòng thủ — về lý thuyết không nên xảy ra vì
	// chính UPDATE này luôn set cả status lẫn code CÙNG lúc, nhưng nếu có dữ liệu cũ/thao tác tay
	// sai lệch thì vẫn cho sửa được thay vì kẹt cứng). Đơn "processing" ĐÃ CÓ code (dù còn hạn
	// hay hết hạn) hoặc cancelled/expired/completed/failed đều KHÔNG khớp WHERE này — request
	// thua cuộc đua nhận RowsAffected=0 -> ErrOrderConflict (xem UpdatePaymentCode).
	return r.db.Model(&model.Order{}).
		Where("id = ? AND (status = 'pending' OR (status = 'processing' AND (payment_transaction_id IS NULL OR payment_transaction_id = '')))", orderID).
		Updates(updates)
}

// UpdatePaymentCode - Update payment info khi tạo payment intent. Trả ErrOrderConflict nếu đơn
// không còn ở trạng thái hợp lệ để tạo/ghi đè payment code (xem comment
// buildUpdatePaymentCodeQuery) — CreatePaymentIntent tự xử lý nhánh "đã có code còn hạn" TRƯỚC
// khi gọi hàm này (đọc order.PaymentCodeExpiredAt để trả lại code cũ), nên ErrOrderConflict ở
// đây chỉ còn xảy ra khi có RACE THẬT giữa 2 request đồng thời.
func (r *OrderRepository) UpdatePaymentCode(orderID uuid.UUID, paymentCode string, expiredAt time.Time) error {
	result := r.buildUpdatePaymentCodeQuery(orderID, paymentCode, expiredAt)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return ErrOrderConflict
	}
	return nil
}

// buildUpdatePaymentInfoQuery (M-02, review vòng 5) — tách phần XÂY câu UPDATE có điều kiện ra
// khỏi phần map RowsAffected -> error, cùng mẫu buildReserveVoucherUsageQuery/
// buildRestoreAndReactivateQuery, để test DryRun gọi được ĐÚNG hàm sản xuất thật.
func (r *OrderRepository) buildUpdatePaymentInfoQuery(orderID uuid.UUID, paymentMethod, paymentGateway, transactionID string, paidAt time.Time) *gorm.DB {
	updates := map[string]interface{}{
		"payment_method":         paymentMethod,
		"payment_gateway":        paymentGateway,
		"payment_transaction_id": transactionID,
		"paid_at":                paidAt,
		"status":                 "completed",
		// L8 mục 3 (deploy cuốn chiếu): instance cũ chưa biết cột payment_code vẫn cấp mã chỉ vào
		// payment_transaction_id, nên đơn tạo trong lúc triển khai có payment_code NULL, và câu UPDATE này
		// sắp ghi đè payment_transaction_id bằng mã giao dịch ngân hàng, tức mã thanh toán mất vĩnh viễn
		// (job đối chiếu không tra lại được khoản chuyển dư). Vì vậy chép mã sang payment_code NGAY TRONG
		// CÙNG câu UPDATE: vế phải của SET luôn đọc giá trị CŨ của dòng nên payment_transaction_id ở đây
		// vẫn là mã thanh toán. Chỉ điền khi payment_code còn trống: mã đã có thì bất biến, không đụng.
		// L9 mục 2: payment_code là varchar(64) còn payment_transaction_id là varchar(255). Mã dài hơn 64 không vừa
		// cột: chép vào sẽ làm UPDATE lỗi SQLSTATE 22001 và rollback cả giao dịch hoàn tất dù khách đã trả tiền.
		// Mã thanh toán do hệ thống sinh luôn ngắn, nên đó chỉ là dữ liệu bất thường: bỏ qua việc chép (đơn vẫn hoàn
		// tất, payment_code để trống) và UpdatePaymentInfo ghi log để người vận hành thấy.
		"payment_code": gorm.Expr(
			"CASE WHEN (payment_code IS NULL OR payment_code = '') AND COALESCE(length(payment_transaction_id), 0) <= ? "+
				"THEN NULLIF(payment_transaction_id, '') ELSE payment_code END", paymentCodeMaxLen),
	}
	// M-02 (review vòng 5): UPDATE CÓ ĐIỀU KIỆN — WHERE status IN ('pending','processing') —
	// thay vì vô điều kiện như trước. Đơn phải đang ở 1 trong 2 trạng thái "còn sống" này mới
	// được chuyển "completed"; nếu một luồng khác (lazy-sweep, CancelOrder) đã đổi status trước
	// đó, UPDATE này khớp 0 dòng thay vì âm thầm ghi đè lên trạng thái đã đổi.
	return r.db.Model(&model.Order{}).
		Where("id = ? AND status IN ('pending','processing')", orderID).
		Updates(updates)
}

// logOverlongLegacyCode ghi log khi câu UPDATE hoàn tất sẽ BỎ QUA việc chép payment_transaction_id sang payment_code
// vì mã dài hơn cột (xem buildUpdatePaymentInfoQuery). SQL không log được nên đọc trước điều kiện đó; chỉ để chẩn
// đoán, nên lỗi đọc không chặn việc hoàn tất (câu UPDATE tự bảo vệ, không phụ thuộc kết quả đọc này).
func (r *OrderRepository) logOverlongLegacyCode(orderID uuid.UUID) {
	var probe struct {
		TxLen     int
		CodeEmpty bool
	}
	err := r.db.Model(&model.Order{}).
		Select("COALESCE(length(payment_transaction_id), 0) AS tx_len, (payment_code IS NULL OR payment_code = '') AS code_empty").
		Where("id = ? AND status IN ('pending','processing')", orderID).
		Take(&probe).Error
	if err != nil {
		return
	}
	if probe.CodeEmpty && probe.TxLen > paymentCodeMaxLen {
		log.Printf("[PAYMENT-CODE] order=%s payment_transaction_id dài %d ký tự (> %d của cột payment_code): không chép sang payment_code, đơn vẫn hoàn tất nhưng sẽ không tra lại được khoản chuyển dư",
			orderID, probe.TxLen, paymentCodeMaxLen)
	}
}

// UpdatePaymentInfo - Update payment information. Trả ErrOrderConflict nếu đơn không còn ở
// "pending"/"processing" tại thời điểm UPDATE thật thực thi (xem comment ErrOrderConflict).
func (r *OrderRepository) UpdatePaymentInfo(orderID uuid.UUID, paymentMethod, paymentGateway, transactionID string, paidAt time.Time) error {
	r.logOverlongLegacyCode(orderID)
	result := r.buildUpdatePaymentInfoQuery(orderID, paymentMethod, paymentGateway, transactionID, paidAt)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return ErrOrderConflict
	}
	return nil
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

// CheckOrderNumberExists - Check if order number exists
func (r *OrderRepository) CheckOrderNumberExists(orderNumber string) (bool, error) {
	var count int64
	if err := r.db.Model(&model.Order{}).Where("order_number = ?", orderNumber).Count(&count).Error; err != nil {
		return false, err
	}
	return count > 0, nil
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

// SetPlatformFeeSnapshot — xem comment tại interface. UPDATE vô điều kiện theo id: caller (trong
// PaymentService.CheckAndProcessPayment) LUÔN gọi hàm này TRONG transaction, NGAY SAU khi
// UpdatePaymentInfo (UPDATE có điều kiện WHERE status IN pending/processing) đã thành công — tại
// thời điểm này hàng đã "thuộc về" transaction hiện tại (đã ghi status='completed'), không còn
// nguy cơ race với transaction khác.
func (r *OrderRepository) SetPlatformFeeSnapshot(orderID uuid.UUID, feePercent, feeAmount decimal.Decimal) error {
	return r.db.Model(&model.Order{}).
		Where("id = ?", orderID).
		Updates(map[string]interface{}{
			"platform_fee_percent": feePercent,
			"platform_fee_amount":  feeAmount,
		}).Error
}

// buildRefundQuery (tính năng đơn hàng+hoàn tiền+doanh thu) — tách phần XÂY câu UPDATE ra khỏi
// phần đọc .RowsAffected, cùng pattern buildUpdatePaymentInfoQuery/buildRestoreAndReactivateQuery,
// để test DryRun (order_repository_test.go) gọi được ĐÚNG hàm sản xuất thật. Caller
// (AdminOrderService.RefundOrder) LUÔN gọi hàm này SAU KHI đã khoá dòng bằng GetForUpdate TRONG
// CÙNG transaction — điều kiện "WHERE status = 'completed'" ở đây là defense-in-depth (giữ cùng
// "hình dạng" UPDATE có điều kiện với phần còn lại của codebase), KHÔNG phải cơ chế chống race
// chính (cơ chế chính là row lock của GetForUpdate).
func (r *OrderRepository) buildRefundQuery(orderID uuid.UUID, reason, refundMethod, transactionRef string, refundedAt time.Time, refundedBy uuid.UUID) *gorm.DB {
	updates := map[string]interface{}{
		"status":        "refunded",
		"refund_reason": reason,
		"refund_method": refundMethod,
		"refunded_at":   refundedAt,
		"refunded_by":   refundedBy,
		// B6: mã giao dịch chuyển khoản hoàn tiền (quyết định #1), ghi cùng UPDATE chuyển trạng thái.
		"refund_transaction_ref": transactionRef,
	}
	return r.db.Model(&model.Order{}).
		Where("id = ? AND status = ?", orderID, "completed").
		Updates(updates)
}

// RefundOrder chuyển đơn sang "refunded" — applied=false nghĩa là đơn KHÔNG còn ở "completed"
// tại thời điểm UPDATE thật thực thi (double-refund, hoặc trạng thái đã đổi) — caller (đã khoá
// dòng qua GetForUpdate) coi đây là ErrOrderAlreadyRefunded.
func (r *OrderRepository) RefundOrder(orderID uuid.UUID, reason, refundMethod, transactionRef string, refundedAt time.Time, refundedBy uuid.UUID) (applied bool, err error) {
	result := r.buildRefundQuery(orderID, reason, refundMethod, transactionRef, refundedAt, refundedBy)
	if result.Error != nil {
		return false, result.Error
	}
	return result.RowsAffected > 0, nil
}

// ListAdmin - GET /api/orders/admin: liệt kê TOÀN BỘ đơn (mọi user), lọc theo AdminOrderFilter.
// Preload User để lấy user_email (contract admin list) + Items.Course để lấy course_title.
func (r *OrderRepository) ListAdmin(filter AdminOrderFilter) ([]model.Order, int64, error) {
	query := r.db.Model(&model.Order{})
	if filter.Status != "" {
		query = query.Where("status = ?", filter.Status)
	}
	if filter.UserID != nil {
		query = query.Where("user_id = ?", *filter.UserID)
	}
	if filter.From != nil {
		query = query.Where("created_at >= ?", *filter.From)
	}
	if filter.To != nil {
		query = query.Where("created_at <= ?", *filter.To)
	}

	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	page, limit := filter.Page, filter.Limit
	if page < 1 {
		page = 1
	}
	if limit < 1 {
		limit = 20
	}
	offset := (page - 1) * limit

	var orders []model.Order
	if err := query.Preload("Items.Course").Preload("User").
		Order("created_at DESC").
		Offset(offset).Limit(limit).
		Find(&orders).Error; err != nil {
		return nil, 0, err
	}

	return orders, total, nil
}
