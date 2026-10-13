package database

// first_order_voucher_backfill.go — QA2 F-02: voucher seed WELCOME20 có mô tả "đơn hàng đầu tiên" nhưng cờ
// vouchers.first_order_only mới có từ bản sửa này, nên DB đã seed từ trước (kể cả production) vẫn có first_order_only
// = false và luật không bao giờ áp dụng; seed chỉ gắn cờ khi chạy seed. Bước này gắn cờ cho đúng voucher mà seed đánh
// dấu "đơn đầu tiên" (model.SeededFirstOrderVoucherCode, cùng hằng với seed).
//
// Chỉ bật (false -> true), không bao giờ tắt: admin đã tự bật cờ cho voucher khác thì giữ nguyên. CHẠY ĐÚNG MỘT LẦN
// trên mỗi DB (runDataMigrationOnce, dấu trong data_migrations): admin tắt cờ sau đó sẽ không bị bật lại mỗi lần khởi
// động. Không lọc deleted_at: voucher đã xoá mềm mà được khôi phục sau này cũng phải mang cờ.

import (
	"fmt"

	"gorm.io/gorm"
	"study.com/v1/internal/model"
)

const firstOrderVoucherBackfillName = "first_order_voucher_backfill"

// backfillFirstOrderVoucher bật first_order_only cho voucher seed và trả số dòng đã sửa (test dùng để kiểm "lần hai sửa 0").
func backfillFirstOrderVoucher(tx *gorm.DB) (int64, error) {
	res := tx.Exec(`UPDATE vouchers SET first_order_only = true WHERE code = ? AND first_order_only = false`,
		model.SeededFirstOrderVoucherCode)
	return res.RowsAffected, res.Error
}

// runFirstOrderVoucherBackfill — xem chú thích đầu file. Gọi từ RunPostMigrations sau AutoMigrate (cần cột đã tồn tại).
func runFirstOrderVoucherBackfill(db *gorm.DB) error {
	err := runDataMigrationOnce(db, firstOrderVoucherBackfillName, func(tx *gorm.DB) error {
		_, err := backfillFirstOrderVoucher(tx)
		return err
	})
	if err != nil {
		return fmt.Errorf("post-migration %q failed: %w", "backfill vouchers.first_order_only", err)
	}
	return nil
}
