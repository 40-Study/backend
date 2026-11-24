package router

import (
	"github.com/gofiber/fiber/v2"
	"study.com/v1/internal/middleware"
)

// parentCannotPurchase (QA6-PG-01, contract C6): phụ huynh (vai ĐANG CHỌN) không đăng ký, thêm giỏ hay mua khoá học
// cho chính mình, vì không trang nào của phụ huynh mở được khoá đã mua (giỏ, "Khoá học của tôi", khu học đều chỉ dành
// cho học viên). Chặn ở backend để gọi API trực tiếp cũng 403 ERR_PARENT_CANNOT_PURCHASE. Đặt SAU AuthMiddleware.
// Tài khoản vừa học viên vừa phụ huynh vẫn mua được khi chọn vai học viên.
func parentCannotPurchase() fiber.Handler {
	return middleware.DenyActiveRole("ERR_PARENT_CANNOT_PURCHASE",
		"Tài khoản phụ huynh không đăng ký hoặc mua khoá học cho chính mình. Hãy chuyển sang vai học viên.", "PARENT")
}
