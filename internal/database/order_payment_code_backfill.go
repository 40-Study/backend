package database

// order_payment_code_backfill.go — L6 review MAJOR. orders.payment_code (mã thanh toán bất biến) là cột mới;
// trước đó mã chỉ nằm ở payment_transaction_id và bị mã giao dịch ngân hàng ghi đè khi đơn hoàn tất.
//
// Khôi phục được: đơn CHƯA hoàn tất (pending/processing/expired/cancelled) mà payment_transaction_id còn là
// mã thanh toán, nên chép sang payment_code. KHÔNG khôi phục được: đơn completed/refunded đã hoàn tất trước
// khi có cột này (payment_transaction_id nay là mã giao dịch ngân hàng, mã thanh toán không còn ở đâu trong
// DB, order_status_history chỉ ghi "Payment initiated" không kèm mã) nên payment_code của chúng để NULL;
// job đối chiếu bỏ qua đơn không có mã chứ không tra bằng mã sai. 'failed' cũng để nguyên vì không có đường
// ghi nào cho thấy cột đó còn là mã thanh toán.
//
// Idempotent: chỉ chạm dòng payment_code IS NULL.

import (
	"fmt"

	"gorm.io/gorm"
)

const orderPaymentCodeBackfillSQL = `
	UPDATE orders
	SET payment_code = payment_transaction_id
	WHERE payment_code IS NULL
	  AND payment_transaction_id IS NOT NULL AND payment_transaction_id <> ''
	  AND status IN ('pending', 'processing', 'expired', 'cancelled');
`

// runOrderPaymentCodeBackfill gọi từ RunPostMigrations sau AutoMigrate (đã thêm cột payment_code).
func runOrderPaymentCodeBackfill(db *gorm.DB) error {
	if err := db.Exec(orderPaymentCodeBackfillSQL).Error; err != nil {
		return fmt.Errorf("post-migration %q failed: %w", "backfill orders.payment_code", err)
	}
	return nil
}
