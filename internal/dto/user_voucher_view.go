package dto

import "study.com/v1/internal/model"

// UserVoucherView là một mục của GET /vouchers/me: bản ghi ví voucher (các field của model.UserVoucher được giữ
// nguyên ở cấp ngoài) kèm kết luận "user này dùng được mã ngay bây giờ không".
type UserVoucherView struct {
	*model.UserVoucher
	// Eligible = false khi một luật chặn mã (hết hạn, tắt, đơn đầu tiên, hết lượt...). Không xét giỏ hàng.
	Eligible bool `json:"eligible"`
	// IneligibleReason: câu tiếng Việt giải thích khi Eligible = false; chuỗi rỗng khi dùng được.
	IneligibleReason string `json:"ineligible_reason"`
}
