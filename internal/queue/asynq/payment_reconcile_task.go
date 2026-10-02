package asynq_queue

import (
	"context"
	"fmt"
)

// TaskPaymentReconcile (L6 mục 5): job định kỳ đối chiếu ngân hàng cho đơn chờ và đơn vừa hoàn tất.
const TaskPaymentReconcile string = "payment_reconcile_sweep"

// RegisterPaymentReconcile đăng ký job quét đối chiếu thanh toán chạy mỗi intervalMinutes phút.
// run là PaymentService.RunReconcileSweep (đã tự khoá chống chạy chồng ở tiến trình và Redis).
// asynq scheduler đăng ký cùng cron ở MỌI instance nên mỗi chu kỳ có thể sinh nhiều task; khoá trong
// run làm các task thừa bỏ qua, nên job idempotent và không chạy chồng.
func RegisterPaymentReconcile(q *Queue, intervalMinutes int, run func(ctx context.Context) error) error {
	if run == nil {
		return fmt.Errorf("payment reconcile: run function is required")
	}
	return q.Schedule(TaskPaymentReconcile, EveryNMinutes(intervalMinutes), func(ctx context.Context, _ []byte) error {
		return run(ctx)
	}, "low")
}
