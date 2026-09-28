package model

// Trạng thái yêu cầu rút tiền của giảng viên (bảng instructor_payouts) — Phase 4, 28/09/2026.
//
// PayoutStatuses là NGUỒN SỰ THẬT DUY NHẤT cho danh sách giá trị hợp lệ của cột
// instructor_payouts.status, cùng khuôn với OrderStatuses (order_status.go):
//   - RunPostMigrations SINH câu CHECK constraint từ slice này (buildCheckConstraintSQL,
//     internal/database/check_constraint.go), không chép tay chuỗi SQL.
//   - Tag `check:` của InstructorPayout.Status (payment.go) buộc phải là literal tĩnh (giới hạn
//     struct tag của Go) — TestPayoutStatusTagMatchesSSOT đối chiếu 2 chiều tag với slice này.
//
// Bỏ "processing"/"failed" của schema cũ: bảng chưa từng có luồng ghi nào (0 service/handler
// trước Phase 4) nên không có dữ liệu cũ cần chuyển đổi.
const (
	PayoutStatusPending   = "pending"   // giáo viên vừa gửi yêu cầu
	PayoutStatusApproved  = "approved"  // admin đã duyệt, chờ chuyển khoản tay
	PayoutStatusRejected  = "rejected"  // admin từ chối (kèm lý do) — giải phóng số dư
	PayoutStatusCompleted = "completed" // admin xác nhận đã chuyển khoản xong
	// PayoutStatusCancelled (Q2, QA vòng 2): giảng viên tự huỷ khi yêu cầu còn pending (vd nhập
	// sai số tiền). Tách khỏi "rejected" vì rejected nghĩa là admin từ chối kèm lý do. Không thuộc
	// Open/Reserved nên huỷ xong là trả lại số dư và gửi được yêu cầu mới ngay.
	PayoutStatusCancelled = "cancelled"
)

var PayoutStatuses = []string{
	PayoutStatusPending, PayoutStatusApproved, PayoutStatusRejected, PayoutStatusCompleted, PayoutStatusCancelled,
}

// PayoutOpenStatuses — yêu cầu "đang xử lý" (chưa kết thúc). Quyết định chủ dự án #7: mỗi giảng
// viên chỉ được có TỐI ĐA 1 yêu cầu ở một trong các trạng thái này tại một thời điểm. "approved"
// vẫn tính là đang xử lý vì tiền chưa chuyển xong — cho gửi yêu cầu mới lúc này sẽ khiến admin
// phải đối soát 2 đợt chuyển khoản chồng nhau cho cùng 1 người.
var PayoutOpenStatuses = []string{PayoutStatusPending, PayoutStatusApproved}

// PayoutReservedStatuses — các yêu cầu đã "giữ chỗ" trên số dư: số dư khả dụng trừ đi tổng tiền
// của mọi yêu cầu ở các trạng thái này. Chỉ "rejected" được loại ra, nên từ chối 1 yêu cầu trả
// lại số dư ngay lập tức mà không cần bút toán riêng.
var PayoutReservedStatuses = []string{PayoutStatusPending, PayoutStatusApproved, PayoutStatusCompleted}

// IsValidPayoutStatus dùng để validate query ?status= từ client.
func IsValidPayoutStatus(s string) bool {
	for _, v := range PayoutStatuses {
		if v == s {
			return true
		}
	}
	return false
}
