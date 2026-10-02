package asynq_queue

import (
	"context"
	"fmt"
	"time"

	"github.com/hibiken/asynq"
)

// TaskPaymentReconcile (L6 mục 5): job định kỳ đối chiếu ngân hàng cho đơn chờ và đơn vừa hoàn tất.
const TaskPaymentReconcile string = "payment_reconcile_sweep"

// RegisterPaymentReconcile đăng ký job quét đối chiếu thanh toán chạy cách đều mỗi intervalMinutes phút
// ("@every Nm", đều với mọi N kể cả không chia hết 60).
//
// MaxRetry(0): job định kỳ không cần retry (chu kỳ sau tự quét lại; mặc định asynq 25 lần sẽ chạy lại một
// lượt quét nặng ngay khi lượt trước lỗi, đúng lúc ngân hàng đang chết).
//
// run là PaymentService.RunReconcileSweep, tự chống chạy chồng ở tiến trình và Redis, và khoá theo khung
// thời gian của chu kỳ nên asynq scheduler đăng ký cùng cron ở MỌI instance vẫn chỉ quét một lần mỗi chu kỳ.
func RegisterPaymentReconcile(q *Queue, intervalMinutes int, run func(ctx context.Context) error) error {
	if run == nil {
		return fmt.Errorf("payment reconcile: run function is required")
	}
	return q.Schedule(TaskPaymentReconcile, EveryInterval(time.Duration(intervalMinutes)*time.Minute), func(ctx context.Context, _ []byte) error {
		return run(ctx)
	}, "low", asynq.MaxRetry(0))
}
