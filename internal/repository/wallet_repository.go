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

// ─── Phần giảng viên của 1 dòng order_items (SSOT, Phase 4) ─────────────────
//
// Quyết định chủ dự án #2: phí nền tảng % được CHỐT vào từng đơn lúc thanh toán
// (orders.platform_fee_amount, tính trên orders.total_amount — PaymentService.CheckAndProcessPayment).
// Một đơn có thể chứa khoá của NHIỀU giảng viên, nên phí cấp đơn được phân bổ về từng item theo tỉ
// lệ final_price / tổng final_price của cả đơn:
//
//	phí của item = ROUND(platform_fee_amount × final_price / tổng final_price đơn, 2)
//	phần GV của item = final_price − phí của item
//
// Làm tròn từng item có thể lệch 1–2 xu so với phí cả đơn, nên item CUỐI của đơn (id lớn nhất, cố
// định) nhận phần dư: phí của nó = phí đơn − tổng phí đã làm tròn của các item còn lại. Nhờ vậy
// Σ phần GV + phí đơn = tổng final_price của đơn, đúng tới từng xu (review Phase 4, MINOR).
//
// Dùng SỐ TIỀN phí đã chốt (không tính lại từ %) để khớp đúng báo cáo doanh thu admin
// (teacher_share = gross − platform_fee_amount) với đơn 1 khoá. Đơn tạo trước khi có phí có
// platform_fee_amount = 0 nên phần GV = final_price như trước. Chỉ đơn `completed` được tính: đơn
// `refunded` rơi khỏi tổng (đó là cách "trừ doanh thu giảng viên khi hoàn tiền" của Phase 2).
const teacherEarningsJoins = `JOIN orders ON orders.id = order_items.order_id
	JOIN courses ON courses.id = order_items.course_id
	JOIN (
		SELECT r.id,
			CASE WHEN r.items_total <= 0 THEN 0
				WHEN r.rn = 1 THEN r.fee - (SUM(r.rounded) OVER (PARTITION BY r.order_id) - r.rounded)
				ELSE r.rounded END AS fee_share
		FROM (
			SELECT oi.id, oi.order_id, o.platform_fee_amount AS fee,
				SUM(oi.final_price) OVER (PARTITION BY oi.order_id) AS items_total,
				ROW_NUMBER() OVER (PARTITION BY oi.order_id ORDER BY oi.id DESC) AS rn,
				CASE WHEN SUM(oi.final_price) OVER (PARTITION BY oi.order_id) > 0
					THEN ROUND(o.platform_fee_amount * oi.final_price / SUM(oi.final_price) OVER (PARTITION BY oi.order_id), 2)
					ELSE 0 END AS rounded
			FROM order_items oi JOIN orders o ON o.id = oi.order_id
			WHERE o.status = 'completed'
		) AS r
	) AS item_fee ON item_fee.id = order_items.id`

const teacherShareExpr = `order_items.final_price - item_fee.fee_share`

// GetTeacherEarnings trả tổng phần giảng viên (sau phí nền tảng) của các đơn `completed`.
func (r *WalletRepository) GetTeacherEarnings(teacherID uuid.UUID) (*TeacherEarningsRow, error) {
	var res TeacherEarningsRow
	err := r.db.Model(&model.OrderItem{}).
		Joins(teacherEarningsJoins).
		Where("courses.instructor_id = ? AND orders.status = ?", teacherID, "completed").
		Select("COALESCE(SUM(" + teacherShareExpr + "), 0) AS total_earnings, COUNT(DISTINCT orders.id) AS order_count").
		Scan(&res).Error
	return &res, err
}

// GetTeacherEarningsByIDs — cùng công thức GetTeacherEarnings, gom nhóm cho nhiều giảng viên trong
// 1 câu (trang admin rút tiền). Giảng viên không có đơn nào sẽ vắng mặt trong map (= 0).
func (r *WalletRepository) GetTeacherEarningsByIDs(teacherIDs []uuid.UUID) (map[uuid.UUID]decimal.Decimal, error) {
	out := make(map[uuid.UUID]decimal.Decimal, len(teacherIDs))
	if len(teacherIDs) == 0 {
		return out, nil
	}
	var rows []struct {
		TeacherID uuid.UUID
		Total     decimal.Decimal
	}
	err := r.db.Model(&model.OrderItem{}).
		Joins(teacherEarningsJoins).
		Where("courses.instructor_id IN ? AND orders.status = ?", teacherIDs, "completed").
		Select("courses.instructor_id AS teacher_id, COALESCE(SUM(" + teacherShareExpr + "), 0) AS total").
		Group("courses.instructor_id").
		Scan(&rows).Error
	for _, row := range rows {
		out[row.TeacherID] = row.Total
	}
	return out, err
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

// TeacherPayoutSums — tổng tiền yêu cầu rút của 1 giảng viên theo nhóm trạng thái.
type TeacherPayoutSums struct {
	Open      decimal.Decimal // pending + approved (đang xử lý)
	Completed decimal.Decimal // đã chuyển khoản xong
}

// Reserved = tổng đã giữ chỗ trên số dư (mọi trạng thái trừ rejected), xem model.PayoutReservedStatuses.
func (s TeacherPayoutSums) Reserved() decimal.Decimal { return s.Open.Add(s.Completed) }

// GetTeacherPayoutSums đọc tổng yêu cầu rút của giảng viên. Khi gọi trên repo tạo từ tx đã khoá
// teacher_profiles (WithdrawalRepository.LockTeacherProfile), kết quả không bị request rút song song
// của cùng giảng viên làm lệch.
func (r *WalletRepository) GetTeacherPayoutSums(teacherID uuid.UUID) (TeacherPayoutSums, error) {
	var res struct {
		Open      decimal.Decimal
		Completed decimal.Decimal
	}
	err := r.db.Model(&model.InstructorPayout{}).
		Where("instructor_id = ?", teacherID).
		Select(`COALESCE(SUM(amount) FILTER (WHERE status IN ?), 0) AS open,
			COALESCE(SUM(amount) FILTER (WHERE status = ?), 0) AS completed`,
			model.PayoutOpenStatuses, model.PayoutStatusCompleted).
		Scan(&res).Error
	return TeacherPayoutSums{Open: res.Open, Completed: res.Completed}, err
}

// GetTeacherReservedByIDs — tổng giữ chỗ (model.PayoutReservedStatuses) theo từng giảng viên.
func (r *WalletRepository) GetTeacherReservedByIDs(teacherIDs []uuid.UUID) (map[uuid.UUID]decimal.Decimal, error) {
	out := make(map[uuid.UUID]decimal.Decimal, len(teacherIDs))
	if len(teacherIDs) == 0 {
		return out, nil
	}
	var rows []struct {
		InstructorID uuid.UUID
		Total        decimal.Decimal
	}
	err := r.db.Model(&model.InstructorPayout{}).
		Where("instructor_id IN ? AND status IN ?", teacherIDs, model.PayoutReservedStatuses).
		Select("instructor_id, COALESCE(SUM(amount), 0) AS total").
		Group("instructor_id").
		Scan(&rows).Error
	for _, row := range rows {
		out[row.InstructorID] = row.Total
	}
	return out, err
}

// GetTeacherIDsWithReservedPayouts — số dư chỉ có thể âm khi giảng viên đã có yêu cầu rút giữ chỗ
// (thu nhập không bao giờ âm), nên đây là tập ứng viên duy nhất cần xét cho cảnh báo số dư âm.
func (r *WalletRepository) GetTeacherIDsWithReservedPayouts() ([]uuid.UUID, error) {
	var ids []uuid.UUID
	err := r.db.Model(&model.InstructorPayout{}).
		Where("status IN ?", model.PayoutReservedStatuses).
		Distinct().
		Pluck("instructor_id", &ids).Error
	return ids, err
}
