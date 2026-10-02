package asynq_queue

import (
	"context"
	"fmt"
	"time"

	"github.com/hibiken/asynq"
)

// TaskPaymentReconcile (L6 mục 5): job định kỳ đối chiếu ngân hàng cho đơn chờ và đơn vừa hoàn tất.
const TaskPaymentReconcile string = "payment_reconcile_sweep"

// paymentReconcileQueue — hàng đợi ưu tiên thấp: job quét nặng không được chen việc thông báo/giao dịch.
const paymentReconcileQueue = "low"

// paymentReconcileSchedule dựng lịch và tuỳ chọn của job quét: tách thành hàm thuần để test được mà không cần
// Redis (asynq không cho đọc lại tuỳ chọn đã đăng ký).
//
// Lịch "@every Nm" cách đều mỗi N phút, đều với mọi N kể cả không chia hết 60.
//
// MaxRetry(0): job định kỳ không cần retry (chu kỳ sau tự quét lại; mặc định asynq 25 lần sẽ chạy lại một
// lượt quét nặng ngay khi lượt trước lỗi, đúng lúc ngân hàng đang chết).
func paymentReconcileSchedule(intervalMinutes int) (freq Frequency, queue string, opts []asynq.Option) {
	return EveryInterval(time.Duration(intervalMinutes) * time.Minute), paymentReconcileQueue, []asynq.Option{asynq.MaxRetry(0)}
}

// RegisterPaymentReconcile đăng ký job quét đối chiếu thanh toán chạy cách đều mỗi intervalMinutes phút
// (xem paymentReconcileSchedule).
//
// run là PaymentService.RunReconcileSweep, tự chống chạy chồng ở tiến trình và Redis, và khoá theo khung
// thời gian của chu kỳ nên asynq scheduler đăng ký cùng cron ở MỌI instance vẫn chỉ quét một lần mỗi chu kỳ.
func RegisterPaymentReconcile(q *Queue, intervalMinutes int, run func(ctx context.Context) error) error {
	if run == nil {
		return fmt.Errorf("payment reconcile: run function is required")
	}
	freq, queue, opts := paymentReconcileSchedule(intervalMinutes)
	return q.Schedule(TaskPaymentReconcile, freq, func(ctx context.Context, _ []byte) error {
		return run(ctx)
	}, queue, opts...)
}
