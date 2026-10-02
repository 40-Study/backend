package database

// course_discount_price_cleanup.go — L6 mục 2. Trước khi service chặn (L1), courses.discount_price
// có thể là 0, âm hoặc >= price. EffectivePrice nay bỏ qua giá trị đó, nhưng dòng vẫn nằm trong DB và
// vẫn bị web/báo cáo đọc thẳng cột nên dọn thành NULL (= không có khuyến mãi). Hạn khuyến mãi của
// chính những dòng đó cũng bỏ, để không còn ngày hết hạn không gắn với giá nào.
//
// Idempotent: chỉ chạm dòng vi phạm, chạy lại ở mỗi lần API khởi động không đổi gì. Khuyến mãi hợp lệ
// (0 < discount_price < price) giữ nguyên, kể cả khoá đã xoá mềm (cột là dữ liệu, không phải hiển thị).

import (
	"fmt"

	"gorm.io/gorm"
)

const courseDiscountPriceCleanupSQL = `
	UPDATE courses
	SET discount_price = NULL, discount_expires_at = NULL
	WHERE discount_price IS NOT NULL
	  AND (discount_price <= 0 OR discount_price >= price);
`

// runCourseDiscountPriceCleanup gọi từ RunPostMigrations sau AutoMigrate.
func runCourseDiscountPriceCleanup(db *gorm.DB) error {
	if err := db.Exec(courseDiscountPriceCleanupSQL).Error; err != nil {
		return fmt.Errorf("post-migration %q failed: %w", "null out invalid courses.discount_price", err)
	}
	return nil
}
