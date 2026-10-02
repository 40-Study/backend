package service

// payment_sweep_failures.go — L9 mục 1. Sổ nhớ các đơn từng tra ngân hàng LỖI ở lượt quét trước.
//
// Vì sao: ngắt mạch của job đối chiếu dừng lượt quét sau reconcileMaxConsecutiveBankErrors lần lỗi LIÊN TIẾP. Lượt
// quét đi theo created_at tăng dần, nên từ 3 đơn chờ cũ nhất mà lỗi dai dẳng RIÊNG TỪNG ĐƠN (ngân hàng vẫn trả lời
// đơn khác) thì mọi lượt đều dừng sau đúng 3 lỗi và các đơn trẻ hơn không bao giờ được tới (đơn lỗi ở lại danh sách tới
// 4 ngày vì job không chốt expired khi chưa xác minh được).
//
// Cách phân biệt lỗi toàn cục với lỗi riêng một đơn không dựa vào nội dung lỗi (service Python dùng cùng một
// Status="error" cho cả hai) mà dựa vào BẰNG CHỨNG: ngân hàng đã trả lời một đơn khác trong cùng lượt quét. Sổ này làm
// hai việc:
//   - xoay vòng: đơn đã lỗi lần trước xếp SAU các đơn chưa lỗi (rồi theo lần lỗi gần nhất, lâu nhất trước), nên mỗi lượt
//     chạm tới đơn mới thay vì lặp lại cùng 3 đơn hỏng. Ngân hàng chết hẳn thì mỗi lượt vẫn chỉ tốn đúng 3 lần gọi
//     nhưng trên 3 đơn KHÁC nhau;
//   - đơn đã lỗi mà ngân hàng vẫn trả lời đơn khác trong lượt này là lỗi riêng đơn: không tính vào ngắt mạch (không
//     báo nhầm "ngân hàng chết") và bị giới hạn reconcileMaxKnownFailingPerSweep lần thử mỗi lượt.
//
// Bộ nhớ nằm trong tiến trình (mất khi khởi động lại thì chỉ quay về hành vi cũ một lượt), không thêm cột DB.

import (
	"sort"
	"sync"
	"time"

	"github.com/google/uuid"
	"study.com/v1/internal/model"
)

// reconcileMaxKnownFailingPerSweep — số lần thử tối đa mỗi lượt cho các đơn ĐÃ lỗi ở lượt trước, tính sau khi ngân
// hàng đã trả lời được ít nhất một đơn. Chặn chi phí (mỗi lần lỗi có thể chờ tới bankLookupTimeout) mà không để đơn
// hỏng chiếm hết lượt; phần dư xoay sang lượt sau nhờ thứ tự theo lần lỗi gần nhất.
const reconcileMaxKnownFailingPerSweep = 3

// bankFailureLedger ghi lần lỗi gần nhất của từng đơn. Giá trị rỗng dùng được ngay.
type bankFailureLedger struct {
	mu       sync.Mutex
	failedAt map[uuid.UUID]time.Time
}

// has: đơn từng lỗi ở lượt trước (và chưa được ngân hàng trả lời kể từ đó).
func (l *bankFailureLedger) has(id uuid.UUID) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	_, ok := l.failedAt[id]
	return ok
}

func (l *bankFailureLedger) recordFailure(id uuid.UUID, at time.Time) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.failedAt == nil {
		l.failedAt = map[uuid.UUID]time.Time{}
	}
	l.failedAt[id] = at
}

func (l *bankFailureLedger) clear(id uuid.UUID) {
	l.mu.Lock()
	defer l.mu.Unlock()
	delete(l.failedAt, id)
}

// retainOnly bỏ các đơn không còn trong danh sách quét (đã rời cửa sổ hoặc đổi trạng thái) để sổ không phình mãi.
func (l *bankFailureLedger) retainOnly(live map[uuid.UUID]bool) {
	l.mu.Lock()
	defer l.mu.Unlock()
	for id := range l.failedAt {
		if !live[id] {
			delete(l.failedAt, id)
		}
	}
}

// failedLast sắp ổn định: đơn chưa từng lỗi giữ nguyên thứ tự gốc ở đầu, đơn đã lỗi xếp sau theo lần lỗi gần nhất
// (lâu nhất trước).
func (l *bankFailureLedger) failedLast(orders []model.Order) {
	l.mu.Lock()
	defer l.mu.Unlock()
	sort.SliceStable(orders, func(i, j int) bool {
		ti, failedI := l.failedAt[orders[i].ID]
		tj, failedJ := l.failedAt[orders[j].ID]
		if failedI != failedJ {
			return !failedI
		}
		return ti.Before(tj)
	})
}
