package model

import "github.com/shopspring/decimal"

// EffectivePrice là giá THỰC TẾ người học phải trả cho khóa học: DiscountPrice nếu đang có giá
// khuyến mãi hợp lệ (khác nil, không âm, thấp hơn Price), ngược lại là Price.
//
// Phát hiện khi chạy smoke test 11/09/2026 trên DB dev: giỏ hàng (cart_service) tính tổng theo
// DiscountPrice (499.000đ) nhưng đơn hàng (order_service) lại lấy Price (999.000đ) — khách thấy
// một giá ở giỏ, bị tính một giá khác lúc thanh toán. Mọi nơi cần "giá phải trả" PHẢI gọi hàm
// này thay vì tự chọn giữa Price/DiscountPrice.
func (c *Course) EffectivePrice() decimal.Decimal {
	if c.DiscountPrice != nil && !c.DiscountPrice.IsNegative() && c.DiscountPrice.LessThan(c.Price) {
		return *c.DiscountPrice
	}
	return c.Price
}
