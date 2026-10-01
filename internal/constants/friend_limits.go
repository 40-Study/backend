package constants

import "time"

// Hạn mức tính năng Bạn bè (contract-api §1, câu hỏi Q4/Q5). Mọi số liệu đặt ở đây để đổi mà không
// phải sửa logic; giá trị là mặc định đề xuất đã được chủ dự án chấp nhận.
const (
	// FriendDailyRequestLimit — số lời mời tối đa một người gửi trong FriendDailyWindow.
	FriendDailyRequestLimit = 20
	FriendDailyWindow       = 24 * time.Hour
	// FriendPendingRequestLimit — số lời mời ĐANG CHỜ tối đa của một người gửi.
	FriendPendingRequestLimit = 30
	// FriendMaxFriends — số bạn tối đa của mỗi người (kiểm cả hai phía khi gửi/chấp nhận).
	FriendMaxFriends = 500
	// FriendDeclineCooldown — bị từ chối rồi phải chờ mới được gửi lại cho đúng người đó (Q5).
	FriendDeclineCooldown = 7 * 24 * time.Hour
	// FriendCancelCooldown — CHỈ áp dụng cho chính người thu hồi khi gửi lại cho cùng người đó (không bao giờ ở
	// chiều ngược lại): chặn kiểu "gửi-huỷ-gửi" liên tục quấy rối một người, vì lần gửi lại dùng lại dòng cũ nên
	// hạn mức 20/24h không đếm thêm. Cố ý rất ngắn (chủ dự án: cooldown dài chỉ khi bị TỪ CHỐI); đặt về 0 nếu
	// muốn bỏ hẳn.
	FriendCancelCooldown = 5 * time.Minute

	// Giới hạn tốc độ theo user (Redis, cửa sổ 1 phút): mọi POST dưới /friends dùng chung một bộ đếm.
	FriendPostsPerMinute  = 10
	FriendSearchPerMinute = 30

	// Tìm kiếm: chống liệt kê danh sách học sinh (trẻ vị thành niên), xem Q2.
	FriendSearchMinChars   = 3
	FriendSearchMaxResults = 20

	// Phân trang danh sách.
	FriendPageDefaultLimit = 20
	FriendPageMaxLimit     = 100
)
