package service

import "study.com/v1/internal/dto"

// hideInternalRefundFields xoá lý do hoàn tiền khỏi phản hồi gửi học viên: đó là ghi chú đối soát
// NỘI BỘ do admin nhập (có thể nêu tên người, vụ khiếu nại...). Học viên chỉ cần biết đơn đã hoàn
// tiền (status + refunded_at). Mọi đường trả OrderResponse cho người KHÔNG phải admin phải đi qua
// đây; đường admin (GetOrderByID với isAdmin=true) giữ nguyên để trang chi tiết admin còn dấu vết.
func hideInternalRefundFields(resp *dto.OrderResponse) *dto.OrderResponse {
	if resp != nil {
		resp.RefundReason = nil
	}
	return resp
}
