package service

import (
	"context"

	"github.com/google/uuid"
	"study.com/v1/internal/apperr"
	"study.com/v1/internal/model"
)

// Review QA2 H2 (quyết định chủ dự án 13/10): khoá `archived` ngừng bán nhưng người ĐÃ ghi danh vẫn học được. Mọi đường BÁN
// hoặc GHI DANH MỚI (giỏ hàng, tạo đơn cart/buy_now, mở phiên thanh toán của đơn cũ, tự ghi danh miễn phí) trả lỗi này.
// Khoá chưa xuất bản (draft/pending_review/rejected) vẫn đi đường "không tồn tại" (ErrCourseHidden/404) như trước.
var ErrCourseNotForSale = apperr.ConflictCode("COURSE_NOT_FOR_SALE",
	"Khóa học này đã ngừng bán nên không thể thêm vào giỏ hàng, mua hoặc ghi danh mới.")

// isCourseNotForSale: khoá đã tồn tại và từng xuất bản nhưng hiện ngừng bán.
func isCourseNotForSale(status string) bool {
	return status == model.CourseStatusArchived
}

// isInsiderOnlyCourseStatus: trạng thái mà chỉ chủ khoá, admin và người đã ghi danh được xem/học: khoá chưa xuất bản
// (isPrivateCourseStatus) và khoá đã ngừng bán (archived).
func isInsiderOnlyCourseStatus(status string) bool {
	return isPrivateCourseStatus(status) || isCourseNotForSale(status)
}

// ensureOrderCoursesForSale: ErrCourseNotForSale nếu BẤT KỲ khoá nào trong đơn đã ngừng bán.
func (s *PaymentService) ensureOrderCoursesForSale(ctx context.Context, orderID uuid.UUID) error {
	var n int64
	err := s.orderRepo.TxDB().WithContext(ctx).Model(&model.OrderItem{}).
		Joins("JOIN courses ON courses.id = order_items.course_id").
		Where("order_items.order_id = ? AND courses.status = ?", orderID, model.CourseStatusArchived).
		Count(&n).Error
	if err != nil {
		return err
	}
	if n > 0 {
		return ErrCourseNotForSale
	}
	return nil
}
